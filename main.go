package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
	drive "google.golang.org/api/drive/v3"
)

const (
	numWorkers = 1

	SHARED_DRIVE_ID_ENV_VAR = "SHARED_DRIVE_ID"
)

var driveID = os.Getenv(SHARED_DRIVE_ID_ENV_VAR)

type RecordType int

const (
	ALL_RECORDS    RecordType = 127
	COMMUNICATIONS            = 129
	CONTRACTS                 = 126
	ELECTION_INFO             = 114
	MINUTES                   = 131
	ORDINACES                 = 132
	RESOLUTIONS               = 133
	STAFF_REPORTS             = 134
)

var (
	recordTypeMap map[RecordType]string = map[RecordType]string{
		COMMUNICATIONS: "Communications",
		CONTRACTS:      "Contracts",
		ELECTION_INFO:  "Elections Info",
		MINUTES:        "Minutes",
		ORDINACES:      "Ordinances",
		RESOLUTIONS:    "Resolutions",
		STAFF_REPORTS:  "Staff Reports",
	}
)

var client = &http.Client{}
var driveService *Drive
var failedRecords sync.Map

func init() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)

}

func cleanup() {
	log.Printf("Cleaning up...")

	failed := make(map[string]string)
	failedRecords.Range(func(k, v interface{}) bool {
		failed[k.(string)] = v.(string)
		return true
	})

	b, _ := json.MarshalIndent(failed, " ", " ")
	log.Printf("Failed records:\n%s", string(b))

	os.Exit(0)
}

func main() {

	if driveID == "" {
		log.Fatalf("Missing %s env var", SHARED_DRIVE_ID_ENV_VAR)
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		log.Println("Caught shutdown signal")
		cleanup()
	}()

	ctx := context.Background()

	var err error

	driveService, err = NewDrive(ctx, driveID)

	if err != nil {
		log.Println("Error initializing drive service:", err)
		return
	}

	// fileMap, err := buildFileMap(ctx, driveService)
	// if err != nil {
	// 	log.Println("Error building filemape:", err)
	// 	return
	// }

	// group, gctx := errgroup.WithContext(ctx)
	for id, name := range recordTypeMap {
		// group.Go(func() error {
		count, err := syncRecords(ctx, driveService, id)
		if err != nil {
			log.Println("Error syncing records:", err)
			return
		}
		log.Printf("sync'd %d files of type: %s", count, name)
		// })
	}

	// err = group.Wait()

	if err != nil {
		log.Println(err)
	}

	cleanup()

}

func buildFileMap(ctx context.Context, driveService *Drive) (*DriveFileMap, error) {

	bar := progressbar.Default(
		int64(-1),
		fmt.Sprintf("Fetching existing files from Drive"),
	)

	defer func() {
		bar.Finish()
		bar.Exit()
	}()

	files, err := driveService.ListSpace(ctx, "drive", bar.Add)

	if err != nil {
		log.Println("Error listing files:", err)
		return nil, err
	}

	return NewDriveFileMap(files), nil
}

// syncRecords syncs all records of recordType from the city repository to Drive
func syncRecords(ctx context.Context, driveService *Drive, recordType RecordType) (int64, error) {

	// log.Printf("Syncing records for recordType: %v", recordType)
	count := int64(0)

	// fetch records from drive
	records, err := fetchRecords(ctx, driveService, recordType)
	if err != nil {
		return count, fmt.Errorf("Error fetching records for type: %d, %w", recordType, err)
	}

	// save records on return
	defer func() {
		if count == 0 {
			return
		}
		serr := saveRecords(ctx, records)
		if serr != nil {
			fmt.Printf("Error saving records of type: %s: %s", recordTypeMap[recordType], serr.Error())
		}
	}()

	var newRecords []*Record
	for _, record := range records.Data {
		if record.Persisted {
			continue
		}
		newRecords = append(newRecords, record)
	}

	name := recordTypeMap[recordType]
	bar := progressbar.Default(
		int64(len(newRecords)),
		fmt.Sprintf("Syncing %s", name),
	)

	defer func() {
		bar.Finish()
		bar.Exit()
	}()

	group, gctx := errgroup.WithContext(ctx)
	tasks := make(chan *Record)

	for i := 0; i < numWorkers; i++ {
		group.Go(func() error {

			defer func() {
				if r := recover(); r != nil {
					fmt.Println("Recovered in f", r)
					log.Printf("%s: %s", r, debug.Stack())
				}
			}()

			for record := range tasks {

				err := transferRecord(gctx, driveService, record)

				switch {
				case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
					return err
				case err == nil:
					record.Persisted = true
				default:
					b, _ := json.MarshalIndent(record, " ", " ")
					log.Printf("%s\n%s", err.Error(), string(b))
					failedRecords.Store(record.ID, err.Error())
				}

				bar.Add(1)
			}

			return nil
		})
	}

	folder, err := driveService.FindOrCreateFolder(ctx, name, driveID)
	if err != nil {
		return count, fmt.Errorf("Error creating folder: %s, %w", name, err)
	}

	for _, record := range newRecords {
		_, ignore := ignoredIds[record.ID]
		if ignore || record.Persisted {
			bar.Add(1)
			continue
		}
		count += 1
		record.ParentId = folder.Id

		// log.Println("Pushing: " + record.Name)
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case tasks <- record:
			// log.Println("Pushed: " + record.Name)
		}
	}

	close(tasks)

	return count, group.Wait()
}

