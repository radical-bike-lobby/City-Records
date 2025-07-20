package main

/*var propertiesFunc = func(n string) map[string]string {
	return map[string]string{
		"hash": hex.EncodeToString(sha256.New().Sum([]byte(n))),
	}
}

func TestNewDriveFileMap(t *testing.T) {
	// Sample drive.File objects for testing
	now := time.Now().Format(time.RFC3339)
	date := time.Now().Format(dateFormat)
	file1 := &drive.File{Id: "file1", Name: "document.docx", Parents: []string{"folder1"}, CreatedTime: now, Properties: propertiesFunc("file1")}
	file2 := &drive.File{Id: "file2", Name: "image.png", Parents: []string{"folder1", "folder2"}, CreatedTime: now, Properties: propertiesFunc("file2")}
	file3 := &drive.File{Id: "file3", Name: "presentation.pptx", Parents: []string{"folder2"}, CreatedTime: now, Properties: propertiesFunc("file3")}
	file4 := &drive.File{Id: "file4", Name: "report.pdf", Parents: []string{"folder1"}, CreatedTime: now, Properties: propertiesFunc("file4")}

	tests := []struct {
		name string
		// Input to the NewDriveFileMap function
		inputFiles []*drive.File
		// Expected output from the NewDriveFileMap function
		expectedMap DriveFileMap
	}{
		{
			name:       "Empty input files",
			inputFiles: []*drive.File{},
			expectedMap: DriveFileMap{
				FileMap: make(map[DriveFolderID]map[string][]*drive.File),
			},
		},
		{
			name: "Single file, single parent",
			inputFiles: []*drive.File{
				file1,
			},
			expectedMap: DriveFileMap{
				FileMap: map[DriveFolderID]map[string][]*drive.File{
					"folder1": {
						date: {file1},
					},
				},
			},
		},
		{
			name: "Multiple files, single parent",
			inputFiles: []*drive.File{
				file1, file4,
			},
			expectedMap: DriveFileMap{
				FileMap: map[DriveFolderID]map[string][]*drive.File{
					"folder1": {
						date: {file1, file4},
					},
				},
			},
		},
		{
			name: "Multiple files, multiple parents",
			inputFiles: []*drive.File{
				file1, file2, file3,
			},
			expectedMap: DriveFileMap{
				FileMap: map[DriveFolderID]map[string][]*drive.File{
					"folder1": {
						date: {file1, file2}, // Note: file2 is in both folder1 and folder2
					},
					"folder2": {
						date: {file2, file3},
					},
				},
			},
		},
		{
			name: "File with no parents (should not be added)",
			inputFiles: []*drive.File{
				{Id: "orphan", Name: "orphan.txt", Parents: []string{}, CreatedTime: now},
				file1,
			},
			expectedMap: DriveFileMap{
				FileMap: map[DriveFolderID]map[string][]*drive.File{
					"folder1": {
						date: {file1},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewDriveFileMap(tt.inputFiles)

			if !reflect.DeepEqual(got.FileMap, tt.expectedMap.FileMap) {
				t.Errorf("NewDriveFileMap() got = %+v, want %+v", got.FileMap, tt.expectedMap.FileMap)
			}
		})
	}
}

func TestDriveFileMap_Get(t *testing.T) {
	// Sample drive.File objects for testing
	now := time.Now()
	nowFormatted := now.Format(time.RFC3339)
	yesterday := now.Add(-24 * time.Hour)
	yesterdayFormatted := yesterday.Format(time.RFC3339)

	file1 := &drive.File{Id: "file1", Name: "Document.docx", Parents: []string{"folder1"}, CreatedTime: nowFormatted, Properties: propertiesFunc("file1")}
	file2 := &drive.File{Id: "file2", Name: "image.png", Parents: []string{"folder1"}, CreatedTime: nowFormatted, Properties: propertiesFunc("file2")}
	file3 := &drive.File{Id: "file3", Name: "Presentation.pptx", Parents: []string{"folder2"}, CreatedTime: yesterdayFormatted, Properties: propertiesFunc("file3")}
	file4 := &drive.File{Id: "file4", Name: "document.docx", Parents: []string{"folder1"}, CreatedTime: nowFormatted, Properties: propertiesFunc("file4")} // Duplicate name, same folder, same time

	// Initialize DriveFileMap for testing
	dfm := NewDriveFileMap([]*drive.File{file1, file2, file3, file4})

	// b, _ := json.MarshalIndent(dfm, " ", " ")
	// log.Println(string(b))
	tests := []struct {
		name          string
		inputRecord   *Record
		expectedFiles []*drive.File
		expectedErr   error
	}{
		{
			name:          "Exact match, single file",
			inputRecord:   NewRecord("Document.docx", "folder1", now),
			expectedFiles: []*drive.File{file1, file4}, // Expecting both files with the same name and time
			expectedErr:   nil,
		},
		{
			name:          "Exact match, multiple files with same name and time",
			inputRecord:   NewRecord("document.docx", "folder1", now),
			expectedFiles: []*drive.File{file1, file4}, // Case-insensitive matching and duplicate handling
			expectedErr:   nil,
		},
		{
			name:          "Case insensitive filename match",
			inputRecord:   NewRecord("IMAGE.PNG", "folder1", now), // Different case
			expectedFiles: []*drive.File{file2},
			expectedErr:   nil,
		},
		{
			name:          "File not found (different name)",
			inputRecord:   NewRecord("nonexistent.txt", "folder1", now),
			expectedFiles: nil,
			expectedErr:   nil,
		},
		{
			name:          "File not found (different folder)",
			inputRecord:   NewRecord("document.docx", "nonexistentFolder", now),
			expectedFiles: nil,
			expectedErr:   nil,
		},
		{
			name:          "File not found (different date)",
			inputRecord:   NewRecord("Presentation.pptx", "folder2", now),
			expectedFiles: nil,
			expectedErr:   nil,
		},
		{
			name:          "Trim whitespace filename match",
			inputRecord:   NewRecord("  document.docx  ", "folder1", now), // With whitespace
			expectedFiles: []*drive.File{file1, file4},
			expectedErr:   nil,
		},
		{
			name:          "Record DocDate() returns error",
			inputRecord:   &Record{},
			expectedFiles: nil,
			expectedErr:   fmt.Errorf("Could not find date column"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFiles, gotErr := dfm.Get(tt.inputRecord)

			// Check for error first
			if !reflect.DeepEqual(gotErr, tt.expectedErr) {
				t.Errorf("Get() error = %v, wantErr %v", gotErr, tt.expectedErr)
				return
			}

			// Only check files if no error is expected or if the error was handled.
			// The current code will return nil files even if err is not nil, so we need to be careful.
			if tt.expectedErr == nil {
				if !reflect.DeepEqual(gotFiles, tt.expectedFiles) {
					t.Errorf("Get() gotFiles = %+v, want %+v", gotFiles, tt.expectedFiles)
				}
			}
		})
	}
}
*/
