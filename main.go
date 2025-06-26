package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sync/errgroup"
	drive "google.golang.org/api/drive/v3"
)

const (
	securePrefix = "19xYG2-JL-onVDe01kDQUgtu5eG9HHLIG"

	numWorkers = 5
)

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

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	ctx := context.Background()

	driveService, err := NewDrive(ctx)

	if err != nil {
		log.Println("Error initializing drive service:", err)
		return
	}

	// err = driveService.DeleteAllFiles(ctx)
	// if err != nil {
	// 	fmt.Println(err.Error())
	// 	return
	// }

	// driveService.PrintAllFiles(ctx)
	driveService.About(ctx)

	fileIDs, err := driveService.ListRecordIds(ctx)
	if err != nil {
		log.Println("Error listing files:", err)
		return
	}

	group, gctx := errgroup.WithContext(ctx)
	for id, name := range recordTypeMap {
		group.Go(func() error {
			count, err := syncRecords(gctx, driveService, id, fileIDs)
			if err != nil {
				return err
			}
			log.Printf("sync'd %d files of type: %s", count, name)
			return nil
		})
	}
	err = group.Wait()
	log.Println(err)
}

// syncRecords syncs all records of recordType from the city repository to Drive
func syncRecords(ctx context.Context, driveService *Drive, recordType RecordType, currentFileIDs map[string]string) (int64, error) {
	count := int64(0)

	group, gctx := errgroup.WithContext(ctx)
	tasks := make(chan *Record)

	for i := 0; i < numWorkers; i++ {
		group.Go(func() error {
			for record := range tasks {
				err := transferRecord(gctx, driveService, record)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	name := recordTypeMap[recordType]
	records, err := fetchRecords(ctx, driveService, recordType)
	if err != nil {
		return count, fmt.Errorf("Error fetching records for type: %d, %w", recordType, err)
	}

	folder, err := driveService.FindOrCreateFolder(ctx, name)
	if err != nil {
		return count, fmt.Errorf("Error creating folder: %s, %w", name, err)
	}

	for _, record := range records.Data {
		count += 1
		record.ParentId = folder.Id
		if _, ok := currentFileIDs[record.ID]; ok {
			continue
		}

		select {
		case <-ctx.Done():
			return count, ctx.Err()
		case tasks <- record:
		}
	}

	return count, group.Wait()
}

// transferRecords iterates the passed in records, downloads from the city records site and uploads to google drive
func transferRecord(ctx context.Context, driveService *Drive, record *Record) error {
	body, err := fetchDocument(ctx, record.ID)
	if err != nil {
		log.Println("Error fetching document :", err)
		return err
	}
	defer body.Close()

	err = driveService.UploadRecord(ctx, record, body)
	if err != nil {
		log.Println("Error uploading document :", err)
		return err
	}

	return nil
}

// fetchRecords fetches records from the city records site (records.cityofberkeley.info)
// It caches these records into drive and will fetch them from drive on subsequent calls
// If the set of records is too old (older than 7 days), they are refetched and cached again.
func fetchRecords(ctx context.Context, driveService *Drive, queryID RecordType) (*Records, error) {

	var data Records

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
	}()

	// Fetch records from google drive first
	// If the set of records are too old, refetch from city records site
	var reader io.ReadCloser

	name := recordTypeMap[queryID]
	key, _ := propertyKeyValue(name, "")

	recordID := fmt.Sprintf("%s:%s:%d", securePrefix, key, queryID)

	file, err := driveService.FindFileByProperty(ctx, "record_id", recordID)
	if err != nil {
		return nil, fmt.Errorf("Error fetching file: %w", err)
	}

	var created time.Time
	if file != nil {
		created, err = time.Parse(time.RFC3339, file.CreatedTime)
		if err != nil {
			return nil, fmt.Errorf("Error parsing created time: %w", err)
		}
	}

	// check if records are out of date
	if diff := time.Now().Sub(created); diff.Hours() < 24 {
		reader, err = driveService.DownloadFile(ctx, file.Id)
		if err != nil {
			fmt.Println("Error downloading file: " + recordID)
			return nil, fmt.Errorf("Error downloading file: %s: %w", recordID, err)
		}
	}

	if reader != nil {
		fmt.Println("Fetched cached records for type: ", name)
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
		fmt.Println("Records fetch request failed with status code:  %d. Body: %s", resp.StatusCode, string(body))
		return nil, err
	}

	// Upload to drive
	file = &drive.File{
		Name: key + ".json",
		Properties: map[string]string{
			"record_id": recordID,
		},
	}
	err = driveService.UploadFile(ctx, file, bytes.NewReader(body))
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

func fetchDocument(ctx context.Context, id string) (io.ReadCloser, error) {
	id = url.QueryEscape(id)
	url := "https://records.cityofberkeley.info/PublicAccess/api/Document/" + id + "/"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request for Document id: %s: %v", id, err)
		return nil, err
	}
	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		log.Printf("Document fetch request for: %s failed with status code:  %d. Body: %s", id, resp.StatusCode, string(body))
		return nil, err
	}

	return resp.Body, nil
}

type Record struct {
	ID                  string
	Type                RecordType `json:"-"` // This field will be ignored
	Name                string
	DisplayType         string
	DisplayColumnValues []DisplayColumnValue
	DisplayColumns      []DisplayColumn `json:"-"` // This field will be ignored
	ParentId            string
	Summary             string
}

func (r *Record) Merge(fixed Record) {

	if fixed.Name != "" {
		r.Name = fixed.Name
	}
	if len(r.DisplayColumnValues) != len(fixed.DisplayColumnValues) {
		return
	}

	for i, column := range fixed.DisplayColumnValues {
		if column.Value != "" {
			r.DisplayColumnValues[i].Value = column.Value
		}
		if column.RawValue != "" {
			r.DisplayColumnValues[i].RawValue = column.RawValue
		}
	}
}

func (r Record) Properties() map[string]string {
	m := map[string]string{}
	for i, column := range r.DisplayColumns {
		if column.Heading == "" {
			continue
		}
		m[column.Heading] = r.DisplayColumnValues[i].Value
	}
	return m
}

func (r Record) DocDate() (time.Time, error) {

	value, rawValue := r.fromDisplayType("date")
	if rawValue == "" && value == "" {
		return time.Time{}, fmt.Errorf("Date columns are empty")
	}
	millis, err := strconv.ParseInt(rawValue, 10, 64)
	if err != nil {
		return time.Parse("1/_2/2006", value)
	}
	return time.UnixMilli(millis), nil
}

func (r Record) DocSource() string {
	value, rawValue := r.fromDisplayHeading("doc source")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
}

func (r Record) DocName() string {
	value, rawValue := r.fromDisplayHeading("doc name")
	switch {
	case value != "" && value != "null":
		return value
	case rawValue != "" && rawValue != "null":
		return rawValue
	default:
		return r.Name
	}
}

func (r Record) MeetingType() string {
	value, rawValue := r.fromDisplayHeading("meeting type")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
}

func (r Record) fromDisplayType(_type string) (string, string) {
	for i, column := range r.DisplayColumns {
		if strings.ToLower(column.DataType) == strings.ToLower(_type) {
			value := r.DisplayColumnValues[i].Value
			rawValue := r.DisplayColumnValues[i].RawValue
			return value, rawValue
		}
	}
	return "", ""
}

func (r Record) fromDisplayHeading(name string) (string, string) {
	for i, column := range r.DisplayColumns {
		if strings.ToLower(column.Heading) == strings.ToLower(name) {
			value := r.DisplayColumnValues[i].Value
			rawValue := r.DisplayColumnValues[i].RawValue
			return value, rawValue
		}
	}
	return "", ""
}

type Records struct {
	Data           []*Record
	Truncated      bool
	DisplayColumns []DisplayColumn
}

type DisplayColumn struct {
	Heading  string
	DataType string
}

type DisplayColumnValue struct {
	Value    string
	RawValue string
}
