package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/internal/fs"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}
}

func TestSyncText_CreateNewFileOnServer(t *testing.T) {
	r := require.New(t)

	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return fs.NewFS("/", afero.NewMemMapFs())

	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	clientFile := file{
		UserID:       -1,
		Path:         "test.md",
		Content:      "Hello World",
		LastModified: 1234567890,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncText(w, req)
	r.Equal(http.StatusOK, w.Code)

	var response file
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusUpdatedOnServer, response.Status)
	r.True(response.LastModified > 0)
}

func TestSyncText_UpdateExistingFile_NoConflict(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	err = userFS.Write("", "test.md", "Original content")
	r.NoError(err)

	clientFile := file{
		UserID:       -1,
		Path:         "test.md",
		Content:      "Updated content",
		LastModified: 1,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusUpdatedOnServer, response["status"])

	// Verify content was updated
	content, err := userFS.Read("", "test.md")
	r.NoError(err)
	r.Equal("Updated content", content)
}

func TestSyncText_NotModified(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	err = userFS.Write("", "test.md", "Original content")
	r.NoError(err)

	clientFile := file{
		UserID:       -1,
		Path:         "test.md",
		Content:      "Original content",
		LastModified: 1,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusNotModified, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusNotModified, response["status"])

	// Verify content was updated
	content, err := userFS.Read("", "test.md")
	r.NoError(err)
	r.Equal("Original content", content)
}

