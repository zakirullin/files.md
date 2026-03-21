package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/server/fs"
)

func init() {
	fs.Ctime = func(fi os.FileInfo) int64 {
		return 1
	}
	fs.Mtime = func(fi os.FileInfo) int64 {
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
		Path:         "test.md",
		Content:      "Hello World",
		LastModified: 1234567890,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
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
		Path:         "test.md",
		Content:      "Updated content",
		LastModified: 1,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
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
		Path:         "test.md",
		Content:      "Original content",
		LastModified: 1,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

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
		Path:         "test.md",
		Content:      "Client content",
		LastModified: 0,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal("merged", response["status"])

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
		Path:         "test.md",
		Content:      "#### 25 May, Friday\nServer content",
		LastModified: 0,
	}

	body, err := json.Marshal(clientFile)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncText", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncText(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal("merged", response["status"])

	// Verify content was updated
	content, err := userFS.Read("", "test.md")
	r.NoError(err)
	r.Equal("#### 25 May, Friday 🚀\nServer content", content)
}

func TestSyncAllTexts_EmptyRequest(t *testing.T) {
	r := require.New(t)

	origFilename := config.BotCfg.ConfigFilename
	config.BotCfg.ConfigFilename = "config.json"
	defer func() {
		config.BotCfg.ConfigFilename = origFilename
	}()

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
		Timestamps: make(map[string]int64),
		Modified:   []file{},
	}

	body, err := json.Marshal(request)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncTexts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncTexts(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response syncResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusOK, response.Status)
	r.Empty(response.Files)
	r.Empty(response.Renames)
}

func TestSyncAllTexts_CreateNewFilesOnServer(t *testing.T) {
	r := require.New(t)

	origFilename := config.BotCfg.ConfigFilename
	config.BotCfg.ConfigFilename = "config.json"
	defer func() {
		config.BotCfg.ConfigFilename = origFilename
	}()

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
		Timestamps: make(map[string]int64),
		Modified: []file{
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
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
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

func TestSyncAllTexts_UpdateExistingFilesOnServer(t *testing.T) {
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

	err = userFS.Write("", "today/existing.md", "Original content")
	r.NoError(err)

	request := syncRequest{
		Timestamps: map[string]int64{
			"today": 0, // Old timestamp
		},
		Modified: []file{
			{
				Path:         "today/existing.md",
				Content:      "Updated content",
				LastModified: 1, // Same as server time
			},
		},
	}

	body, err := json.Marshal(request)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/sync-all", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncTexts(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response syncResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusOK, response.Status)

	// Verify file was updated
	content, err := userFS.Read("", "today/existing.md")
	r.NoError(err)
	r.Equal("Updated content", content)
}

func TestSyncAllTexts_SendUpdatedFilesToClient(t *testing.T) {
	r := require.New(t)

	origFilename := config.BotCfg.ConfigFilename
	config.BotCfg.ConfigFilename = "config.json"
	defer func() {
		config.BotCfg.ConfigFilename = origFilename
	}()

	userFS, err := fs.NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	origFS := fs.NewUserFS
	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
		return userFS, nil
	}
	defer func() {
		fs.NewUserFS = origFS
	}()

	// Create files on server
	err = userFS.Write("", "today/new.md", "New server file")
	r.NoError(err)
	err = userFS.Write("", "today/old.md", "Old file")
	r.NoError(err)

	request := syncRequest{
		Timestamps: map[string]int64{
			"today": 0,
		},
		Modified: []file{},
	}

	body, err := json.Marshal(request)
	r.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/syncTexts", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), "userID", int64(-1)))
	w := httptest.NewRecorder()

	SyncTexts(w, req)

	r.Equal(http.StatusOK, w.Code)

	var response syncResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	r.NoError(err)
	r.Equal(StatusOK, response.Status)
	r.Len(response.Files, 2) // Both files should be sent

	// Check that files contain content
	for _, file := range response.Files {
		r.NotEmpty(file.Content)
		r.True(file.LastModified > 0)
	}
}

//func TestSyncAllTexts_PathTraversalAttack(t *testing.T) {
//	r := require.New(t)
//
//	memFS := afero.NewMemMapFs()
//	rootFS, err := fs.NewFS("/", memFS)
//	r.NoError(err)
//	err = rootFS.Write("", "user1/secret1.md", "New server file")
//	r.NoError(err)
//	err = rootFS.Write("", "user2/secret2.md", "Old file")
//	r.NoError(err)
//
//	userFS, err := fs.NewFS("/user1", afero.NewMemMapFs())
//	r.NoError(err)
//	origFS := fs.NewUserFS
//	fs.NewUserFS = func(userID int64) (*fs.FS, error) {
//		return userFS, nil
//	}
//	defer func() {
//		fs.NewUserFS = origFS
//	}()
//
//	request := syncRequest{
//		UserID: 0,
//		Timestamps: map[string]int64{
//			"": 0,
//		},
//		Modified: []file{},
//	}
//
//	body, err := json.Marshal(request)
//	r.NoError(err)
//
//	req := httptest.NewRequest(http.MethodPost, "/syncTexts", bytes.NewBuffer(body))
//	req.DisplayName.Set("Content-Type", "application/json")
//	w := httptest.NewRecorder()
//
//	SyncTexts(w, req)
//
//	r.Equal(http.StatusOK, w.Code)
//
//	var response syncResponse
//	err = json.Unmarshal(w.Body.Bytes(), &response)
//	r.NoError(err)
//	r.Equal(StatusOK, response.Status)
//	r.Len(response.Modified, 2) // Both files should be sent
//
//	// Check that files contain content
//	for _, file := range response.Modified {
//		r.NotEmpty(file.Content)
//		r.True(file.LastModified > 0)
//	}
//}
