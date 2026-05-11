// Package fs provides a simple interface for files manipulations.
// Bot users should have all their artefacts saved in cross-platform
// plain text files, that's why we use good old-fashioned filesystem.
// Each user should have its own isolated root folder.
// TODO maybe make ... access for all methods? So we can use both paths and segments
// Why not BasePathFS?
package fs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"github.com/zakirullin/files.md/server/config"
	"github.com/zakirullin/files.md/server/pkg/txt"
)

var (
	NewUserFS = newUserFS

	LogRename = func(time int64, oldPath, newPath string) {} // callback to track renames
	LogDelete = func(time int64, path string) {}             // callback to track deletes

	ErrQuotaExceeded = errors.New("storage quota exceeded")
	ErrUnsafePath    = errors.New("unsafe path, possible security issue")
	ErrCannotUnhash  = errors.New("cannot unhash, maybe the file is missing")
)

const (
	DirUserRoot = "/"
	DirArchive  = "archive"
	DirMedia    = "media"
	DirJournal  = "journal"
	DirHabits   = "habits"
	DirInsights = "insights"

	ChatFilename  = "Chat.md"
	LaterFilename = "Later.md"
	DoneFilename  = "Done.md"

	ShopFilename  = "Shop.md"
	WatchFilename = "Watch.md"
	ReadFilename  = "Read.md"

	PomodoroTask = "Finished a break"

	MDExt = ".md"

	minSearchSimilarity = 70
)

// FS allows us to manipulate user files. We can use different
// backends, like an in-memory backend, which we use for testing.
// Check out types implementing afero.Fs for all available backends.
type FS struct {
	rootPath string
	backend  afero.Fs
	quotaKB  int64
}

// File represents a file or directory
type File struct {
	Name        string // Filename with extension
	Hash        string
	DisplayName string
	Ctime       int64
	IsMultiline bool
	IsDir       bool
	ParentDir   string
}

// newUserFS creates a new FS for a specific user with os.FS backend.
func newUserFS(userID int64) (*FS, error) {
	userAbsPath := path.Join(config.ServerCfg.StorageDir, txt.I64(userID))
	backend := afero.NewOsFs()

	quotaKB := config.ServerCfg.StorageQuotaKB
	if isUnlimitedQuota(userID, config.ServerCfg.UnlimitedQuotaIDs) {
		quotaKB = 0
	}

	return NewFS(userAbsPath, backend, quotaKB)
}

func NewFS(absRootPath string, backend afero.Fs, quotaKB ...int64) (*FS, error) {
	exists, err := afero.Exists(backend, absRootPath)
	if err != nil {
		return nil, fmt.Errorf("new fs: %w", err)
	}
	if !exists {
		err = backend.Mkdir(absRootPath, 0o755)
		if err != nil {
			return nil, fmt.Errorf("new fs: %w", err)
		}
	}

	var q int64
	if len(quotaKB) > 0 {
		q = quotaKB[0]
	}

	return &FS{absRootPath, backend, q}, nil
}

func NewFile(name, hash, displayName string, ctime int64, isMultiline, isDir bool, parentDir string) File {
	return File{
		Name:        name,
		Hash:        hash,
		DisplayName: displayName,
		Ctime:       ctime,
		IsMultiline: isMultiline,
		IsDir:       isDir,
		ParentDir:   parentDir,
	}
}

// CreateDirsIfNotExist creates specified directories for a user if they do not exist.
// If dirs are not specified, it creates default directories.
func (fs FS) CreateDirsIfNotExist(dirs ...string) error {
	for _, dir := range dirs {
		if dir == DirUserRoot {
			continue
		}

		userPath := path.Join(fs.rootPath, dir)
		exists, err := afero.Exists(fs.backend, userPath)
		if err != nil {
			return fmt.Errorf("create default dirs: %w", err)
		}

		if !exists {
			err = fs.backend.Mkdir(userPath, 0o755)
			if err != nil {
				return fmt.Errorf("create default dirs: %w", err)
			}
		}
	}

	return nil
}

func (fs FS) CreateSystemDirs() error {
	return fs.CreateDirsIfNotExist(DirArchive, DirMedia, DirJournal)
}

func (fs FS) Exists(dir, filename string) (bool, error) {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return false, fmt.Errorf("exists: unsafe path '%s': %w", filepath.Join(dir, filename), ErrUnsafePath)
	}

	exists, err := afero.Exists(fs.backend, filePath)
	if err != nil {
		return false, fmt.Errorf("exists: can't check whether the file '%s'/'%s' exists: %w", dir, filename, err)
	}

	return exists, nil
}

