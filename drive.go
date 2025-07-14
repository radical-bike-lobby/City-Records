package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var credentialsFile = os.Getenv("CREDENTIALS_FILE")
var userToImpersonate = os.Getenv("IMPERSONATE_SUBJECT")

const (
	fields = "id, name, createdTime, modifiedTime, properties, parents, size, sha256Checksum"
)

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
		drive.DriveScope,
		drive.DriveAppdataScope,
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

	if len(files) > 0 {
		return files[0], nil
	}

	return nil, nil
}
func (d *Drive) FindRecord(ctx context.Context, id string) (*drive.File, error) {
	query := "properties has { key='record_id' and value='" + id + "'}"
	files, err := d.List(ctx, query)
	if err != nil {
		return nil, err
	}

	if len(files) > 0 {
		return files[0], nil
	}

	return nil, nil
}

func (d *Drive) FindOrCreateFolder(ctx context.Context, dirname, parentID string) (*drive.File, error) {
	query := "name='" + dirname + "' and mimeType='application/vnd.google-apps.folder'"
	files, err := d.List(ctx, query, fmt.Sprintf("'%s' in parents", parentID))
	if err != nil {
		return nil, fmt.Errorf("Error listing files for query: %s: %w", query, err)
	}
	if len(files) == 0 {
		newFolder := &drive.File{
			Name:     dirname,
			MimeType: "application/vnd.google-apps.folder",
			Parents:  []string{d.driveID},
		}
		createdFolder, err := d.service.Files.Create(newFolder).SupportsAllDrives(true).Do()
		if err != nil {
			return nil, fmt.Errorf("Error listing creating folder for query: %s: %w", query, err)
		}
		return createdFolder, nil
	}
	return files[0], nil
}

func (d *Drive) List(ctx context.Context, query ...string) (files []*drive.File, err error) {

	pageToken := ""
	call := d.service.Files.List().PageSize(1000).
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("nextPageToken, files(" + fields + ")")
	if len(query) > 0 {
		call = call.Q(query[0])
	}

	for {
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		list, err := call.Do()

		if err != nil {
			return nil, err
		}
		files = append(files, list.Files...)

		pageToken = list.NextPageToken
		if pageToken == "" {
			break // No more pages
		}

	}

	return files, err
}

func (d *Drive) PrintAllFiles(ctx context.Context) error {
	fmt.Println("Printing files.")
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
	return d.service.Files.Delete(id).
		SupportsAllDrives(true).
		Do()
}

func (d *Drive) DeleteAllFiles(ctx context.Context) error {

	log.Printf("Start deleting all files")
	files, err := d.List(ctx)
	if err != nil {
		return err
	}

	log.Printf("Deleting %d files", len(files))
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

	for _, file := range files {
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

func (d *Drive) UploadFile(ctx context.Context, metadata *drive.File, reader io.Reader) (*drive.File, error) {
	// log.Printf("Uploading file with ID: : %s", record.ID)
	if len(metadata.Parents) == 0 {
		metadata.Parents = []string{d.driveID} // ensure root directory is always the root parent id if unset
	}
	res, err := d.service.Files.Create(metadata).
		Media(reader).
		SupportsAllDrives(true).
		Fields(fields).
		Do()

	if err != nil {
		b, _ := json.MarshalIndent(metadata, " ", " ")
		return nil, fmt.Errorf("Error creating file: %s: %w", string(b), err)
	}

	b, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling body for file: %s: %w", metadata.Name, err)
	}
	log.Printf("File uploaded successfully. Resp: %v", string(b))
	return res, nil
}

func (d *Drive) UploadRecord(ctx context.Context, record *Record, reader io.Reader, length int64) error {

	file, err := d.FindRecord(ctx, record.ID)

	if err != nil {
		return err
	} else if file != nil {
		log.Printf("File already exists with ID: %s and name %s", record.ID, record.Name)
		return nil
	}

	metadata, err := record.ToDriveFile()
	if err != nil {
		return err
	}

	// compute sha256 on stream
	hasher := sha256.New()
	teeReader := io.TeeReader(reader, hasher)

	// upload file to temporary folder
	file, err = d.UploadFile(ctx, metadata, teeReader)
	if err != nil {
		return err
	}

	// delete temp file if not successful
	successful := false
	defer func() {
		if successful {
			return
		}
		derr := d.Delete(ctx, file.Id)
		if derr != nil {
			fmt.Printf("Error cleaning up temp file: %s", derr.Error())
		}
	}()

	sum := hasher.Sum(nil)
	hash := hex.EncodeToString(sum)

	// dedupe
	files, err := d.List(ctx, fmt.Sprintf("properties has { key='hash' and value='%s' }", hash))
	if err != nil {
		return err
	}

	if len(files) > 0 {
		log.Printf("File already exists with ID: %s and hash: %s", file.Id, hash)
		return nil
	}

	// move file to final destination and update hash field
	_, err = d.service.Files.Update(file.Id, &drive.File{
		ModifiedTime: file.ModifiedTime,
		Properties: map[string]string{
			"hash": hash,
		},
	}).SupportsAllDrives(true).Do()

	if err != nil {
		return fmt.Errorf("Erroring moving file from appDataFolder folder: %w", err)
	}

	successful = true
	return nil
}
