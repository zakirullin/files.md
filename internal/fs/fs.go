// Package fs provides a simple interface for files manipulations.
// Bot users should have all their artefacts saved in cross-platform
// plain text files, that's why we chose a filesystem over some database.
package fs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"zakirullin/stuffbot/pkg/text"
)

var (
	errUnsafePath   = errors.New("unsafe path, possible security issue")
	errCannotUnhash = errors.New("cannot unhash, maybe the file is missing")
)

const (
	DirArchive   = "_archive_"
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

func NewFS(rootPath string, backend afero.Fs) (*FS, error) {
	exists, err := afero.Exists(backend, rootPath)
	if err != nil {
		return nil, fmt.Errorf("new fs: %w", err)
	}
	if !exists {
		err = backend.Mkdir(rootPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("new fs: %w", err)
		}
	}

	return &FS{rootPath, backend}, nil
}

func (fs FS) CreateUserDirs() error {
	for _, dir := range []string{DirArchive, DirToday, DirLater, DirInbox, DirImg, DirRead, DirWatch, DirShop} {
		path := fmt.Sprintf("%s/%s", fs.rootPath, dir)
		exists, err := afero.Exists(fs.backend, path)
		if err != nil {
			return fmt.Errorf("create default dirs: %w", err)
		}

		if !exists {
			err = fs.backend.Mkdir(path, 0755)
			if err != nil {
				return fmt.Errorf("create default dirs: %w", err)
			}
		}
	}

	return nil
}

func (fs FS) Exists(dir, filename string) (bool, error) {
	path := fs.Path(dir, filename)
	if !fs.isSafe(path) {
		return false, fmt.Errorf("exists: unsafe path '%s': %w", path, errUnsafePath)
	}

	exists, err := afero.Exists(fs.backend, path)
	if err != nil {
		return false, fmt.Errorf("exists: can't check whether the file '%s'/'%s' exists: %w", dir, filename, err)
	}

	return exists, nil
}

func (fs FS) Content(dir, filename string) (string, error) {
	path := fs.Path(dir, filename)
	if !fs.isSafe(path) {
		return "", fmt.Errorf("get content: unsafe path '%s': %w", path, errUnsafePath)
	}

	content, err := afero.ReadFile(fs.backend, path)
	if err != nil {
		return "", fmt.Errorf("get content: can't read file '%s': %w", path, err)
	}

	return string(content), nil
}

func (fs FS) Put(dir, filename, content string) error {
	path := fs.Path(dir, filename)
	if !fs.isSafe(path) {
		return fmt.Errorf("put: unsafe path '%s': %w", path, errUnsafePath)
	}

	if err := afero.WriteFile(fs.backend, path, []byte(content), 0644); err != nil {
		return fmt.Errorf("put to '%s/%s': %w", dir, filename, err)
	}

	return nil
}

func (fs FS) MakeDir(dir string) error {
	path := fs.Path(dir, "")
	if !fs.isSafe(path) {
		return fmt.Errorf("make dir: unsafe path '%s': %w", path, errUnsafePath)
	}

	err := fs.backend.Mkdir(path, 0755)
	if err != nil {
		return fmt.Errorf("make dir: %w", err)
	}

	return nil
}

func (fs FS) Del(dir, filename string) error {
	path := fs.Path(dir, filename)
	if !fs.isSafe(path) {
		return fmt.Errorf("delete file: unsafe path '%s': %w", path, errUnsafePath)
	}

	err := fs.backend.Remove(path)
	if err != nil {
		return fmt.Errorf("delete file: can't remove '%s': %w", path, err)
	}

	return nil
}

func (fs FS) Rename(oldDir, oldFilename, newDir, newFilename string) error {
	oldPath := fs.Path(oldDir, oldFilename)
	if !fs.isSafe(oldPath) {
		return fmt.Errorf("can't rename from '%s': %w", oldPath, errUnsafePath)
	}

	newPath := fs.Path(newDir, newFilename)
	if !fs.isSafe(newPath) {
		return fmt.Errorf("can't rename to '%s': %w", newPath, errUnsafePath)
	}

	err := fs.backend.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("can't rename from '%s' to '%s': %w", oldPath, newPath, err)
	}

	return nil
}

func Filename(title string) string {
	return text.Ucfirst(title) + ".md"
}