func (fs FS) Read(dir, filename string) (string, error) {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return "", fmt.Errorf("fs read: unsafe filePath dir: '%s', filename: '%s': %w", dir, filename, ErrUnsafePath)
	}

	content, err := afero.ReadFile(fs.backend, filePath)
	if err != nil {
		return "", fmt.Errorf("fs read: can't read file '%s': %w", filePath, err)
	}

	return string(content), nil
}

func (fs FS) Write(dir, filename, content string) error {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return fmt.Errorf("fs write: unsafe filePath '%s': %w", filepath.Join(dir, filename), ErrUnsafePath)
	}

	dirs := strings.Split(filePath, "/")
	dirs = dirs[:len(dirs)-1]
	pathToDir := strings.Join(dirs, "/")
	if err := fs.backend.MkdirAll(pathToDir, 0o755); err != nil {
		return fmt.Errorf("fs write: can't create dirs '%s': %w", pathToDir, err)
	}

	// Track old size for quota accounting.
	var oldSize int64
	if info, err := fs.backend.Stat(filePath); err == nil {
		oldSize = info.Size()
	}

	newSize := int64(len(content))
	if err := checkQuota(fs.rootPath, fs.backend, fs.quotaKB, newSize-oldSize); err != nil {
		return err
	}

	// Append mode for forwards?
	if err := afero.WriteFile(fs.backend, filePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("fs write to '%s/%s': %w", dir, filename, err)
	}

	recordQuotaUsage(fs.rootPath, newSize-oldSize)

	return nil
}

func (fs FS) MakeDir(dir string) error {
	filePath, err := fs.SafePath(dir, "")
	if err != nil {
		return fmt.Errorf("fs make dir: unsafe filePath '%s': %w", filePath, ErrUnsafePath)
	}

	err = fs.backend.Mkdir(filePath, 0o755)
	if err != nil {
		return fmt.Errorf("fs can't make dir: %w", err)
	}

	return nil
}

func (fs FS) Del(dir, filename string) error {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return fmt.Errorf("fs del file: unsafe filePath '%s': %w", filePath, ErrUnsafePath)
	}

	var fileSize int64
	if info, err := fs.backend.Stat(filePath); err == nil {
		fileSize = info.Size()
	}

	err = fs.backend.Remove(filePath)
	if err != nil {
		return fmt.Errorf("fs file: can't remove '%s': %w", filePath, err)
	}

	recordQuotaUsage(fs.rootPath, -fileSize)

	// Log deletion.
	ctime, err := fs.Ctime(filePath, "")
	// Nothing terrible will happen if we don't log a rename. The client would just have duplicate files.
	if err == nil {
		absPath := path.Join(fs.rootPath, filePath)
		LogDelete(ctime, absPath)
	}

	return nil
}

func (fs FS) Rename(oldDir, oldFilename, newDir, newFilename string) error {
	oldPath, err := fs.SafePath(oldDir, oldFilename)
	if err != nil {
		return fmt.Errorf("fs can't rename from '%s': %w", oldPath, ErrUnsafePath)
	}

	newPath, err := fs.SafePath(newDir, newFilename)
	if err != nil {
		return fmt.Errorf("fs can't rename to '%s': %w", newPath, ErrUnsafePath)
	}

	err = fs.CreateDirsIfNotExist(oldDir, newDir)
	if err != nil {
		return fmt.Errorf("fs can't rename: %w", err)
	}

	err = fs.backend.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("can't rename from '%s' to '%s': %w", oldPath, newPath, err)
	}

	// Log renaming.
	ctime, err := fs.Ctime(newDir, newFilename)
	// Nothing terrible will happen if we don't log a rename. The client would just have duplicate files.
	if err == nil {
		absOldPath := path.Join(fs.rootPath, oldDir, oldFilename)
		absNewPath := path.Join(fs.rootPath, newDir, newFilename)
		LogRename(ctime, absOldPath, absNewPath)
	}

	return nil
}

func (fs FS) Unhash(dir, filenameHash string) (string, error) {
	if dir == DirUserRoot && filenameHash == DirUserRoot {
		return DirUserRoot, nil
	}

	filenames, err := fs.FilesAndDirs(dir)
	if err != nil {
		return "", fmt.Errorf("can't unhash: %w", err)
	}
	for _, file := range filenames {
		if strings.HasPrefix(fs.md5(file.Name), filenameHash) {
			return file.Name, nil
		}
	}

	// Fallback, treat hash as filename
	for _, file := range filenames {
		if strings.HasPrefix(file.Name, filenameHash) {
			return file.Name, nil
		}
	}

	return "", fmt.Errorf("can't unhash '%s' in '%s': %w", filenameHash, dir, ErrCannotUnhash)
}

