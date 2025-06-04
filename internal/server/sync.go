// TODO gzip
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"zakirullin/stuffbot/internal/fs"
)

const (
	StorageDir            = "/app/mystorage"
	StatusOK              = "ok"
	StatusNotModified     = "notModified"
	StatusUpdatedOnServer = "updatedOnServer"
)

var (
	AuthToken string
)

type file struct {
	UserID       int64  `json:"userId"`
	Status       string `json:"status"`
	Path         string `json:"path"`
	LastModified int64  `json:"lastModified"`
	Content      string `json:"content"`
}

type syncRequest struct {
	UserID     int64            `json:"userId"`
	Timestamps map[string]int64 `json:"timestamps"`
	Files      []file           `json:"files"` // New or modified files from client
}

type syncResponse struct {
	Status     string           `json:"status"`     // Status
	Files      []file           `json:"files"`      // Files with content that need syncing
	Timestamps map[string]int64 `json:"timestamps"` // Current server timestamps in Unix format
	Deletions  []string         `json:"deletions"`
}

func SyncTexts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request syncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		log.Printf("Error parsing syncMediasRequest JSON: %v", err)
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	userFS, err := fs.NewUserFS(request.UserID)
	if err != nil {
		log.Printf("Error creating user FS: %v", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	// 1) Save client-modified files to the server
	// 2) In case of conflict (server has a newer modification), merge the files and include them in the response
	// 3) Based on known client dirs timestamps, send newly updated or created files
	// 4) Respond with last modification timestamps for every dir

	// Save client-modified files to the server
	for _, clientFile := range request.Files {
		path := clientFile.Path

		serverModifiedTime, err := userFS.Ctime("", path)
		var clientContent string
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("Error reading file '%s': %v", path, err)
			logSync(fmt.Sprintf("Error reading file '%s': %v", path, err))
			// TODO All-or-nothing sync?
			continue
		} else if os.IsNotExist(err) {
			logSync(fmt.Sprintf("Creating: '%s'", clientFile.Path))
			clientContent = clientFile.Content
		} else {
			// file locks?
			fileWasModifiedOnServer := serverModifiedTime > clientFile.LastModified
			if fileWasModifiedOnServer {
				serverContent, err := userFS.Read("", path)
				if err != nil {
					log.Printf("Error reading file '%s': %v", path, err)
					continue
				}
				logSync(fmt.Sprintf("Merging and writing: '%s'", clientFile.Path))
				clientContent = Merge(string(serverContent), clientFile.Content)
			} else {
				// Server file hasn't changed since client's last sync
				logSync(fmt.Sprintf("Writing only: '%s'", clientFile.Path))
				clientContent = clientFile.Content
			}
		}

		// Write the clientContent to the server at path
		err = userFS.Write("", path, clientContent)
		if err != nil {
			log.Printf("Error writing file '%s': %v", path, err)
			logSync(fmt.Sprintf("Error writing file '%s': %v", path, err))
			continue
		}
	}

	serverTimestamps, err := userFS.Ctimes()
	if err != nil {
		log.Printf("Error getting server timestamps: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get timestamps: %v", err), http.StatusInternalServerError)
		return
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
			content, err := userFS.Read("", path)
			if err != nil {
				log.Printf("Error reading file %s: %v", path, err)
				logSync(fmt.Sprintf("Error reading file %s: %v", path, err))
				continue
			}
			logSync(fmt.Sprintf("Sending file: '%s'", path))

			files = append(files, file{
				Status:       StatusOK,
				Path:         path,
				LastModified: serverFileTime,
				Content:      string(content),
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
	deletions := make([]string, 0)
	for clientPath := range request.Timestamps {
		if _, existsOnServer := serverTimestamps[clientPath]; !existsOnServer {
			deletions = append(deletions, clientPath)
		}
	}
	if len(deletions) > 0 {
		logSync(fmt.Sprintf("Deleting files: %v", deletions))
	}

	response := syncResponse{
		Status:     StatusOK,
		Files:      files,
		Timestamps: dirTimestamps,
		Deletions:  deletions,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding sync response: %v", err)
	}
}

func SyncText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clientFile file
	if err := json.NewDecoder(r.Body).Decode(&clientFile); err != nil {
		log.Printf("Error parsing syncMediasRequest JSON: %v", err)
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	path := clientFile.Path
	userFS, err := fs.NewUserFS(clientFile.UserID)
	if err != nil {
		log.Printf("Error creating user FS: %v", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	// 1) Save client-modified file to the server
	// 2) In case of conflict (server has a newer modification), merge the clientFile and include them in the response

	// TODO if no clientFile, severContent = ""
	serverContent, err := userFS.Read("", path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("Error reading one clientFile '%s': %v", path, err)
		http.Error(w, "Error reading server clientFile", http.StatusBadRequest)
		return
	}

	ctime, err := userFS.Ctime("", path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Error getting ctime for clientFile '%s': %v", path, err)
			http.Error(w, "Error getting ctime for clientFile", http.StatusBadRequest)
			return
		}
	}

	// TODO when clientFile does not exist the content is empty, which is implicit
	// Return already up-to-date status
	if serverContent == clientFile.Content {
		response := map[string]interface{}{
			"status":       StatusNotModified,
			"lastModified": ctime,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotModified)
		json.NewEncoder(w).Encode(response)
		return
	}

	var content string
	fileWasModifiedOnServer := false
	if errors.Is(err, os.ErrNotExist) {
		logSync(fmt.Sprintf("Creating one clientFile: '%s'", clientFile.Path))
		content = clientFile.Content
	} else {
		fileWasModifiedOnServer = ctime > clientFile.LastModified
		if fileWasModifiedOnServer {
			logSync(fmt.Sprintf("Server one clientFile '%s' was modified at %d, client timestamp is %d", path, ctime, clientFile.LastModified))
			logSync(fmt.Sprintf("Merging and writing one clientFile: '%s'", clientFile.Path))
			content = Merge(string(serverContent), clientFile.Content)
		} else {
			// TODO for resilience add merge here, because we had case when server saved latest TS but no conent.
			// Also, if for some reason timestamps would change on server migration and such.
			// Server clientFile hasn't changed since client's last sync
			logSync(fmt.Sprintf("Writing only one clientFile: '%s'", clientFile.Path))
			content = clientFile.Content
		}
	}

	// Write the content to the server at path
	err = userFS.Write("", path, content)
	if err != nil {
		log.Printf("Error writing clientFile '%s': %v", path, err)
		logSync(fmt.Sprintf("Error writing clientFile '%s': %v", path, err))
		http.Error(w, "Error writing clientFile", http.StatusInternalServerError)
		return
	}

	ctime, err = userFS.Ctime("", path)
	// TODO what if 0?
	logSync(fmt.Sprintf("Server timestamp for '%s': %d", path, ctime))

	if !fileWasModifiedOnServer {
		response := map[string]interface{}{
			"status":       StatusUpdatedOnServer,
			"lastModified": ctime,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	response := file{
		Status:       StatusOK,
		Content:      content,
		Path:         clientFile.Path,
		LastModified: ctime,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding sync response: %v", err)
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

// validateAuthToken checks if the syncMediasRequest has a valid auth token
func validateAuthToken(r *http.Request) bool {
	token := r.Header.Get("Authorization")

	if strings.HasPrefix(token, "Bearer ") {
		token = token[7:]
	}

	return token == AuthToken
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validateAuthToken(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-CSRF-Token")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

func logSync(msg string) {
	file, err := os.OpenFile("/tmp/sync", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return
	}
	defer file.Close()

	time := time.Now().Format("2006-01-02 15:04:05")
	if _, err := file.WriteString(time + ": " + msg + "\n"); err != nil {
		fmt.Println("Error writing to log file:", err)
		return
	}
}
