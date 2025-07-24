package main

import (
	"encoding/json"
	"fmt"
	"html"
	"slices"
	"strconv"
	"strings"
	"time"

	drive "google.golang.org/api/drive/v3"
)

const (
	dateFormat = "1/_2/2006"
)

type DriveFolderID string

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
	DriveFileID    string          `json:"-"`
}

func (r *Records) Sort() {
	slices.SortFunc(r.Data, func(i, j *Record) int {
		first, _ := i.DocDate()
		second, _ := j.DocDate()
		return second.Compare(first)
	})
}

// Merge merges the records found in the 'other' with those found in r
// New records found in other, not present in r, are append to r's records (Data field)
func (r *Records) Merge(other *Records) (newRecords []*Record) {
	for _, candidate := range other.Data {
		if !slices.ContainsFunc(r.Data, func(record *Record) bool {
			return record.Equal(candidate)
		}) {
			newRecords = append(newRecords, candidate)
			r.Data = append(r.Data, candidate)
		}
	}
	return newRecords
}

type DisplayColumn struct {
	Heading  string
	DataType string
}

type DisplayColumnValue struct {
	Value    string
	RawValue string
}

func (d DisplayColumnValue) Equal(other DisplayColumnValue) bool {
	return d.Value == other.Value && d.RawValue == other.RawValue
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

	colummsEqual := slices.EqualFunc(r.DisplayColumnValues, other.DisplayColumnValues, func(a, b DisplayColumnValue) bool {
		return a.Equal(b)
	})

	if r.Name == other.Name && colummsEqual {
		return true
	}

	return false

}

// Merge merges the fields from 'fixed' into r
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

func (r *Record) String() string {

	var b strings.Builder // Use strings.Builder for efficient string concatenation

	b.WriteString(fmt.Sprintf("Record{ID: %s, Name: %s", r.ID, r.Name))

	if r.DisplayType != "" {
		b.WriteString(fmt.Sprintf(", DisplayType: %s", r.DisplayType))
	}

	if len(r.DisplayColumnValues) > 0 {
		b.WriteString(", DisplayColumnValues: [")
		for i, dcv := range r.DisplayColumnValues {
			b.WriteString(fmt.Sprintf("%s: %s", dcv.Value, dcv.RawValue))
			if i < len(r.DisplayColumnValues)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString("]")
	}

	if r.Summary != "" {
		b.WriteString(fmt.Sprintf(", Summary: %q", r.Summary)) // Using %q for quoted string
	}

	b.WriteString(fmt.Sprintf(", Persisted: %t}", r.Persisted))

	return b.String()
}