// transferRecords iterates the passed in records, downloads from the city records site and uploads to google drive
func transferRecord(ctx context.Context, driveService *Drive, record *Record) error {
	// log.Printf("Transferring record: %s\n", record.Name)
	body, length, err := fetchDocument(ctx, record.ID)
	if err != nil {
		return fmt.Errorf("Error fetching document: %w", err)
	}
	defer func() {
		body.Close()
	}()

	err = driveService.UploadRecord(ctx, record, body, length)
	if err != nil {
		return fmt.Errorf("Error uploading document: %w", err)
	}
	// log.Printf("Done transfering record: %s\n", record.Name)

	return nil
}

// fetchRecords fetches records from the city records site (records.cityofberkeley.info)
// It caches these records into drive and will fetch them from drive on subsequent calls
// If the set of records is too old (older than 7 days), they are refetched and cached again.
func fetchRecords(ctx context.Context, driveService *Drive, queryID RecordType) (*Records, error) {

	var data Records

	name := recordTypeMap[queryID]
	key, _ := propertyKeyValue(name, "")
	fmt.Println("Fetching records for type: ", name)

	recordID := fmt.Sprintf("%s:%s:%d", driveID, key, queryID)

	// populate each record with the top level DisplayColumns, which
	// hold information on what individual record display column values hold
	// see Records type
	defer func() {
		for _, record := range data.Data {
			if fixed, ok := fixedRecords[record.ID]; ok {
				record.Merge(fixed)
			}
			record.Type = queryID
			record.DisplayColumns = data.DisplayColumns
		}

		data.ID = recordID
		data.Name = key + ".json"
	}()

	// Fetch records from google drive first
	// If the set of records are too old, refetch from city records site
	var reader io.ReadCloser

	query := fmt.Sprintf("properties has { key='record_id' and value='%s' }", recordID)

	bar := progressbar.Default(
		int64(-1),
		fmt.Sprintf("Fetching records for: %s", name),
	)
	defer func() {
		bar.Finish()
		bar.Exit()
	}()

	files, err := driveService.ListSpace(ctx, appDataFolderID, bar.Add, query)
	if err != nil {
		return nil, fmt.Errorf("Error fetching file: %w", err)
	}

	// var created time.Time
	if len(files) > 0 {
		file := files[0]
		reader, err = driveService.DownloadFile(ctx, file.Id)
		if err != nil {
			return nil, fmt.Errorf("Error parsing created time: %w", err)
		}
		if reader == nil {
			panic("file not found")
		}
	}

	// check if records are out of date
	// if diff := time.Now().Sub(created); diff.Hours() < 24 {
	// 	reader, err = driveService.DownloadFile(ctx, file.Id)
	// 	if err != nil {
	// 		fmt.Println("Error downloading file: " + recordID)
	// 		return nil, fmt.Errorf("Error downloading file: %s: %w", recordID, err)
	// 	}
	// }

	if reader != nil {
		// fmt.Println("Fetched cached records for type: ", name)
		defer reader.Close()
		decoder := json.NewDecoder(reader)
		err := decoder.Decode(&data)
		return &data, err
	}

	payload := struct {
		QueryID    int
		Keywords   []string
		QueryLimit int
	}{
		int(queryID),
		[]string{},
		0,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling payload: %s: %w", recordID, err)
	}

	fmt.Printf("Fetching records: %s\n", recordTypeMap[queryID])
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://records.cityofberkeley.info/PublicAccess/api/CustomQuery/KeywordSearch",
		bytes.NewReader(b))

	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s: %w", recordID, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request for queryID: %d: %v", queryID, err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return nil, err
	}

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Records fetch request failed with status code:  %d. Body: %s\n", resp.StatusCode, string(body))
		return nil, err
	}

	// Upload to drive
	file := &drive.File{
		Name:    key + ".json",
		Parents: []string{appDataFolderID},
		Properties: map[string]string{
			"record_id": recordID,
		},
	}
	_, err = driveService.UploadFile(ctx, file, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Error uploading records: %s: %w", recordID, err)
	}

	// Unmarshal the JSON response into the ResponseData struct
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling json: %s: %w", recordID, err)
	}

	return &data, nil

}

// saveRecords persists records, along with any update state (i.e. Persisted), back to Drive
func saveRecords(ctx context.Context, data *Records) error {

	fmt.Println("Saving records: " + data.Name)
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("Error marshaling records for persistence: %w", err)
	}

	file := &drive.File{
		Name:    data.Name,
		Parents: []string{appDataFolderID},
		Properties: map[string]string{
			"record_id": data.ID,
		},
	}

	_, err = driveService.UploadFile(ctx, file, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("Error saving records: %s: %w", data.ID, err)
	}

	fmt.Println("Finished saving records: " + data.Name)
	return nil
}

func fetchDocument(ctx context.Context, origID string) (io.ReadCloser, int64, error) {
	// log.Printf("Fetching record id: %s", origID)
	id := url.QueryEscape(origID)
	url := "https://records.cityofberkeley.info/PublicAccess/api/Document/" + id + "/"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Error sending request for Document id: %s: %w", id, err)
	}
	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("Document fetch request for: %s failed with status code:  %d. Body: %s", origID, resp.StatusCode, string(body))
	}

	return resp.Body, resp.ContentLength, nil
}
