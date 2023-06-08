// Package fs provides a simple interface for files manipulations.
// Bot users should have all their artefacts saved in cross-platform
// plain text files, that's why we chose a filesystem over some database.
package fs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"zakirullin/dumpbot/pkg/str"
)

var (
	errUnsafePath   = errors.New("unsafe path, possible security issue")
	errCannotUnhash = errors.New("cannot unhash, maybe the file is missing")
)

const (
	DirTrash     = "_trash_"
	DirToday     = "today"
	DirLater     = "later"
	DirInbox     = "inbox"
	DirImg       = "img"
	DirJournal   = "journal"
	DirRead      = "-read-"
	DirWatch     = "-watch-"
	DirShop      = "-shop-"
	FilePomodoro = "Take a break.md"

	minSearchSimilarity = 70
)

// FS allows us to manipulate user files. We can use different
// backends, like an in-memory backend, which we use for testing.
// Check out types implementing afero.Fs for all available backends.
type FS struct {
	userID   int64
	rootPath string
	backend  afero.Fs
}

// File represents a file or directory
type File struct {
	Name        string
	Hash        string
	Title       string
	Ctime       int64
	IsMultiline bool
	IsDir       bool
	ParentDir   string
}

// TODO create Unsorted
func NewFS(userID int64, backend afero.Fs) (*FS, error) {
	rootDir := "./cmd/testdata"
	for _, dir := range []string{DirTrash, DirToday, DirLater} {
		path := fmt.Sprintf("%s/%s", rootDir, dir)
		exists, err := afero.Exists(backend, path)
		if err != nil {
			return nil, fmt.Errorf("NewFS: can't check whether base dirs exist: %w", err)
		}

		if !exists {
			err = backend.Mkdir(path, 0755)
			if err != nil {
				return nil, fmt.Errorf("NewFS: can't create base dirs: %w", err)
			}
		}
	}

	return &FS{userID, rootDir, backend}, nil
}

func (fs FS) Exists(dir, filename string) (bool, error) {
	path := fs.path(dir, filename)
	if !fs.isSafe(path) {
		return false, fmt.Errorf("fs.exists: unsafe path '%s': %w", path, errUnsafePath)
	}

	exists, err := afero.Exists(fs.backend, path)
	if err != nil {
		return false, fmt.Errorf("fs.exists: can't check whether the file '%s'/'%s' exists: %w", dir, filename, err)
	}

	return exists, nil
}

func (fs FS) Content(dir, filename string) (string, error) {
	path := fs.path(dir, filename)
	if !fs.isSafe(path) {
		return "", fmt.Errorf("fs.get: unsafe path '%s': %w", path, errUnsafePath)
	}

	content, err := afero.ReadFile(fs.backend, path)
	if err != nil {
		return "", fmt.Errorf("fs.get can't read file '%s': %w", path, err)
	}

	return string(content), nil
}

func (fs FS) Put(dir, filename, content string) error {
	path := fs.path(dir, filename)
	if !fs.isSafe(path) {
		return fmt.Errorf("fs.Put: unsafe path '%s': %w", path, errUnsafePath)
	}

	if err := afero.WriteFile(fs.backend, path, []byte(content), 0644); err != nil {
		return fmt.Errorf("fs.put to '%s/%s': %w", dir, filename, err)
	}

	return nil
}

func (fs FS) MakeDir(dir string) error {
	path := fs.path(dir, "")
	if !fs.isSafe(path) {
		return fmt.Errorf("fs.MakeDir: unsafe path '%s': %w", path, errUnsafePath)
	}

	err := fs.backend.Mkdir(path, 0755)
	if err != nil {
		return fmt.Errorf("b.MakeDir: can't create dir: %w", err)
	}

	return nil
}

func (fs FS) Del(dir, filename string) error {
	path := fs.path(dir, filename)
	if !fs.isSafe(path) {
		return fmt.Errorf("fs.Del: unsafe path '%s': %w", path, errUnsafePath)
	}

	err := fs.backend.Remove(path)
	if err != nil {
		return fmt.Errorf("fs.Del: can't remove '%s': %w", path, err)
	}

	return nil
}

func (fs FS) Rename(oldDir, oldFilename, newDir, newFilename string) error {
	oldPath := fs.path(oldDir, oldFilename)
	if !fs.isSafe(oldPath) {
		return fmt.Errorf("fs.rename: can't rename from '%s': %w", oldPath, errUnsafePath)
	}

	newPath := fs.path(newDir, newFilename)
	if !fs.isSafe(newPath) {
		return fmt.Errorf("fs.rename: can't rename to '%s': %w", newPath, errUnsafePath)
	}

	err := fs.backend.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("fs.rename: can't rename from '%s' to '%s': %w", oldPath, newPath, err)
	}

	return nil
}

func Filename(title string) string {
	return str.Ucfirst(title) + ".md"
}

