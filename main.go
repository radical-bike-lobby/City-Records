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
)

const (
	securePrefix = "19xYG2-JL-onVDe01kDQUgtu5eG9HHLIG"

	numWorkers = 10
)
const (
	ALL_RECORDS    = 127
	COMMUNICATIONS = 129
	CONTRACTS      = 126
	ELECTION_INFO  = 114
	MINUTES        = 131
	ORDINACES      = 132
	RESOLUTIONS    = 133
	STAFF_REPORTS  = 134
)

var (
	recordTypeMap map[int]string = map[int]string{
		COMMUNICATIONS: "communications",
		CONTRACTS:      "contracts",
		ELECTION_INFO:  "elections_info",
		MINUTES:        "minutes",
		ORDINACES:      "ordinances",
		RESOLUTIONS:    "resolutions",
		STAFF_REPORTS:  "staff_reports",
	}
)

var client = &http.Client{}

func main() {
	ctx := context.Background()

	driveService, err := NewDrive(ctx)

	if err != nil {
		log.Println("Error initializing drive service:", err)
		return
	}

	group, gctx := errgroup.WithContext(ctx)
	tasks := make(chan *Record)

	for i := 0; i < numWorkers; i++ {
		group.Go(func() error {
			for record := range tasks {
				err = transferRecord(gctx, driveService, record)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	fmt.Println("Files:")
	fileIDs, err := driveService.ListIds(ctx)

	// for _, id := range fileIDs {
	// 	err := driveService.Delete(ctx, id)
	// 	if err != nil {
	// 		log.Println("Error fetching records:", err)
	// 		return
	// 	}
	// }

	files, err := driveService.List(ctx)
	b, _ := json.MarshalIndent(files, " ", " ")
	fmt.Println(string(b))

loop:
	for id, _ := range recordTypeMap {
		recordType := recordTypeMap[id]
		records, err := fetchRecords(ctx, driveService, id)
		if err != nil {
			log.Println("Error fetching records:", err)
			return
		}

		folder, err := driveService.FindOrCreateFolder(ctx, recordType)
		if err != nil {
			log.Println("Error fetching records:", err)
			return
		}

		for _, record := range records.Data {
			record.ParentId = folder.Id
			if _, ok := fileIDs[record.ID]; ok {
				continue
			}

			select {
			case <-gctx.Done():
				break loop
			case tasks <- record:
			}
		}
	}

	err = group.Wait()
	if err != nil {
		log.Println("Error encountered: %w", err)
	}

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
func fetchRecords(ctx context.Context, driveService *Drive, queryID int) (*Records, error) {

	var data Records

	// populate each record with the top level DisplayColumns, which
	// hold information on what individual record display column values hold
	// see Records type
	defer func() {
		for _, record := range data.Data {
			record.DisplayColumns = data.DisplayColumns
		}
	}()

	// Fetch records from google drive first
	// If the set of records are too old, refetch from city records site
	var reader io.ReadCloser
	recordsType := recordTypeMap[queryID]

	recordID := fmt.Sprintf("%s:%s:%d", securePrefix, recordsType, queryID)

	file, err := driveService.FindRecord(ctx, recordID)
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
	if diff := time.Now().Sub(created); diff.Hours() < 7*24 {
		reader, err = driveService.DownloadRecord(ctx, recordID)
		if err != nil {
			fmt.Println("Error downloading file: " + recordID)
			return nil, fmt.Errorf("Error downloading file: %s: %w", recordID, err)
		}
	}

	if reader != nil {
		fmt.Println("Fetched cached records")
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
		queryID,
		[]string{},
		0,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling payload: %s: %w", recordID, err)
	}

	fmt.Printf("Fetching records: %s\n", recordTypeMap[queryID])
	req, err := http.NewRequestWithContext(ctx, "POST", "https://records.cityofberkeley.info/PublicAccess/api/CustomQuery/KeywordSearch", bytes.NewReader(b))

	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s: %w", recordID, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request for queryID: %s: %v", queryID, err)
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
	err = driveService.UploadRecord(ctx, &Record{ID: recordID, Name: recordsType + ".json"}, bytes.NewReader(body))
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
	Name                string
	DisplayType         string
	DisplayColumnValues []DisplayColumnValue
	DisplayColumns      []DisplayColumn `json:"-"` // This field will be ignored
	ParentId            string
	Summary             string
}

func (r Record) Properties() map[string]string {
	m := map[string]string{}
	for i, column := range r.DisplayColumns {
		key := propertyKey(column.Heading)
		if key == "" {
			continue
		}
		m[key] = r.DisplayColumnValues[i].Value
	}
	return m
}

func (r Record) DocDate() (time.Time, error) {
	value, rawValue := r.fromDisplay("doc date")
	millis, err := strconv.ParseInt(rawValue, 10, 64)
	if err != nil {
		return time.Parse("6/15/2006", value)
	}
	return time.UnixMilli(millis), nil
}

func (r Record) DocSource() string {
	value, rawValue := r.fromDisplay("doc source")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
}

func (r Record) DocName() string {
	value, rawValue := r.fromDisplay("doc name")
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
	value, rawValue := r.fromDisplay("meeting type")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
}

func (r Record) fromDisplay(name string) (string, string) {
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