func (fs FS) FilesAndDirs(dir string) ([]File, error) {
	userPath, err := fs.SafePath(dir, "")
	if err != nil {
		return nil, fmt.Errorf("can't get files for '%s': %w", path.Join(fs.rootPath, dir), ErrUnsafePath)
	}

	entries, err := afero.ReadDir(fs.backend, userPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Create folders only on write-actions
			return nil, nil
		}

		return nil, fmt.Errorf("can't get files for '%s': %w", path.Join(fs.rootPath, dir), err)
	}

	var files []File
	ignoredFiles := []string{".", "..", ".obsidian", ".gitignore", ".DS_Store", ".git"}
	for _, entry := range entries {
		if slices.Contains(ignoredFiles, entry.Name()) {
			continue
		}

		file := NewFile(
			entry.Name(),
			Hash(entry.Name()),
			DisplayName(entry.Name()),
			Ctime(entry),
			entry.Size() > 0,
			entry.IsDir(),
			dir,
		)
		files = append(files, file)
	}

	return files, nil
}

func (fs FS) Dirs() ([]File, error) {
	files, err := fs.FilesAndDirs(DirUserRoot)
	if err != nil {
		return nil, fmt.Errorf("can't get dirs: %w", err)
	}

	var dirs []File
	for _, file := range files {
		filePath, err := fs.SafePath(DirUserRoot, file.Name)
		if err != nil {
			return nil, fmt.Errorf("can't get dirs: unsafe path '%s': %w", filePath, ErrUnsafePath)
		}

		isDir, err := afero.IsDir(fs.backend, filePath)
		if err != nil {
			return nil, fmt.Errorf("can't get dirs: %w", err)
		}
		if !isDir {
			continue
		}

		dirs = append(dirs, file)
	}

	return dirs, nil
}

func (fs FS) IsMultiline(dir, filename string) (bool, error) {
	content, err := fs.Read(dir, filename)
	if err != nil {
		return false, fmt.Errorf("can't check for multiline: %w", err)
	}
	content = strings.TrimSpace(content)

	return len(content) > 0, nil
}

func (fs FS) md5(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}

func Filename(header string) string {
	return txt.Ucfirst(header) + MDExt
}

func IsChecklistItem(filename string) bool {
	validChecklistItem := regexp.MustCompile(`^-.*?-(.+)`)

	return validChecklistItem.MatchString(filename)
}

// SearchFilesByName performs search among all user .md files
// Allowed query formats:
// "directory" - return all notes from directories prefixed by this directory
// "directory note_name" - search for this note_name in all matching directories
// "note_name" - search for this note_name across all directories
// "" - return all the notes
func (fs FS) SearchFilesByName(query string) ([]File, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	// Check for directory traversal attack
	if strings.Contains(query, "/") {
		return nil, fmt.Errorf("search notes: unsafe query '%s': %w", query, ErrUnsafePath)
	}

	var supposedDir, search string
	dirExists, err := fs.Exists(DirUserRoot, query)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	if dirExists {
		supposedDir = query
	} else {
		parts := strings.SplitN(query, " ", 2)
		supposedDir = parts[0]
		if len(parts) > 1 {
			search = strings.TrimSpace(parts[1])
		}
	}

	rootPath, err := fs.SafePath(DirUserRoot, "")
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}

	// Walk the whole tree once - up to 10 levels deep - and collect .md
	// files. Filtering by dir prefix happens after the walk.
	var notes []File
	_ = afero.Walk(fs.backend, rootPath, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(rootPath, p)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) >= 10 {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(base) != MDExt {
			return nil
		}
		relativeToUserRootPath, _ := filepath.Rel(rootPath, filepath.Dir(p))
		if relativeToUserRootPath == "." || relativeToUserRootPath == "" {
			relativeToUserRootPath = DirUserRoot
		}
		notes = append(notes, NewFile(
			base, Hash(base), DisplayName(base),
			Ctime(info), info.Size() > 0, false, relativeToUserRootPath,
		))
		return nil
	})
	notes = OnlyUserMDFiles(notes)

	// Restrict to the matching top-level dir if the user typed one.
	if supposedDir != "" {
		var pruned []File
		for _, n := range notes {
			top := strings.SplitN(n.ParentDir, "/", 2)[0]
			if top == DirUserRoot {
				top = ""
			}
			if strings.HasPrefix(top, supposedDir) {
				pruned = append(pruned, n)
			}
		}
		if len(pruned) > 0 {
			notes = pruned
		} else {
			// No dir matched - fall back to a name search across all notes.
			search = query
		}
	}
	notes = SortByCtimeDesc(notes)

	var matchedNotes []File
	for _, note := range notes {
		isWildcard := len(search) == 0
		isSubstring := strings.Contains(strings.ToLower(note.DisplayName), search)
		isSimilar := txt.Similar(strings.ToLower(note.DisplayName), search) > minSearchSimilarity
		if isWildcard || isSubstring || isSimilar {
			matchedNotes = append(matchedNotes, note)
		}
	}

	return matchedNotes, nil
}