func (fs FS) Unhash(dir, filenameHash string) (string, error) {
	// TODO add safety checks

	filenames, err := fs.FilesAndDirs(dir)
	if err != nil {
		return "", fmt.Errorf("fs.unhash: can't get filenames in '%s': %w", dir, err)
	}
	for _, file := range filenames {
		if strings.HasPrefix(fs.md5(file.Name), filenameHash) {
			return file.Name, nil
		}
	}

	// Compatibility, first we check for full Name match,
	// When do we need it?
	for _, file := range filenames {
		if file.Name == filenameHash {
			return file.Name, nil
		}
	}

	for _, file := range filenames {
		if strings.HasPrefix(file.Name, filenameHash) {
			return file.Name, nil
		}
	}

	return "", fmt.Errorf("fs.unhash: can't unhash '%s' in '%s': %w", filenameHash, dir, errCannotUnhash)
}

func (fs FS) FilesAndDirs(dir string) ([]File, error) {
	path := fs.path(dir, "")
	if !fs.isSafe(path) {
		return nil, fmt.Errorf("fs.getFilenames: %w", errUnsafePath)
	}

	entries, err := afero.ReadDir(fs.backend, path)
	if err != nil {
		return nil, fmt.Errorf("fs.getFilenames: can't read dir: %w", err)
	}

	var files []File
	// TODO remove gitignore
	ignoredFiles := []string{".", "..", ".obsidian", ".gitignore", ".DS_Store"}
	for _, entry := range entries {
		if slices.Contains(ignoredFiles, entry.Name()) {
			continue
		}

		file := File{
			entry.Name(),
			Hash(entry.Name()),
			Title(entry.Name()),
			Ctime(entry),
			entry.Size() > 0,
			entry.IsDir(),
			dir,
		}
		files = append(files, file)
	}

	return files, nil
}

func (fs FS) Dirs() ([]File, error) {
	files, err := fs.FilesAndDirs("")
	if err != nil {
		return nil, fmt.Errorf("fs.GetDirs: %w", err)
	}

	var dirs []File
	for _, file := range files {
		isDir, err := afero.IsDir(fs.backend, fs.path("", file.Name))
		if err != nil {
			return nil, fmt.Errorf("fs.GetDirs: can't check whether '%s' is a dir: %w", file.Name, err)
		}
		if !isDir {
			continue
		}

		dirs = append(dirs, file)
	}

	return dirs, nil
}

func (fs FS) isSafe(path string) bool {
	if strings.Contains(path, "../data") {
		return false
	}

	if !strings.HasPrefix(path, fs.rootPath) {
		return false
	}

	return true
}

func (fs FS) md5(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}

func (fs FS) IsMultiline(dir, filename string) (bool, error) {
	path := fs.path(dir, filename)
	stat, err := fs.backend.Stat(path)
	if err != nil {
		return false, fmt.Errorf("fs.IsMultiline: can't check filesize for '%s': %w", path, err)
	}

	return stat.Size() > 0, nil
}

// RestoreText restores original user's message text by given file
func (fs FS) RestoreText(dir, filename string) (string, error) {
	path := fs.path(dir, filename)
	if !fs.isSafe(path) {
		return "", fmt.Errorf("fs.RestoreMsgText: unsafe path '%s': %w", path, errUnsafePath)
	}

	title := Title(filename)
	content, err := fs.Content(dir, filename)
	if err != nil {
		return "", fmt.Errorf("fs.RestoreMsgText: can't get content: %w", err)
	}
	content = strings.TrimSpace(content)

	if strings.HasSuffix(title, "...") {
		title = strings.TrimSuffix(title, "...")
		if strings.HasPrefix(strings.ToLower(content), strings.ToLower(title)) {
			return content, nil
		}
	}

	msg := title
	if len(content) > 0 {
		msg = fmt.Sprintf("%s\n%s", title, content)
	}

	return msg, nil
}

// TODO rewrite for tests?
func AllUserIDs() ([]int64, error) {
	adminUserID, err := strconv.ParseInt(os.Getenv("ADMIN_USER_ID"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("fs.AllUserIDs: can't cast ADMIN_USER_ID to int64 %w", err)
	}

	return []int64{-1, adminUserID}, nil
}

func IsChecklistItem(filename string) bool {
	validChecklistItem := regexp.MustCompile(`^-.*?-(.+)`)

	return validChecklistItem.MatchString(filename)
}

func Title(filename string) string {
	// Once we move our items from checklists to _trash_,
	// they got named like -checklist-itemName
	stripChecklistChars := regexp.MustCompile(`^-.*?-(.+)`)
	title := stripChecklistChars.ReplaceAllString(filename, "$1")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "-"), "-")
	title = str.Ucfirst(strings.TrimSuffix(strings.TrimSpace(title), ".md"))

	return title
}

func Hash(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}

