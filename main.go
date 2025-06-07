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
	"os/exec"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const (
	SCOPE = drive.DriveScope

	driveAPIKey = "AIzaSyC9-NgnooctRQ8r4m2_93U7cyKNFylXvio"

	ALL_RECORDS    = 127
	COMMUNICATIONS = 129
	CONTRACTS      = 126
	ELECTION_INFO  = 114
	MINUTES        = 131
	ORDINACES      = 132
	RESOLUTIONS    = 133
	STAFF_REPORTS  = 134
)

var client = &http.Client{}

func main() {
	ctx := context.Background()
	driveService, err := drive.NewService(ctx, option.WithAPIKey(driveAPIKey))

	if err != nil {
		log.Println("Error initializing drive service:", err)
		return
	}

	log.Println("Fetching COMMUNICATIONS records")
	records, err := fetchRecords(ctx, COMMUNICATIONS)
	if err != nil {
		log.Println("Error fetching records :", err)
		return
	}

	record := records.Data[0]
	body, err := fetchDocument(ctx, record.ID)
	if err != nil {
		log.Println("Error fetching document :", err)
		return
	}
	defer body.Close()

	driveFile := &drive.File{Name: record.Name}

	res, err := driveService.Files.Create(driveFile).Do()
	if err != nil {
		log.Fatalf("Unable to upload file: %v", err)
	}
	log.Printf("File uploaded successfully. File ID: %v", res.Id)

	// b, err := json.MarshalIndent(records, " ", " ")
	// log.Println(string(b))

}

func fetchRecords(ctx context.Context, queryID int) (*Records, error) {

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
		return nil, err
	}
	reader := bytes.NewReader(b)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://records.cityofberkeley.info/PublicAccess/api/CustomQuery/KeywordSearch", reader)

	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil, err
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

	// Unmarshal the JSON response into the ResponseData struct
	var data Records
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return nil, err
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
		fmt.Println("Document fetch request failed with status code:  %d. Body: %s", resp.StatusCode, string(body))
		return nil, err
	}

	return resp.Body, nil
}

func parsePdf(ctx context.Context, id string) (string, error) {

	id = url.QueryEscape(id)
	url := "https://records.cityofberkeley.info/PublicAccess/api/Document/" + id + "/"
	log.Println("Fetching url: " + url)
	resp, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP error status: %v", resp.Status)
	}
	defer resp.Body.Close()

	if err != nil {
		return "", fmt.Errorf("Error reading response body:%v", resp.Status)
	}

	if err != nil {
		return "", err
	}

	params := []string{
		"-", // Read from stdin
		"-", // Write to stdout
	}

	cmd := exec.Command("pdftotext", params...)
	cmd.Stdin = resp.Body

	var out bytes.Buffer
	cmd.Stdout = &out

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("error executing pdftotext binary: %w", err)
	}

	body := strings.TrimSpace(string(out.Bytes()))
	return body, err

}

type Record struct {
	ID                  string
	Name                string
	DisplayType         string
	DisplayColumnValues []DisplayColumnValue
}

type Records struct {
	Data           []Record
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