func (fs FS) Unhash(dir, filenameHash string) (string, error) {
	// TODO add safety checks

	filenames, err := fs.FilesAndDirs(dir)
	if err != nil {
		return "", fmt.Errorf("can't unhash: %w", err)
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

	return "", fmt.Errorf("can't unhash '%s' in '%s': %w", filenameHash, dir, errCannotUnhash)
}

func (fs FS) FilesAndDirs(dir string) ([]File, error) {
	path := fs.Path(dir, "")
	if !fs.isSafe(path) {
		return nil, fmt.Errorf("can't get files: %w", errUnsafePath)
	}

	entries, err := afero.ReadDir(fs.backend, path)
	if err != nil {
		return nil, fmt.Errorf("can't get files: %w", err)
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
		return nil, fmt.Errorf("can't get dirs: %w", err)
	}

	var dirs []File
	for _, file := range files {
		isDir, err := afero.IsDir(fs.backend, fs.Path("", file.Name))
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

// Maybe we should replace / with | and use filepath.Clean by default
// instead of throwing an error up the stack
// TODO test all Fs' public the methods for Path traversal
// TODO after you cover everything with the tests, we may remove this method
// because we build our own paths
func (fs FS) isSafe(path string) bool {
	if !strings.HasPrefix(path, fs.rootPath) {
		return false
	}

	// Path traversal attack
	if strings.Contains(path, "../") {
		return false
	}

	return true
}

func (fs FS) md5(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}

func (fs FS) IsMultiline(dir, filename string) (bool, error) {
	path := fs.Path(dir, filename)
	stat, err := fs.backend.Stat(path)
	if err != nil {
		return false, fmt.Errorf("can't check for multiline: %w", err)
	}

	return stat.Size() > 0, nil
}

// RestoreContent restores original user's message text by given file
func (fs FS) RestoreContent(dir, filename string) (string, error) {
	path := fs.Path(dir, filename)
	if !fs.isSafe(path) {
		return "", fmt.Errorf("can't restore text: unsafe path '%s': %w", path, errUnsafePath)
	}

	title := Title(filename)
	content, err := fs.Content(dir, filename)
	if err != nil {
		return "", fmt.Errorf("can't restore text: %w", err)
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

func IsChecklistItem(filename string) bool {
	validChecklistItem := regexp.MustCompile(`^-.*?-(.+)`)

	return validChecklistItem.MatchString(filename)
}

func Title(filename string) string {
	// Once we move our items from checklists to _archive_,
	// they got named like -checklist-itemName
	stripChecklistChars := regexp.MustCompile(`^-.*?-(.+)`)
	title := stripChecklistChars.ReplaceAllString(filename, "$1")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "-"), "-")
	title = text.Ucfirst(strings.TrimSuffix(strings.TrimSpace(title), ".md"))

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
		return nil, fmt.Errorf("search notes: %w", err)
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
		return nil, fmt.Errorf("search notes: %w", err)
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
		isSimilar := text.Similar(strings.ToLower(note.Title), search) > minSearchSimilarity
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
		if slices.Contains([]string{DirImg, DirArchive, DirJournal}, dir.Name) {
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

// OnlyUserDirs returns only directories that look like user IDs
func OnlyUserDirs(entries []File) []File {
	var dirs []File
	for _, file := range entries {
		if !file.IsDir {
			continue
		}
		if _, err := strconv.Atoi(file.Name); err != nil {
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

func UserPath(storagePath string, userID int64) string {
	return fmt.Sprintf("%s/%d", storagePath, userID)
}

// Touch updates an existing file's access and modification times.
// If there's no such file it creates an empty file.
func (fs FS) Touch(dir, filename string) error {
	exists, err := fs.Exists(dir, filename)
	if err != nil {
		return fmt.Errorf("touch: %w", err)
	}
	if exists {
		err = fs.backend.Chtimes(fs.Path(dir, filename), time.Now(), time.Now())
		if err != nil {
			return fmt.Errorf("touch: can't update file's ctime: %w", err)
		}
		return nil
	}
	err = fs.Put(dir, filename, "")
	if err != nil {
		return fmt.Errorf("touch: can't create empty file: %w", err)
	}
	return nil
}

func (fs FS) Path(dir, filename string) string {
	dir = strings.ReplaceAll(dir, "/", "|")
	filename = strings.ReplaceAll(filename, "/", "|")
	if len(dir) == 0 {
		return fmt.Sprintf("%s/%s", fs.rootPath, filename)
	}

	return fmt.Sprintf("%s/%s/%s", fs.rootPath, dir, filename)
}
