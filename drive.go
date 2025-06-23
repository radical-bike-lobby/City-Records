package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	file = "/Users/navgattu/Downloads/prismatic-fact-96623-19438c8ca24b.json"
)

type Drive struct {
	service *drive.Service
}

// NewDrive creates a new drive service
func NewDrive(ctx context.Context) (*Drive, error) {

	file, err := os.Open(file)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		log.Fatalf("Error reading file: %v", err)
	}
	driveService, err := drive.NewService(ctx, option.WithCredentialsJSON(content))

	if err != nil {
		return nil, err
	}
	return &Drive{
		service: driveService,
	}, nil
}

// drive functions

// about returns info about the drive account
func (d *Drive) About(ctx context.Context) error {
	about, err := d.service.About.Get().Fields("*").Do()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(about, " ", "")
	if err != nil {
		return err
	}

	fmt.Println(string(b))
	return nil
}

func (d *Drive) FindRecord(ctx context.Context, id string) (*drive.File, error) {
	query := "properties has { key='record_id' and value='" + id + "'}"
	files, err := d.List(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(files.Files) > 0 {
		return files.Files[0], nil
	}

	return nil, nil
}

func (d *Drive) FindOrCreateFolder(ctx context.Context, path string) (*drive.File, error) {
	query := "name='" + path + "' and mimeType='application/vnd.google-apps.folder'"
	files, err := d.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("Error listing files for query: %s: %w", query, err)
	}
	if len(files.Files) == 0 {
		newFolder := &drive.File{
			Name:     path,
			MimeType: "application/vnd.google-apps.folder",
		}
		createdFolder, err := d.service.Files.Create(newFolder).Do()
		if err != nil {
			return nil, fmt.Errorf("Error listing creating folder for query: %s: %w", query, err)
		}
		return createdFolder, nil
	}
	return files.Files[0], nil
}

func (d *Drive) Delete(ctx context.Context, id string) error {
	log.Printf("Deleting file with ID: : %s", id)
	return d.service.Files.Delete(id).Do()
}

func (d *Drive) List(ctx context.Context, filter ...string) (*drive.FileList, error) {

	query := d.service.Files.List().PageSize(1000).
		Fields("nextPageToken, files(id, name, createdTime, modifiedTime, properties, appProperties, parents, size)")

	if len(filter) > 0 {
		query = query.Q(filter[0])
	}

	return query.Do()
}

func (d *Drive) ListIds(ctx context.Context) (map[string]string, error) {

	fileIDs := map[string]string{}
	pageToken := ""

	for {
		call := d.service.Files.List().Fields("nextPageToken, files(id, name, properties)")
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("Error listing files: %w", err)
		}

		for _, f := range r.Files {
			id, ok := f.Properties["record_id"]
			if !ok {
				fmt.Printf("Record id not found for file: %s: %s\n", f.Name, f.Id)
				continue
			}

			fileIDs[id] = f.Id
		}

		if r.NextPageToken == "" {
			break // No more pages
		}
		pageToken = r.NextPageToken
	}

	return fileIDs, nil
}

// Download downloads a file from google drive by 'record_id' field. This is distinct from the
// google assigned document id. It is derived from the original document id from the city records.
func (d *Drive) DownloadRecord(ctx context.Context, record_id string) (io.ReadCloser, error) {
	file, err := d.FindRecord(ctx, record_id)
	if err != nil {
		return nil, err
	}

	if file == nil {
		return nil, nil
	}

	resp, err := d.service.Files.Get(file.Id).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	return resp.Body, nil
}

func (d *Drive) UploadRecord(ctx context.Context, record *Record, reader io.Reader) error {

	if file, err := d.FindRecord(ctx, record.ID); err != nil {
		return err
	} else if file != nil {
		log.Printf("File already exists with ID: %s", record.ID)
		return nil
	}

	name := record.DocName()
	date, err := record.DocDate()
	if err != nil {
		date = time.Now()
		b, _ := json.MarshalIndent(record, " ", " ")
		fmt.Println(string(b))
		fmt.Printf("Error fetching created data from record: %s, err: %w", record.ID, err)
	}

	created := date.Format(time.RFC3339)
	metadata := &drive.File{
		Name:         name,
		CreatedTime:  created,
		ModifiedTime: created,
	}

	metadata.Properties = record.Properties()
	metadata.Properties["record_id"] = record.ID

	if record.ParentId != "" {
		metadata.Parents = []string{record.ParentId}
	}

	// log.Printf("Uploading file with ID: : %s", record.ID)
	res, err := d.service.Files.Create(metadata).Media(reader).Do()
	if err != nil {
		return fmt.Errorf("Error creating file: %s: %w", record.ID, err)
	}

	b, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("Error marshalling body: %s: %w", record.ID, err)
	}
	log.Printf("File uploaded successfully. Resp: %v", string(b))
	return nil
}