func TestSyncText_UpdateExistingFile_Conflict(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	err = userFS.Write("", "test.md", "Server content")
	r.NoError(err)

	clientFile := file{
		UserID:       -1,
		Path:         "test.md",
		Content:      "Client content",
		LastModified: 0,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal("ok", response["status"])

	// Verify content was updated
	content, err := userFS.Read("", "test.md")
	r.NoError(err)
	r.Equal("Server content\nClient content", content)
}

func TestSyncText_UpdateExistingFile_JournalConflict(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	err = userFS.Write("", "test.md", "#### 25 May, Friday 🚀\nServer content")
	r.NoError(err)

	clientFile := file{
		UserID:       -1,
		Path:         "test.md",
		Content:      "#### 25 May, Friday\nServer content",
		LastModified: 0,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal("ok", response["status"])

	// Verify content was updated
	content, err := userFS.Read("", "test.md")
	r.NoError(err)
	r.Equal("#### 25 May, Friday 🚀\nServer content", content)
}

func TestSyncAllTexts_EmptyRequest(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	request := syncRequest{
		UserID:     -1,
		Timestamps: make(map[string]int64),
		Files:      []file{},
	}

	body, err := json.Marshal(request)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncTexts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncTexts(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response syncResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusOK, response.Status)
	r.Empty(response.Files)
	r.Empty(response.Deletions)
}

func TestSyncAllTexts_CreateNewFilesOnServer(t *testing.T) {
	r := require.New(t)

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	request := syncRequest{
		UserID:     -1,
		Timestamps: make(map[string]int64),
		Files: []file{
			{
				Path:         "today/task1.md",
				Content:      "Task 1 content",
				LastModified: 0,
			},
			{
				Path:         "later/task2.md",
				Content:      "Task 2 content",
				LastModified: 0,
			},
		},
	}

	body, err := json.Marshal(request)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncTexts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	SyncTexts(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response syncResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusOK, response.Status)
	r.Len(response.Files, 2)
	r.Contains(response.Timestamps, "today")
	r.Contains(response.Timestamps, "later")

	// Verify files were created on server
	r.NoError(err)
	content1, err := userFS.Read("today", "task1.md")
	r.NoError(err)
	r.Equal("Task 1 content", content1)

	content2, err := userFS.Read("later", "task2.md")
	r.NoError(err)
	r.Equal("Task 2 content", content2)
}

//func TestSyncAllTexts_UpdateExistingFiles(t *testing.T) {
//	r := require.New(t)
//	setupTestServer(t)
//
//	// Create existing files on server
//	userFS, err := fs.NewUserFS(123)
//	r.NoError(err)
//	err = userFS.Write("", "today/existing.md", "Original content")
//	r.NoError(err)
//
//	serverTime, err := userFS.Ctime("", "today/existing.md")
//	r.NoError(err)
//
//	request := syncRequest{
//		UserID: 123,
//		Timestamps: map[string]int64{
//			"today": serverTime - 1000, // Old timestamp
//		},
//		Files: []file{
//			{
//				Path:         "today/existing.md",
//				Content:      "Updated content",
//				LastModified: serverTime, // Same as server time
//			},
//		},
//	}
//
//	body, err := json.Marshal(request)
//	r.NoError(err)
//
//	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
//	req.Header.Set("Content-Type", "application/json")
//	w := httptest.NewRecorder()
//
//	SyncAllTexts(w, req)
//
//	r.Equal(http.StatusOK, w.Code)
//
//	var response syncResponse
//	err = json.Unmarshal(w.Body.Bytes(), &response)
//	r.NoError(err)
//	r.Equal(StatusOK, response.Status)
//
//	// Verify file was updated
//	content, err := userFS.Read("", "today/existing.md")
//	r.NoError(err)
//	r.Equal("Updated content", content)
//}
//
//func TestSyncAllTexts_ConflictMerge(t *testing.T) {
//	r := require.New(t)
//	setupTestServer(t)
//
//	// Mock the Merge function
//	originalMerge := Merge
//	defer func() {
//		Merge = originalMerge
//	}()
//	Merge = func(serverContent, clientContent string) string {
//		return "MERGED: " + serverContent + " + " + clientContent
//	}
//
//	// Create file on server
//	userFS, err := fs.NewUserFS(123)
//	r.NoError(err)
//	err = userFS.Write("", "conflict.md", "Server version")
//	r.NoError(err)
//
//	request := syncRequest{
//		UserID:     123,
//		Timestamps: make(map[string]int64),
//		Files: []file{
//			{
//				Path:         "conflict.md",
//				Content:      "Client version",
//				LastModified: 1000, // Very old timestamp
//			},
//		},
//	}
//
//	body, err := json.Marshal(request)
//	r.NoError(err)
//
//	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
//	req.Header.Set("Content-Type", "application/json")
//	w := httptest.NewRecorder()
//
//	SyncAllTexts(w, req)
//
//	r.Equal(http.StatusOK, w.Code)
//
//	// Verify merged content was saved
//	content, err := userFS.Read("", "conflict.md")
//	r.NoError(err)
//	r.Equal("MERGED: Server version + Client version", content)
//}
//
//func TestSyncAllTexts_SendUpdatedFiles(t *testing.T) {
//	r := require.New(t)
//	setupTestServer(t)
//
//	// Create files on server
//	userFS, err := fs.NewUserFS(123)
//	r.NoError(err)
//	err = userFS.Write("", "today/new.md", "New server file")
//	r.NoError(err)
//	err = userFS.Write("", "today/old.md", "Old file")
//	r.NoError(err)
//
//	oldFileTime, err := userFS.Ctime("", "today/old.md")
//	r.NoError(err)
//
//	request := syncRequest{
//		UserID: 123,
//		Timestamps: map[string]int64{
//			"today": oldFileTime - 1000, // Client has old timestamp
//		},
//		Files: []file{},
//	}
//
//	body, err := json.Marshal(request)
//	r.NoError(err)
//
//	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
//	req.Header.Set("Content-Type", "application/json")
//	w := httptest.NewRecorder()
//
//	SyncAllTexts(w, req)
//
//	r.Equal(http.StatusOK, w.Code)
//
//	var response syncResponse
//	err = json.Unmarshal(w.Body.Bytes(), &response)
//	r.NoError(err)
//	r.Equal(StatusOK, response.Status)
//	r.Len(response.Files, 2) // Both files should be sent
//
//	// Check that files contain content
//	for _, file := range response.Files {
//		r.NotEmpty(file.Content)
//		r.True(file.LastModified > 0)
//	}
//}
//
////func TestSyncAllTexts_CalculateDeletions(t *testing.T) {
////	r := require.New(t)
////	setupTestServer(t)
////
////	// Create one file on server
////	userFS, err := fs.NewUserFS(123)
////	r.NoError(err)
////	err = userFS.Write("", "existing.md", "Exists")
////	r.NoError(err)
////
////	request := syncRequest{
////		UserID: 123,
////		Timestamps: map[string]int64{
////			"existing.md":    1000,
////			"nonexistent.md": 1000, // Client thinks this exists
////		},
////		Files: []file{},
////	}
////
////	body, err := json.Marshal(request)
////	r.NoError(err)
////
////	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
////	req.Header.Set("Content-Type", "application/json")
////	w := httptest.NewRecorder()
////
////	SyncAllTexts(w, req)
////
////	r.Equal(http.StatusOK, w.Code)
////
////	var response syncResponse
////	err = json.Unmarshal(w.Body.Bytes(), &response)
////	r.NoError(err)
////	r.Equal(StatusOK, response.Status)
////	r.Contains(response.Deletions, "nonexistent.md")
////	r.NotContains(response.Deletions, "existing.md")
////}
////
////func TestSyncAllTexts_InvalidMethod(t *testing.T) {
////	r := require.New(t)
////
////	req := httptest.NewRequest(http.MethodGet, "/sync-all", nil)
////	w := httptest.NewRecorder()
////
////	SyncAllTexts(w, req)
////
////	r.Equal(http.StatusMethodNotAllowed, w.Code)
////}
////
////func TestSyncAllTexts_InvalidJSON(t *testing.T) {
////	r := require.New(t)
////
////	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBufferString("invalid json"))
////	w := httptest.NewRecorder()
////
////	SyncAllTexts(w, req)
////
////	r.Equal(http.StatusBadRequest, w.Code)
////}
//
//func TestSyncAllTexts_RootDirectoryFiles(t *testing.T) {
//	r := require.New(t)
//	setupTestServer(t)
//
//	request := syncRequest{
//		UserID:     123,
//		Timestamps: make(map[string]int64),
//		Files: []file{
//			{
//				Path:         "root.md", // File in root directory
//				Content:      "Root file content",
//				LastModified: 1234567890,
//			},
//		},
//	}
//
//	body, err := json.Marshal(request)
//	r.NoError(err)
//
//	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
//	req.Header.Set("Content-Type", "application/json")
//	w := httptest.NewRecorder()
//
//	SyncAllTexts(w, req)
//
//	r.Equal(http.StatusOK, w.Code)
//
//	var response syncResponse
//	err = json.Unmarshal(w.Body.Bytes(), &response)
//	r.NoError(err)
//	r.Equal(StatusOK, response.Status)
//	r.Contains(response.Timestamps, ".") // Root directory should be "."
//}
