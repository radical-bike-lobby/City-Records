package main

import (
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	drive "google.golang.org/api/drive/v3"
)

const (
	dateFormat = "1/_2/2006"
)

type DriveFolderID string

type DriveFileMap struct {
	FileMap map[string][]*drive.File
}

func NewDriveFileMap(files []*drive.File) *DriveFileMap {
	fileMap := make(map[string][]*drive.File)
	for _, file := range files {

		hash := file.Properties["hash"]
		record_id := file.Properties["record_id"]
		if hash == "" || record_id == "" { // files without a hash should not be indexed
			continue
		}

		fileMap[record_id] = append(fileMap[record_id], file)
	}

	return &DriveFileMap{
		FileMap: fileMap,
	}
}

func (d *DriveFileMap) Get(record *Record) (files []*drive.File, err error) {
	return d.FileMap[record.ID], nil
}

type Record struct {
	ID                  string               `json:"ID"`
	Type                RecordType           `json:"-"` // This field will be ignored
	Name                string               `json:"Name"`
	DisplayType         string               `json:"DisplayType"`
	DisplayColumnValues []DisplayColumnValue `json:"DisplayColumnValues"`
	DisplayColumns      []DisplayColumn      `json:"-"` // This field will be ignored
	ParentId            string               `json:"-"` // This field will not be persisted
	Summary             string               `json:"Summary"`
	Persisted           bool                 `json:"Persisted"` // indicates if the record was persisted successfully to Drive
}

type Records struct {
	ID             string          `json:"ID"`
	Name           string          `json:"Name"`
	Data           []*Record       `json:"Data"`
	Truncated      bool            `json:"Truncated"`
	DisplayColumns []DisplayColumn `json:"DisplayColumns"`
}

type DisplayColumn struct {
	Heading  string
	DataType string
}

type DisplayColumnValue struct {
	Value    string
	RawValue string
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

func (r *Record) Equal(other *Record) bool {
	if r == nil || other == nil {
		return false
	}

	if r.ID != "" && r.ID == other.ID {
		return true
	}

	return false

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
	if value == "" || value == "null" {
		return rawValue
	}
	return value
}

func (r Record) DocNumber() string {
	value, rawValue := r.fromDisplayHeading("doc number")
	if value == "" || value == "null" {
		return rawValue
	}
	return value
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
	name := r.DocName()
	meeting := r.MeetingType()
	if name == "" {
		name = r.DocNumber()
	}
	if name == "" {
		if name = r.DocType(); name != "" {
			if meeting != "" {
				name = name + " - " + meeting
			}
		}
	}
	if name == "" {
		name = r.Name
	}

	name = strings.TrimSpace(html.UnescapeString(name))
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
