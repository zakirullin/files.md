package server

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"zakirullin/stuffbot/config"
)

const (
	Rename = "ren"
	Delete = "del"
)

var lock sync.RWMutex

func LogRename(time int64, oldPath, newPath string) {
	lock.Lock()
	defer lock.Unlock()

	file, err := os.OpenFile(path.Join(config.BotCfg.WorkingDir, "fslog"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	oldPath = url.QueryEscape(oldPath)
	newPath = url.QueryEscape(newPath)
	record := fmt.Sprintf("%d %s %s %s\n", time, Rename, oldPath, newPath)

	file.WriteString(record)
	file.Sync()
}

func LogDelete(time int64, filepath string) {
	lock.Lock()
	defer lock.Unlock()

	file, err := os.OpenFile(path.Join(config.BotCfg.WorkingDir, "fslog"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	filepath = url.QueryEscape(filepath)
	record := fmt.Sprintf("%d %s %s\n", time, Delete, filepath)

	file.WriteString(record)
	file.Sync()
}

// RenamesLog reads the file system renames log and returns a map of:
// newPath -> oldPath
// AfterTimestamp is inclusive.
func RenamesLog(userID, afterTimestamp int64) map[string]string {
	lock.RLock()
	defer lock.RUnlock()

	// TODO can we tolerate errors? The worst that happens are duplicates on client side
	file, err := os.Open(path.Join(config.BotCfg.WorkingDir, "fslog"))
	if err != nil {
		return nil
	}
	defer file.Close()

	logEntries := make(map[string]string)
	scanner := bufio.NewScanner(file)
	userPathPrefix := path.Join(config.BotCfg.StorageDir, fmt.Sprintf("%d", userID)) + "/"
	for scanner.Scan() {
		line := scanner.Text()
		var timestamp int64
		var op, oldPath, newPath string
		n, err := fmt.Sscanf(line, "%d %s %s %s", &timestamp, &op, &oldPath, &newPath)
		if op != Rename {
			continue
		}
		if err != nil || n != 4 || timestamp < afterTimestamp {
			continue
		}
		oldPath, err = url.QueryUnescape(oldPath)
		if err != nil {
			continue
		}
		newPath, err = url.QueryUnescape(newPath)
		if err != nil {
			continue
		}

		// TODO exclude ../ from log to prevent Filename Traversal attack

		if !strings.HasPrefix(oldPath, userPathPrefix) || !strings.HasPrefix(newPath, userPathPrefix) {
			continue
		}
		oldPath = strings.TrimPrefix(oldPath, userPathPrefix)
		newPath = strings.TrimPrefix(newPath, userPathPrefix)

		logEntries[newPath] = oldPath
	}

	return logEntries
}
