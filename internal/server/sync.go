// TODO gzip
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"zakirullin/stuffbot/config"
	"zakirullin/stuffbot/internal/fs"
)

const (
	// TODO remove my stroage
	StorageDir            = "/app/mystorage"
	StatusOK              = "ok"
	StatusNotModified     = "notModified"
	StatusUpdatedOnServer = "updatedOnServer"
)

type file struct {
	Status             string `json:"status"`
	Path               string `json:"path"`
	LastModified       int64  `json:"lastModified"`
	ClientLastModified int64  `json:"clientLastModified,omitempty"`
	ClientLastSynced   int64  `json:"clientLastSynced,omitempty"`
	Content            string `json:"content"`
}

type syncRequest struct {
	Modified   []file           `json:"modified"` // New or modified files from client
	Deleted    []string         `json:"deleted"`  // Deleted files from client
	Timestamps map[string]int64 `json:"timestamps"`
}

type syncResponse struct {
	Status     string            `json:"status"`     // Status
	Files      []file            `json:"files"`      // Files with content that need syncing
	Timestamps map[string]int64  `json:"timestamps"` // Current server timestamps in Unix format
	Renames    map[string]string `json:"renames"`    // What files to rename on client
}

// SyncTexts sync texts between client and server.
// The following steps are executed:
// 1) Save client-modified files to the server
// 2) In case of conflict (server has a newer modification), merge the files and include them in the response
// 3) Based on known client dirs timestamps, send newly updated or created files
// 4) Respond with last modification timestamps for every dir
func SyncTexts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request syncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		slog.Error("Sync error: syncTexts: error creating user FS", "error", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	// Delete files.
	for _, path := range request.Deleted {
		err = userFS.Del(fs.DirRoot, path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			slog.Error("Sync error: syncTexts: error deleting file", "path", path, "error", err)
			continue
		}
		logDelete(fmt.Sprintf("Deleting file: '%s'", path), r)
	}

	// TODO using rename log first replace old paths in client request to new so other code will work okay
	// and maybe include it right away for files to send
	// TODO what if multiply moves, back and forth? Merge them?
	lastSync := int64(0)
	for _, ts := range request.Timestamps {
		if ts > lastSync {
			lastSync = ts
		}
	}
	// TODO if a file was changed on client on oldPath, merge it with the new path

	renames := make(map[string]string)
	// Don't respond renames on first sync
	if lastSync != 0 {
		renames = RenamesLog(userID(r), lastSync)
	}

	// If a file was renamed and changed, on client we would rename then change?
	// Save client-modified files to the server
	for _, clientFile := range request.Modified {
		path := clientFile.Path

		serverModifiedTime, err := userFS.Ctime(fs.DirRoot, path)
		var clientContent string
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Error("Sync error: syncTexts: error reading file '%s': %v", path, err)
			logSync(fmt.Sprintf("Error reading file '%s': %v", path, err), r)
			// TODO All-or-nothing sync?
			continue
		} else if errors.Is(err, os.ErrNotExist) {
			logSync(fmt.Sprintf("Creating: '%s'", clientFile.Path), r)
			clientContent = clientFile.Content
		} else {
			// file locks?
			fileWasModifiedOnServer := serverModifiedTime > clientFile.LastModified
			if fileWasModifiedOnServer {
				serverContent, err := userFS.Read(fs.DirRoot, path)
				if err != nil {
					slog.Error("Sync error: syncTexts: error reading modified on server file '%s': %v", path, err)
					continue
				}
				logSync(fmt.Sprintf("Merging and writing: '%s'", clientFile.Path), r)
				clientContent = Merge(string(serverContent), clientFile.Content)
			} else {
				// Changed on client, unchanged on client
				logSync(fmt.Sprintf("Writing only: '%s'", clientFile.Path), r)
				clientContent = clientFile.Content
			}
		}

		// Write the clientContent to the server at path
		err = userFS.Write(fs.DirRoot, path, clientContent)
		if err != nil {
			slog.Error("Sync error: syncTexts: error writing file '%s': %v", path, err)
			logSync(fmt.Sprintf("Error writing file '%s': %v", path, err), r)
			continue
		}
	}

	// Based on known client dirs timestamps, send newly updated or created files.
	serverTimestamps, err := userFS.Ctimes(fs.DirRoot, fs.MDExt, ".txt")
	if err != nil {
		slog.Error("Sync error: syncTexts: error getting server timestamps", "error", err)
		http.Error(w, fmt.Sprintf("Failed to get timestamps: %v", err), http.StatusInternalServerError)
		return
	}

	configCtime, err := userFS.Ctime(fs.DirRoot, config.BotCfg.ConfigFilename)
	if err != nil {
		slog.Error("Sync error: syncTexts: error getting timestamp for config file", "error", err)
	} else {
		serverTimestamps[config.BotCfg.ConfigFilename] = configCtime
	}

	// Prepare the list of files to send to the client
	// TODO optimize don't send files known to client.
	// For now we save client file to server, and the code below would include it again.
	files := make([]file, 0)
	dirTimestamps := make(map[string]int64)
	for path, serverFileTime := range serverTimestamps {
		// TOOD make it not as ugly?
		parts := strings.Split(path, string(os.PathSeparator))
		dir := parts[0]
		isInRoot := len(parts) == 1
		if isInRoot {
			dir = "."
		}

		requestDirTime, exists := request.Timestamps[dir]
		if !exists || serverFileTime > requestDirTime {
			// Client needs this file - read its content
			content, err := userFS.Read(fs.DirRoot, path)
			if err != nil {
				slog.Error("Sync error: syncTexts: error reading file", "path", path, "error", err)
				logSync(fmt.Sprintf("Error reading file %s: %v", path, err), r)
				continue
			}

			files = append(files, file{
				Status:       StatusOK,
				Path:         path,
				LastModified: serverFileTime,
				Content:      content,
			})
		}

		// Calculate the latest file timestamp for each directory
		existingTimestamp, exists := dirTimestamps[dir]
		if !exists {
			dirTimestamps[dir] = serverFileTime
			continue
		}
		if serverFileTime > existingTimestamp {
			dirTimestamps[dir] = serverFileTime
		}
	}

	// Calculate deletions for client (files that exist on client but not on server)
	// NO real delete yet
	deletions := make([]string, 0)
	for clientPath := range request.Timestamps {
		if _, existsOnServer := serverTimestamps[clientPath]; !existsOnServer {
			deletions = append(deletions, clientPath)
		}
	}
	if len(deletions) > 0 {
		logSync(fmt.Sprintf("Deleting files: %v", deletions), r)
	}

	response := syncResponse{
		Status:     StatusOK,
		Files:      files,
		Timestamps: dirTimestamps,
		Renames:    renames,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

func SyncText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clientFile file
	if err := json.NewDecoder(r.Body).Decode(&clientFile); err != nil {
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	path := clientFile.Path
	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		slog.Error("Sync error: syncText: error creating user FS", "error", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	// 1) Save client-modified file to the server
	// 2) In case of conflict (server has a newer modification), merge the clientFile and include them in the response

	// TODO if no clientFile, severContent = ""
	serverContent, err := userFS.Read(fs.DirRoot, path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("Sync error: syncText: error reading clientFile '%s': %v", path, err)
		http.Error(w, "Error reading server clientFile", http.StatusBadRequest)
		return
	}

	serverLastModified, err := userFS.Ctime(fs.DirRoot, path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Error("Sync error: syncText: error getting ctime for clientFile '%s': %v", path, err)
			http.Error(w, "Error getting ctime for clientFile", http.StatusBadRequest)
			return
		}
	}

	// TODO when clientFile does not exist the content is empty, which is implicit
	// Return already up-to-date status
	if serverContent == clientFile.Content {
		response := map[string]interface{}{
			"status":       StatusNotModified,
			"lastModified": serverLastModified,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	var content string
	fileWasModifiedOnServer := false
	if errors.Is(err, os.ErrNotExist) {
		logSync(fmt.Sprintf("Creating one clientFile: '%s'", clientFile.Path), r)
		content = clientFile.Content
	} else {
		wasNotModifiedOnClient := clientFile.ClientLastSynced != 0 && clientFile.ClientLastModified == clientFile.ClientLastSynced
		fileWasModifiedOnServer = serverLastModified > clientFile.LastModified
		if fileWasModifiedOnServer && wasNotModifiedOnClient {
			content = serverContent
		} else if fileWasModifiedOnServer {
			logSync(fmt.Sprintf("Server one clientFile '%s' was modified at %d, client timestamp is %d", path, serverLastModified, clientFile.LastModified), r)
			logSync(fmt.Sprintf("Merging and writing one clientFile: '%s'", clientFile.Path), r)
			content = Merge(serverContent, clientFile.Content)
		} else {
			// TODO for resilience add merge here, because we had case when server saved latest TS but no conent.
			// Also, if for some reason timestamps would change on server migration and such.
			// Server clientFile hasn't changed since client's last sync
			logSync(fmt.Sprintf("Writing only one clientFile: '%s'", clientFile.Path), r)
			content = clientFile.Content
		}
	}

	// Write the content to the server at path
	err = userFS.Write(fs.DirRoot, path, content)
	if err != nil {
		slog.Error("Sync error: syncText: error writing clientFile '%s': %v", path, err)
		logSync(fmt.Sprintf("Error writing clientFile '%s': %v", path, err), r)
		http.Error(w, "Error writing clientFile", http.StatusInternalServerError)
		return
	}

	serverLastModified, err = userFS.Ctime(fs.DirRoot, path)
	// TODO what if 0?
	logSync(fmt.Sprintf("Server timestamp for '%s': %d", path, serverLastModified), r)

	if !fileWasModifiedOnServer {
		response := map[string]interface{}{
			"status":       StatusUpdatedOnServer,
			"lastModified": serverLastModified,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	response := file{
		Status:       StatusOK,
		Content:      content,
		Path:         clientFile.Path,
		LastModified: serverLastModified,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

func logSync(msg string, r *http.Request) {
	msg = fmt.Sprintf("%d: %s", userID(r), msg)

	file, err := os.OpenFile("/tmp/sync", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return
	}
	defer file.Close()

	time := time.Now().Format("2006-01-02 15:04:05")
	if _, err := file.WriteString(time + ": " + msg + "\n"); err != nil {
		slog.Error("Sync error: logSync: error writing to log file", "error", err)
		return
	}
}

func logDelete(msg string, r *http.Request) {
	msg = fmt.Sprintf("%d: %s", userID(r), msg)
	file, err := os.OpenFile("/tmp/del", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Sync error: logDelete: error opening log file", "error", err)
		return
	}
	defer file.Close()

	time := time.Now().Format("2006-01-02 15:04:05")
	if _, err := file.WriteString(time + ": " + msg + "\n"); err != nil {
		fmt.Println("Error writing to log file:", err)
		return
	}
}

func userID(r *http.Request) int64 {
	return r.Context().Value("userID").(int64)
}