// SearchNotes performs search among all user notes
// Allowed query formats:
// "directory" - return all notes from directories prefixed by this directory
// "directory note_name" - search for this note_name in all matching directories
// "note_name" - search for this note_name across all directories
// "" - return all the notes
func (fs FS) SearchNotes(query string) ([]File, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	// Check for directory traversal attack
	if strings.Contains(query, "/") {
		return nil, nil
	}

	var supposedDir, search string
	exists, err := fs.Exists("", query)
	if err != nil {
		return nil, fmt.Errorf("fs.SearchNotes: %w", err)
	}
	if exists {
		supposedDir = query
	} else {
		parts := strings.SplitN(query, " ", 2)
		supposedDir = parts[0]
		if len(parts) > 1 {
			search = strings.TrimSpace(parts[1])
		}
	}

	// Find all similar notes directories
	var searchInDirs []string
	notesDirs, err := fs.FilesAndDirs("")
	if err != nil {
		return nil, fmt.Errorf("b.replyToInlineQuery: %w", err)
	}
	notesDirs = OnlyNotes(notesDirs)
	for _, noteDir := range notesDirs {
		if strings.HasPrefix(noteDir.Name, supposedDir) {
			searchInDirs = append(searchInDirs, noteDir.Name)
		}
	}

	// If no matching directories are found, we search through all directories
	if len(searchInDirs) == 0 {
		for _, noteDir := range notesDirs {
			searchInDirs = append(searchInDirs, noteDir.Name)
		}
		search = query
	}

	var notes []File
	for _, dir := range searchInDirs {
		// We can tolerate incomplete search
		files, _ := fs.FilesAndDirs(dir)
		files = OnlyFiles(files)
		notes = append(notes, files...)
	}
	notes = SortByCtime(notes)

	var matchedNotes []File
	for _, note := range notes {
		isWildcard := len(search) == 0
		isSubstring := strings.Contains(strings.ToLower(note.Title), search)
		isSimilar := str.Similar(strings.ToLower(note.Title), search) > minSearchSimilarity
		if isWildcard || isSubstring || isSimilar {
			matchedNotes = append(matchedNotes, note)
		}
	}

	return matchedNotes, nil
}

func ExcludeChecklists(dirs []File) []File {
	var newDirs []File
	for _, dir := range dirs {
		isChecklist := strings.HasPrefix(dir.Name, "-") && strings.HasSuffix(dir.Name, "-")
		if isChecklist {
			continue
		}

		newDirs = append(newDirs, dir)
	}

	return newDirs
}

func ExcludeSystemDirs(dirs []File) []File {
	var newDirs []File
	for _, dir := range dirs {
		if slices.Contains([]string{DirImg, DirTrash, DirJournal}, dir.Name) {
			continue
		}

		newDirs = append(newDirs, dir)
	}

	return newDirs
}

func ExcludeTaskDirs(dirs []File) []File {
	var newDirs []File
	for _, dir := range dirs {
		if slices.Contains([]string{DirToday, DirLater}, dir.Name) {
			continue
		}

		newDirs = append(newDirs, dir)
	}

	return newDirs
}

func ExcludePomodoro(files []File) []File {
	var newFiles []File
	for _, file := range files {
		if file.Name == FilePomodoro {
			continue
		}

		newFiles = append(newFiles, file)
	}

	return newFiles
}

func OnlyNotes(dirs []File) []File {
	return ExcludeSystemDirs(ExcludeTaskDirs(ExcludeChecklists(dirs)))
}

func OnlyChecklists(dirs []File) []File {
	entries := OnlyDirs(ExcludeSystemDirs(ExcludeTaskDirs(dirs)))

	var dirsWithChecklists []File
	for _, entry := range entries {
		isChecklist := strings.HasSuffix(entry.Name, "-") && strings.HasSuffix(entry.Name, "-")
		if isChecklist {
			dirsWithChecklists = append(dirsWithChecklists, entry)
		}
	}

	return dirsWithChecklists
}

func OnlyFiles(entries []File) []File {
	var files []File
	for _, file := range entries {
		if file.IsDir {
			continue
		}

		files = append(files, file)
	}

	return files
}

func OnlyDirs(entries []File) []File {
	var dirs []File
	for _, file := range entries {
		if !file.IsDir {
			continue
		}

		dirs = append(dirs, file)
	}

	return dirs
}

func OnlyFilenames(entries []File) []string {
	var filenames []string
	for _, entry := range entries {
		filenames = append(filenames, entry.Name)
	}

	return filenames
}

func SortByCtime(entries []File) []File {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Ctime < entries[j].Ctime
	})

	return entries
}

// Touch updates an existing file's access and modification times.
// If there's no such file it creates an empty file.
func (fs FS) Touch(dir, filename string) error {
	exists, err := fs.Exists(dir, filename)
	if err != nil {
		return fmt.Errorf("fs.Touch: can't check if file exists: %w", err)
	}
	if exists {
		err = fs.backend.Chtimes(fs.path(dir, filename), time.Now(), time.Now())
		if err != nil {
			return fmt.Errorf("fs.Touch: can't update file's ctime: %w", err)
		}
		return nil
	}
	err = fs.Put(dir, filename, "")
	if err != nil {
		return fmt.Errorf("fs.Touch: can't create empty file: %w", err)
	}
	return nil
}

// TODO fix empty dir
func (fs FS) path(dir, filename string) string {
	if len(dir) == 0 {
		return fmt.Sprintf("%s/%s", fs.rootPath, filename)
	}

	return fmt.Sprintf("%s/%s/%s", fs.rootPath, dir, filename)
}
