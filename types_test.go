package main

import (
	"slices"
	"testing"
)

func TestRecords_Merge(t *testing.T) {

	// Test case 5: Merging with new records including DisplayColumnValues
	recordsI := &Records{
		Data: []*Record{
			{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
					{Value: "Field2", RawValue: "Value2"},
				},
			},
			{
				ID:   "2",
				Name: "Record B",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field3", RawValue: "Value3"},
				},
			},
			{
				ID:   "3",
				Name: "Record C",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
					{Value: "Field3", RawValue: "Value3"},
				},
			},
		},
	}
	recordsJ := &Records{
		Data: []*Record{
			{
				ID:   "2",
				Name: "Record B",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field3", RawValue: "Value3"},
				},
			},
			{
				ID:   "3",
				Name: "Record C",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
					{Value: "Field3", RawValue: "Value3"},
				},
			},
			{
				ID:   "4",
				Name: "Record D",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field4", RawValue: "Value4"},
					{Value: "Field5", RawValue: "Value5"},
				},
			},
			{
				ID:   "2 duplicate",
				Name: "Record B",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field3", RawValue: "Value3"},
				},
			},
		},
	}
	newRecords := recordsI.Merge(recordsJ)

	expected5 := []*Record{
		{
			ID:   "1",
			Name: "Record A",
			DisplayColumnValues: []DisplayColumnValue{
				{Value: "Field1", RawValue: "Value1"},
				{Value: "Field2", RawValue: "Value2"},
			},
		},
		{
			ID:   "2",
			Name: "Record B",
			DisplayColumnValues: []DisplayColumnValue{
				{Value: "Field3", RawValue: "Value3"},
			},
		},
		{
			ID:   "3",
			Name: "Record C",
			DisplayColumnValues: []DisplayColumnValue{
				{Value: "Field4", RawValue: "Value4"},
				{Value: "Field5", RawValue: "Value5"},
			},
		},
		{
			ID:   "4",
			Name: "Record D",
			DisplayColumnValues: []DisplayColumnValue{
				{Value: "Field4", RawValue: "Value4"},
				{Value: "Field5", RawValue: "Value5"},
			},
		},
	}

	expectedNew5 := []*Record{
		{
			ID:   "4",
			Name: "Record D",
			DisplayColumnValues: []DisplayColumnValue{
				{Value: "Field4", RawValue: "Value4"},
				{Value: "Field5", RawValue: "Value5"},
			},
		},
	}

	// Sort both recordsI.Data and expected5 if the order of DisplayColumnValues matters
	// within the slice, or update the EqualFunc for slices of DisplayColumnValue.
	// For simplicity, we assume DisplayColumnValues order does not matter in this example.
	if !slices.EqualFunc(recordsI.Data, expected5, func(a, b *Record) bool {
		return a.Equal(b) // Use the updated Equal method for Record
	}) {
		t.Errorf("TestRecords_Merge : Expected %+v, got %+v", expected5, recordsI.Data)
	}

	if !slices.EqualFunc(newRecords, expectedNew5, func(a, b *Record) bool {
		return a.Equal(b) // Use the updated Equal method for Record
	}) {
		t.Errorf("TestRecords_Merge newRecords: Expected %+v, got %+v", expectedNew5, newRecords)
	}
}

func TestRecord_Equal(t *testing.T) {
	tests := []struct {
		name     string
		record   *Record
		other    *Record
		expected bool
	}{
		{
			name:     "Case 1: Both nil - Should be false",
			record:   nil,
			other:    nil,
			expected: false,
		},
		{
			name:     "Case 2: One nil - Should be false",
			record:   &Record{ID: "1"},
			other:    nil,
			expected: false,
		},
		{
			name:     "Case 3: Other nil - Should be false",
			record:   nil,
			other:    &Record{ID: "1"},
			expected: false,
		},
		{
			name:     "Case 4: IDs match - Should be true",
			record:   &Record{ID: "1", Name: "Record A"},
			other:    &Record{ID: "1", Name: "Record B"}, // Name differs, but ID matches
			expected: true,
		},
		{
			name: "Case 5: IDs don't match, Names match, DisplayColumnValues match - Should be true",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			other: &Record{
				ID:   "2", // Different ID
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			expected: true,
		},
		{
			name: "Case 6: IDs don't match, Names match, DisplayColumnValues differ - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value2"}, // Different value
				},
			},
			expected: false,
		},
		{
			name: "Case 7: IDs don't match, Names differ, DisplayColumnValues match - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record B", // Different Name
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			expected: false,
		},
		{
			name: "Case 8: IDs don't match, Names and DisplayColumnValues both differ - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record B",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field2", RawValue: "Value2"},
				},
			},
			expected: false,
		},
		{
			name: "Case 9: Empty DisplayColumnValues - Should be true (Names match)",
			record: &Record{
				ID:                  "1",
				Name:                "Record A",
				DisplayColumnValues: []DisplayColumnValue{},
			},
			other: &Record{
				ID:                  "2", // Different ID
				Name:                "Record A",
				DisplayColumnValues: []DisplayColumnValue{},
			},
			expected: true,
		},
		{
			name: "Case 10: Empty DisplayColumnValues - Should be false (Names differ)",
			record: &Record{
				ID:                  "1",
				Name:                "Record A",
				DisplayColumnValues: []DisplayColumnValue{},
			},
			other: &Record{
				ID:                  "2",
				Name:                "Record B", // Different Name
				DisplayColumnValues: []DisplayColumnValue{},
			},
			expected: false,
		},
		{
			name: "Case 11: DisplayColumnValues length mismatch - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "Field1", RawValue: "Value1"},
				},
			},
			other: &Record{
				ID:                  "2",
				Name:                "Record A",
				DisplayColumnValues: []DisplayColumnValue{}, // Length mismatch
			},
			expected: false,
		},
		{
			name: "Case 12: Complex DisplayColumnValues match - Should be true",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2", RawValue: "V2"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2", RawValue: "V2"},
				},
			},
			expected: true,
		},
		{
			name: "Case 13: Complex DisplayColumnValues differ (value) - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2", RawValue: "V2"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2", RawValue: "V2a"}, // Different value
				},
			},
			expected: false,
		},
		{
			name: "Case 14: Complex DisplayColumnValues differ (name) - Should be false",
			record: &Record{
				ID:   "1",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2", RawValue: "V2"},
				},
			},
			other: &Record{
				ID:   "2",
				Name: "Record A",
				DisplayColumnValues: []DisplayColumnValue{
					{Value: "F1", RawValue: "V1"},
					{Value: "F2a", RawValue: "V2"}, // Different name
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.record.Equal(tt.other)
			if actual != tt.expected {
				t.Errorf("Record.Equal() = %v, want %v", actual, tt.expected)
			}
		})
	}
}
