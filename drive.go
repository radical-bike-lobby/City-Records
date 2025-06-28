package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var credentialsFile = os.Getenv("CREDENTIALS_FILE")
var userToImpersonate = os.Getenv("IMPERSONATE_SUBJECT")

type Drive struct {
	service *drive.Service
	driveID string
}

// NewDrive creates a new drive service
func NewDrive(ctx context.Context, driveID string) (*Drive, error) {

	file, err := os.Open(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("Error opening file: %s, %v", credentialsFile, err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Error reading file: %s, %v", credentialsFile, err)

	}

	jwtConfig, err := google.JWTConfigFromJSON(
		[]byte(content),
		drive.DriveScope, // Or other necessary Drive API scopes
	)
	if err != nil {
		log.Fatalf("Error creating JWT config: %v", err)
	}

	jwtConfig.Subject = userToImpersonate
	// Create a new HTTP client with the configured JWT credentials
	client := jwtConfig.Client(ctx)

	driveService, err := drive.NewService(ctx, option.WithHTTPClient(client))

	if err != nil {
		return nil, err
	}
	return &Drive{
		service: driveService,
		driveID: driveID,
	}, nil
}

// drive functions

// about returns info about the drive account
func (d *Drive) About(ctx context.Context) error {
	about, err := d.service.About.Get().Fields("storageQuota").Do()
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

func (d *Drive) FindFileByProperty(ctx context.Context, key string, value string) (*drive.File, error) {
	query := fmt.Sprintf("properties has { key='%s' and value='%s' }", key, value)
	files, err := d.List(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(files.Files) > 0 {
		return files.Files[0], nil
	}

	return nil, nil
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
			Parents:  []string{d.driveID},
		}
		createdFolder, err := d.service.Files.Create(newFolder).SupportsAllDrives(true).Do()
		if err != nil {
			return nil, fmt.Errorf("Error listing creating folder for query: %s: %w", query, err)
		}
		return createdFolder, nil
	}
	return files.Files[0], nil
}

func (d *Drive) List(ctx context.Context, filter ...string) (*drive.FileList, error) {

	query := d.service.Files.List().PageSize(1000).
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("nextPageToken, files(id, name, createdTime, modifiedTime, properties, appProperties, parents, size)")

	if len(filter) > 0 {
		query = query.Q(filter[0])
	}

	return query.Do()
}

func (d *Drive) PrintAllFiles(ctx context.Context) error {
	files, err := d.List(ctx)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(files, " ", " ")
	fmt.Println(string(b))
	return nil
}

func (d *Drive) Delete(ctx context.Context, id string) error {
	log.Printf("Deleting file with ID: : %s", id)
	return d.service.Files.Delete(id).Do()
}

func (d *Drive) DeleteAllFiles(ctx context.Context) error {

	log.Printf("Start deleting all files")
	files, err := d.List(ctx)
	if err != nil {
		return err
	}

	log.Printf("Deleting %d files", len(files.Files))
	group, gctx := errgroup.WithContext(ctx)
	tasks := make(chan *drive.File)

	for i := 0; i < 50; i++ {
		group.Go(func() error {
			for file := range tasks {
				err := d.Delete(gctx, file.Id)
				if err != nil {
					return fmt.Errorf("Error deleting file: %s. %w", file.Name, err)
				}
			}
			return nil
		})
	}

	for _, file := range files.Files {
		if file.Id == d.driveID { // ignore parent ID
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case tasks <- file:
		}

	}
	close(tasks)
	return group.Wait()
}

// ListRecordIds returns a map of Record ids to Drive file ids
func (d *Drive) ListRecordIds(ctx context.Context) (map[string]string, error) {

	fileIDs := map[string]string{}
	pageToken := ""

	for {
		call := d.service.Files.List().
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true).
			Fields("nextPageToken, files(id, name, properties)")
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

// DownloadFile downloads the specified Drive file
func (d *Drive) DownloadFile(ctx context.Context, id string) (io.ReadCloser, error) {
	resp, err := d.service.Files.Get(id).Download()
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	return resp.Body, nil
}

func (d *Drive) UploadFile(ctx context.Context, metadata *drive.File, reader io.Reader) error {
	// log.Printf("Uploading file with ID: : %s", record.ID)
	if len(metadata.Parents) == 0 {
		metadata.Parents = []string{d.driveID} // ensure root directory is always the root parent id if unset
	}
	res, err := d.service.Files.Create(metadata).Media(reader).SupportsAllDrives(true).Do()
	if err != nil {
		b, _ := json.MarshalIndent(metadata, " ", " ")
		return fmt.Errorf("Error creating file: %s: %w", string(b), err)
	}

	b, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("Error marshalling body for file: %s: %w", metadata.Name, err)
	}
	log.Printf("File uploaded successfully. Resp: %v", string(b))
	return nil
}

func (d *Drive) UploadRecord(ctx context.Context, record *Record, reader io.Reader) error {

	log.Println("Uploading record: ", record.ID)
	if file, err := d.FindRecord(ctx, record.ID); err != nil {
		return err
	} else if file != nil {
		log.Printf("File already exists with ID: %s", record.ID)
		return nil
	}

	name := strings.TrimSpace(record.DocName())
	date, err := record.DocDate()

	if err != nil {
		date = time.Now()
		b, _ := json.MarshalIndent(record, " ", " ")
		fmt.Println(string(b))
		b, _ = json.MarshalIndent(record.DisplayColumns, " ", " ")
		fmt.Println(string(b))
		return fmt.Errorf("Error fetching CreatedDate from record: %s, err: %w", record.ID, err)

	}

	created := date.Format(time.RFC3339)
	metadata := &drive.File{
		Name:         name,
		CreatedTime:  created,
		ModifiedTime: created,
	}

	metadata.Properties = map[string]string{}
	for key, value := range record.Properties() {
		fKey, fValue := propertyKeyValue(key, value)
		metadata.Properties[fKey] = fValue
	}
	metadata.Properties["record_id"] = record.ID
	metadata.Properties["record_type"] = fmt.Sprintf("%d", record.Type)

	if record.ParentId != "" {
		metadata.Parents = []string{record.ParentId}
	}

	return d.UploadFile(ctx, metadata, reader)
}