// TODO check if safe
// Touch updates an existing file's access and modification times.
// If there's no such file it creates an empty file.
func (fs FS) Touch(dir, filename string) error {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return fmt.Errorf("touch: unsafe path '%s': %w", filePath, ErrUnsafePath)
	}

	exists, err := fs.Exists(dir, filename)
	if err != nil {
		return fmt.Errorf("touch: %w", err)
	}

	if exists {
		err = fs.backend.Chtimes(filePath, time.Now(), time.Now())
		if err != nil {
			return fmt.Errorf("touch: can't update file's ctime: %w", err)
		}
		return nil
	}
	err = fs.Write(dir, filename, "")
	if err != nil {
		return fmt.Errorf("touch: can't create empty file: %w", err)
	}
	return nil
}

// Ctime returns the change time of a file as Unix timestamp in milliseconds.
// It updates on file creation, modification, metadata changes, moving to a different dir, renames.
func (fs FS) Ctime(dir, filename string) (int64, error) {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return 0, fmt.Errorf("fs file: unsafe filePath '%s': %w", filePath, ErrUnsafePath)
	}

	info, err := fs.backend.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("fs file: can't stat file '%s': %w", filePath, err)
	}

	return Ctime(info), nil
}

// Mtime returns the modification time of a file as Unix timestamp in milliseconds.
// It only updates on file modification.
func (fs FS) Mtime(dir, filename string) (int64, error) {
	filePath, err := fs.SafePath(dir, filename)
	if err != nil {
		return 0, fmt.Errorf("fs mtime: unsafe filePath '%s': %w", filePath, ErrUnsafePath)
	}

	info, err := fs.backend.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("fs mtime: can't stat file '%s': %w", filePath, err)
	}

	return Mtime(info), nil
}

// Mtimes recursively scans a directory and returns the mtime
// for all files with specified extension as Unix timestamps.
// Returns [relPath] => ctime
// TODO add tests
func (fs FS) Mtimes(root string, extensions ...string) (map[string]int64, error) {
	rootPath, err := fs.SafePath(root, "")
	if err != nil {
		return nil, fmt.Errorf("fs mtimes: unsafe rootPath '%s': %w", rootPath, ErrUnsafePath)
	}

	mtimes := make(map[string]int64)
	err = afero.Walk(fs.backend, rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		base := filepath.Base(path)
		// Skip hidden files.
		if strings.HasPrefix(base, ".") && path != rootPath {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Only process specified file extension.
		if len(extensions) > 0 {
			ext := filepath.Ext(path)
			if !slices.Contains(extensions, ext) {
				return nil
			}
		}

		// TODO what if a file inside folder?
		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		if relPath == "" {
			relPath = "."
		}

		mtimes[relPath] = Mtime(info)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return mtimes, nil
}

// SafePath returns safe path to a file or directory, error if the path is unsafe.
// Sanitize Early, call SanitizeFilename
// as soon as you get on dir and filename from user input
// TODO test all FS' public the methods for unsafePath traversal
// TODO after you cover everything with the tests, we may remove this method
// because we build our own paths (???)
// TODO release remove error?
// isSafe doesn't eval symlinks, so an attacker can create a symlink to a file
// outside the rootPath. If we use filepath.EvalSymlinks to expand symlinks and
// check the real path for safety - we are still prone to TOCTOU (time-of-check to time-of-use)
// attacks due to the race condition. The only real way to prevent this is to disallow symlinks
// at the OS level. We can do this by mounting a folder with nosymfollow flag, see README.md.
func (fs FS) SafePath(dir, filename string) (string, error) {
	var relativePath string
	if dir == "/" {
		if filename == "" {
			// Just the root directory
			return fs.rootPath, nil
		}
		relativePath = filename
	} else {
		relativePath = filepath.Join(dir, filename)
	}

	if !filepath.IsLocal(relativePath) {
		return "", ErrUnsafePath
	}

	return filepath.Join(fs.rootPath, relativePath), nil
}
