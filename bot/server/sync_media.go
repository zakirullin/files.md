package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"zakirullin/stuffbot/bot/fs"
)

var syncMediasRequest struct {
	Timestamp     int64  `json:"timestamp"`
	FilenamesHash string `json:"filenamesHash"`
}

type media struct {
	Filename     string `json:"filename"`
	LastModified int64  `json:"lastModified"`
	Data         string `json:"data"`
}

func SyncMedias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&syncMediasRequest); err != nil {
		http.Error(w, "Invalid syncMediasRequest JSON", http.StatusBadRequest)
		return
	}

	// TODO ../.. Attacks (fixed with fs.FS?)
	logSync(fmt.Sprintf("Media sync syncMediasRequest for folder: '%s', last sync: %d", fs.DirMedia, syncMediasRequest.Timestamp), r)

	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		slog.Error("Sync error: syncMedias: error creating media FS", "error", err)
		http.Error(w, "Error creating media FS", http.StatusInternalServerError)
		return
	}

	// Find media files newer than client's timestamp
	ctimes, err := userFS.Mtimes(fs.DirMedia)
	if err != nil {
		slog.Error("Sync error: syncMedias: error getting media file times", "error", err)
		http.Error(w, "Error getting media file times", http.StatusInternalServerError)
		return
	}

	mediaFiles := make([]media, 0)
	latestTimestamp := int64(0)
	for filename, modTime := range ctimes {
		// TODO theoretically it is possible to miss some files if there were created in the same second.
		if modTime <= syncMediasRequest.Timestamp {
			continue
		}
		if modTime > latestTimestamp {
			latestTimestamp = modTime
		}

		mediaFiles = append(mediaFiles, media{
			Filename:     filename,
			LastModified: modTime,
		})
	}

	response := struct {
		Files     []media `json:"files"`
		Timestamp int64   `json:"timestamp"`
	}{
		Files:     mediaFiles,
		Timestamp: latestTimestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
	}
}

// SyncMedia syncs a single media file by path.
func SyncMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clientMedia media
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot read request body: %s", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, &clientMedia); err != nil {
		http.Error(w, fmt.Sprintf("Invalid syncMedia Request JSON: %s", body), http.StatusBadRequest)
		return
	}

	userFS, err := fs.NewUserFS(userID(r))
	if err != nil {
		slog.Error("Sync error: syncMedia: error creating user FS", "error", err)
		http.Error(w, "Error creating user FS", http.StatusInternalServerError)
		return
	}

	exists, err := userFS.Exists(fs.DirMedia, clientMedia.Filename)
	if err != nil {
		slog.Error("Sync error: syncMedia: error checking media existence", "error", err)
		http.Error(w, "Error checking media existence", http.StatusInternalServerError)
		return
	}

	// TODO add size limits!
	shouldWriteToServer := clientMedia.Data != "" && !exists
	if shouldWriteToServer {
		content, err := base64.StdEncoding.DecodeString(clientMedia.Data)
		if err != nil {
			http.Error(w, "Invalid base64 data", http.StatusBadRequest)
			return
		}

		err = userFS.Write(fs.DirMedia, clientMedia.Filename, string(content))
		if err != nil {
			http.Error(w, "Invalid base64 data", http.StatusBadRequest)
			return
		}

		logSync(fmt.Sprintf("Media created: %s", clientMedia.Filename), r)
		return
	}

	path, err := userFS.SafePath(fs.DirMedia, clientMedia.Filename)
	if err != nil {
		slog.Error("Sync error: syncMedia: unsafe path", "error", err)
		http.Error(w, "The path is unsafe", http.StatusInternalServerError)
		return
	}

	http.ServeFile(w, r, path)
}
