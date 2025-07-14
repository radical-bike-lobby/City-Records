package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"

	drive "google.golang.org/api/drive/v3"
)

const (
	dateFormat = "1/_2/2006"
)

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

func NewRecord(name, parent string, date time.Time) *Record {
	return &Record{
		Name:     name,
		ParentId: parent,
		DisplayColumns: []DisplayColumn{
			{
				DataType: "date",
			},
		},
		DisplayColumnValues: []DisplayColumnValue{
			{
				Value: date.Format(dateFormat),
			},
		},
	}
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
		return time.Time{}, fmt.Errorf("Could not find date column")
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

func (r Record) DocType() string {
	value, rawValue := r.fromDisplayHeading("document type")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
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

func (r Record) ToDriveFile() (*drive.File, error) {
	name := strings.TrimSpace(r.DocName())
	name = html.UnescapeString(name)
	date, err := r.DocDate()

	if err != nil {
		date = time.Now()
		b, _ := json.MarshalIndent(r, " ", " ")
		fmt.Println(string(b))
		b, _ = json.MarshalIndent(r.DisplayColumns, " ", " ")
		fmt.Println(string(b))
		return nil, fmt.Errorf("Error fetching CreatedDate from record: %s, err: %w", r.ID, err)

	}

	created := date.Format(time.RFC3339)
	metadata := &drive.File{
		Name:         name,
		CreatedTime:  created,
		ModifiedTime: created,
	}

	metadata.Properties = map[string]string{}
	for key, value := range r.Properties() {
		fKey, fValue := propertyKeyValue(key, value)
		metadata.Properties[fKey] = fValue
	}
	metadata.Properties["record_id"] = r.ID
	metadata.Properties["record_type"] = fmt.Sprintf("%d", r.Type)

	if r.ParentId != "" {
		metadata.Parents = []string{r.ParentId}
	}

	return metadata, nil
}

type DriveFolderID string

type DriveFileMap struct {
	FileMap map[DriveFolderID]map[string][]*drive.File
}

func NewDriveFileMap(files []*drive.File) *DriveFileMap {
	fileMap := make(map[DriveFolderID]map[string][]*drive.File)
	for _, file := range files {

		if file.Properties == nil || file.Properties["hash"] == "" { // files without a hash should not be indexed
			continue
		}
		createdTime, err := time.Parse(time.RFC3339, file.CreatedTime)
		if err != nil {
			log.Fatalf("Invalid date time for file: %s", createdTime)
		}
		date := createdTime.Format(dateFormat)
		for _, parent := range file.Parents {
			dayMap, ok := fileMap[DriveFolderID(parent)]
			if !ok {
				dayMap = map[string][]*drive.File{}
				fileMap[DriveFolderID(parent)] = dayMap
			}

			dayMap[date] = append(dayMap[date], file)
		}
	}

	return &DriveFileMap{
		FileMap: fileMap,
	}
}

func (d *DriveFileMap) Get(record *Record) (files []*drive.File, err error) {

	dt, err := record.DocDate()
	if err != nil {
		return nil, err
	}
	name, folder, date := record.DocName(), record.ParentId, dt.Format(dateFormat)
	name = strings.ToLower(strings.TrimSpace(name))
	if dayMap, ok := d.FileMap[DriveFolderID(folder)]; ok {
		for _, file := range dayMap[date] {
			filename := strings.ToLower(strings.TrimSpace(file.Name))
			recordName := strings.ToLower(strings.TrimSpace(record.DocName()))
			recordID := file.Properties["record_id"]
			if recordID == record.ID || recordName == filename {
				files = append(files, file)
			}
		}
	}
	return files, nil
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
