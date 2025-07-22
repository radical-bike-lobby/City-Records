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
	"strings"

	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var credentialsFile = os.Getenv("CREDENTIALS_FILE")
var userToImpersonate = os.Getenv("IMPERSONATE_SUBJECT")

const (
	fields          = "id, name, createdTime, modifiedTime, properties, parents, size, sha256Checksum"
	appDataFolderID = "appDataFolder"

	folderMime = "application/vnd.google-apps.folder"
)

type Drive struct {
	service *drive.Service
	driveID string
	sg      *singleflight.Group
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
		sg:      &singleflight.Group{},
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

// FindOrCreateFolder locates or creates a specified folder withing a parent folder
func (d *Drive) FindOrCreateFolder(ctx context.Context, dirname, parentID string) (file *drive.File, err error) {
	dirname = strings.TrimSpace(dirname)
	query := fmt.Sprintf("name='%s' and ('%s' in parents) and mimeType='%s'", dirname, parentID, folderMime)

	data, err, _ := d.sg.Do(query, func() (interface{}, error) {
		files, err := d.List(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("Error listing files for query: %s: %w", query, err)
		}
		if len(files) == 0 {

			log.Printf("Could not find dir: %s. Creating. Query: %s", dirname, query)
			newFolder := &drive.File{
				Name:     dirname,
				MimeType: folderMime,
				Parents:  []string{parentID},
			}
			createdFolder, err := d.service.Files.Create(newFolder).SupportsAllDrives(true).Do()
			if err != nil {
				return nil, fmt.Errorf("Error listing creating folder for query: %s: %w", query, err)
			}
			return createdFolder, nil
		}
		return files[0], nil
	})

	if err != nil {
		return nil, err
	}
	return data.(*drive.File), nil
}

// ListSpace lists all files within the "drive" space
func (d *Drive) List(ctx context.Context, query ...string) (files []*drive.File, err error) {
	return d.ListSpace(ctx, "drive", nil, query...)
}

// ListSpace lists all files within a specified space ("appDataFolder", "drive", etc...)
func (d *Drive) ListSpace(ctx context.Context, space string, progressCallback func(int) error, query ...string) (files []*drive.File, err error) {

	pageToken := ""
	call := d.service.Files.List().Spaces(space).PageSize(1000).
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("nextPageToken, files(" + fields + ")")
		// OrderBy("modifiedDate desc,name")
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
		if progressCallback != nil {
			err = progressCallback(len(list.Files))
			if err != nil {
				return nil, fmt.Errorf("Error incrementing progress bar: %w", err)
			}
		}

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

func (d *Drive) DeleteFile(ctx context.Context, id string) error {
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
				err := d.DeleteFile(gctx, file.Id)
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

// DownloadFile downloads the specified Drive file
func (d *Drive) DownloadFile(ctx context.Context, id string) (*drive.File, io.ReadCloser, error) {
	// fmt.Println("Downloading file: " + id)

	file, err := d.service.Files.Get(id).Do()
	if err != nil {
		return nil, nil, err
	}

	resp, err := d.service.Files.Get(id).Download()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download file: %w", err)
	}
	return file, resp.Body, nil
}

func (d *Drive) MoveFile(ctx context.Context, file *drive.File, folderID string) error {
	_, err := d.service.Files.Update(file.Id, &drive.File{
		ModifiedTime: file.ModifiedTime,
	}).
		SupportsAllDrives(true).
		RemoveParents(strings.Join(file.Parents, ",")).
		AddParents(folderID).
		Do()

	return err
}

func (d *Drive) UpdateFile(ctx context.Context, fileID string, body io.Reader) error {
	_, err := d.service.Files.Update(fileID, &drive.File{}).
		SupportsAllDrives(true).
		Media(body).
		Do()

	return err
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

	// b, err := json.Marshal(res)
	// if err != nil {
	// 	return nil, fmt.Errorf("Error marshalling body for file: %s: %w", metadata.Name, err)
	// }
	// log.Printf("File uploaded successfully. Resp: %v", string(b))
	return res, nil
}

func (d *Drive) UploadRecord(ctx context.Context, record *Record, reader io.Reader, length int64) error {

	file, err := d.FindRecord(ctx, record.ID)

	if err != nil {
		return err
	} else if file != nil {
		// log.Printf("File already exists with ID: %s and name %s", record.ID, record.Name)
		return nil
	}

	metadata, err := record.ToDriveFile()
	if err != nil {
		return err
	}

	if docSource := record.DocSource(); docSource != "" {
		docSource = cases.Title(language.English).String(docSource)
		folder, err := d.FindOrCreateFolder(ctx, docSource, record.ParentId)
		if err != nil {
			return err
		}
		metadata.Parents = []string{folder.Id}
	}

	// compute sha256 on stream
	hasher := sha256.New()
	teeReader := io.TeeReader(reader, hasher)

	// upload temp file
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
		derr := d.DeleteFile(ctx, file.Id)
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
		// log.Printf("File already exists with ID: %s and hash: %s", file.Id, hash)
		return nil
	}

	// update hash field of file
	_, err = d.service.Files.Update(file.Id, &drive.File{
		ModifiedTime: file.ModifiedTime,
		Properties: map[string]string{
			"hash": hash,
		},
	}).SupportsAllDrives(true).Do()

	if err != nil {
		return fmt.Errorf("Erroring updating file: %s", err.Error())
	}

	successful = true
	return nil
}
