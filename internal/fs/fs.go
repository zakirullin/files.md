// Package fs provides a simple interface for files manipulations.
// Bot users should have all their artefacts saved in cross-platform
// plain text files, that's why we chose a filesystem over some database.
// Each user should have its own isolated root folder.
package fs

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/exp/slices"

	"zakirullin/stuffbot/pkg/txt"
)

var (
	DefaultBackend  = afero.NewOsFs()
	errUnsafePath   = errors.New("unsafe path, possible security issue")
	errCannotUnhash = errors.New("cannot unhash, maybe the file is missing")
)

const (
	DirRoot      = ""
	DirArchive   = "archive"
	DirToday     = "today"
	DirLater     = "later"
	DirInbox     = "inbox"
	DirImg       = "img"
	DirJournal   = "journal"
	DirHabits    = "habits"
	DirInsights  = "insights"
	DirRead      = "-read-"
	DirWatch     = "-watch-"
	DirShop      = "-shop-"
	FilePomodoro = "Took a break.md"
	FileConfig   = "config.json"

	minSearchSimilarity  = 70
	escapedForwardSlash  = "{|}"
	escapedBackwardSlash = "{||}"
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
	Name        string // Filename with extension
	Hash        string
	Title       string
	Ctime       int64
	IsMultiline bool
	IsDir       bool
	ParentDir   string
}

func NewFS(absRootPath string, backend afero.Fs) (*FS, error) {
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

	return &FS{absRootPath, backend}, nil
}

func NewFile(name, hash, title string, ctime int64, isMultiline, isDir bool, parentDir string) File {
	return File{name, hash, title, ctime, isMultiline, isDir, parentDir}
}

func (fs FS) CreateUserDirs() error {
	for _, dir := range []string{
		DirArchive,
		DirToday,
		DirLater,
		DirInbox,
		DirImg,
		DirRead,
		DirWatch,
		DirShop,
		DirHabits,
		DirInsights,
	} {
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

func (fs FS) Exists(dir, filename string) (bool, error) {
	path := fs.UnsafePath(dir, filename)
	isSafe, err := fs.isSafe(path)
	if err != nil {
		return false, fmt.Errorf("exists: can't check if the file is safe to access '%s': %w", path, err)
	}
	if !isSafe {
		return false, fmt.Errorf("exists: unsafe path '%s': %w", path, errUnsafePath)
	}

	exists, err := afero.Exists(fs.backend, path)
	if err != nil {
		return false, fmt.Errorf("exists: can't check whether the file '%s'/'%s' exists: %w", dir, filename, err)
	}

	return exists, nil
}

func (fs FS) Read(dir, filename string) (string, error) {
	path := fs.UnsafePath(dir, filename)
	isSafe, err := fs.isSafe(path)
	if err != nil {
		return "", fmt.Errorf("fs read: can't check if the file is safe to access '%s': %w", path, err)
	}
	if !isSafe {
		return "", fmt.Errorf("fs read: unsafe path '%s': %w", path, errUnsafePath)
	}

	content, err := afero.ReadFile(fs.backend, path)
	if err != nil {
		return "", fmt.Errorf("fs read: can't read file '%s': %w", path, err)
	}

	return string(content), nil
}

func (fs FS) Write(dir, filename, content string) error {
	path := fs.UnsafePath(dir, filename)
	isSafe, err := fs.isSafe(path)
	if err != nil {
		return fmt.Errorf("fs write: check if file is safe to access '%s': %w", path, err)
	}

	if !isSafe {
		return fmt.Errorf("fs write: unsafe path '%s': %w", path, errUnsafePath)
	}

	dirs := strings.Split(path, "/")
	dirs = dirs[:len(dirs)-1]
	pathToDir := strings.Join(dirs, "/")
	if err := fs.backend.MkdirAll(pathToDir, 0o755); err != nil {
		return fmt.Errorf("put: can't create dirs '%s': %w", pathToDir, err)
	}

	if err := afero.WriteFile(fs.backend, path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("put to '%s/%s': %w", dir, filename, err)
	}

	return nil
}

func (fs FS) MakeDir(dir string) error {
	path := fs.UnsafePath(dir, "")
	isSafe, err := fs.isSafe(path)
	if err != nil {
		return fmt.Errorf("fs make dir: check if file is safe to access '%s': %w", path, err)
	}
	if !isSafe {
		return fmt.Errorf("fs make dir: unsafe path '%s': %w", path, errUnsafePath)
	}

	err = fs.backend.Mkdir(path, 0o755)
	if err != nil {
		return fmt.Errorf("fs can't make dir: %w", err)
	}

	return nil
}

func (fs FS) Del(dir, filename string) error {
	path := fs.UnsafePath(dir, filename)
	isSafe, err := fs.isSafe(path)
	if err != nil {
		return fmt.Errorf("fs del: check if file is safe to access '%s': %w", path, err)
	}
	if !isSafe {
		return fmt.Errorf("fs del file: unsafe path '%s': %w", path, errUnsafePath)
	}

	err = fs.backend.Remove(path)
	if err != nil {
		return fmt.Errorf("fs file: can't remove '%s': %w", path, err)
	}

	return nil
}

func (fs FS) Rename(oldDir, oldFilename, newDir, newFilename string) error {
	oldPath := fs.UnsafePath(oldDir, oldFilename)
	isSafe, err := fs.isSafe(oldPath)
	if err != nil {
		return fmt.Errorf("fs rename: check if file is safe to access '%s': %w", oldPath, err)
	}
	if !isSafe {
		return fmt.Errorf("fs can't rename from '%s': %w", oldPath, errUnsafePath)
	}

	newPath := fs.UnsafePath(newDir, newFilename)
	isSafe, err = fs.isSafe(newPath)
	if err != nil {
		return fmt.Errorf("fs rename: check if file is safe to access '%s': %w", newPath, err)
	}
	if !isSafe {
		return fmt.Errorf("fs can't rename to '%s': %w", newPath, errUnsafePath)
	}

	err = fs.backend.Rename(oldPath, newPath)
	if err != nil {
		return fmt.Errorf("can't rename from '%s' to '%s': %w", oldPath, newPath, err)
	}

	return nil
}

func (fs FS) Unhash(dir, filenameHash string) (string, error) {
	if dir == DirRoot && filenameHash == DirRoot {
		return DirRoot, nil
	}

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
	//for _, file := range filenames {
	//	if file.Name == filenameHash {
	//		return file.Name, nil
	//	}
	//}

	for _, file := range filenames {
		if strings.HasPrefix(file.Name, filenameHash) {
			return file.Name, nil
		}
	}

	return "", fmt.Errorf("can't unhash '%s' in '%s': %w", filenameHash, dir, errCannotUnhash)
}

func (fs FS) FilesAndDirs(dir string) ([]File, error) {
	userPath := fs.UnsafePath(dir, "")
	isSafe, err := fs.isSafe(userPath)
	if err != nil {
		return nil, fmt.Errorf("exists: check if file is safe to access '%s': %w", userPath, err)
	}
	if !isSafe {
		return nil, fmt.Errorf("can't get files for '%s': %w", path.Join(fs.rootPath, dir), errUnsafePath)
	}

	entries, err := afero.ReadDir(fs.backend, userPath)
	if err != nil {
		return nil, fmt.Errorf("can't get files for '%s': %w", path.Join(fs.rootPath, dir), err)
	}

	var files []File
	// TODO remove gitignore
	ignoredFiles := []string{".", "..", ".obsidian", ".gitignore", ".DS_Store"}
	for _, entry := range entries {
		if slices.Contains(ignoredFiles, entry.Name()) {
			continue
		}

		file := NewFile(
			entry.Name(),
			Hash(entry.Name()),
			Title(entry.Name()),
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
	files, err := fs.FilesAndDirs(DirRoot)
	if err != nil {
		return nil, fmt.Errorf("can't get dirs: %w", err)
	}

	var dirs []File
	for _, file := range files {
		isDir, err := afero.IsDir(fs.backend, fs.UnsafePath(DirRoot, file.Name))
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
// TODO test all FS' public the methods for UnsafePath traversal
// TODO after you cover everything with the tests, we may remove this method
// because we build our own paths
func (fs FS) isSafe(path string) (bool, error) {
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, fs.rootPath) {
		return false, nil
	}

	// Not safe if we have symlink
	exists, err := afero.Exists(fs.backend, path)
	if err != nil {
		return false, err
	}
	if exists {
		// TODO release uncomment
		//lstater, ok := fs.backend.(afero.Lstater)
		//if !ok {
		//	return false, fmt.Errorf("safety can't be checked, fs should support lstater interface: %w", err)
		//}

		//stat, _, err := lstater.LstatIfPossible(path)
		//if err != nil {
		//	return false, fmt.Errorf("safety can't be checked, fs should support lstat: %w", err)
		//}
		//// On MacOS dangling symlinks have drwxr-xr-x stats (i.e. treated as directory)
		//if stat.Mode()&os.ModeSymlink != 0 {
		//	return false, nil
		//}
	}

	// Path traversal attack (filepath.Clean only cleans absolute paths from ../)
	// https://owasp.org/www-community/attacks/Path_Traversal
	// A better way would be to convert the path to absolute path, but AferoFS doesn't support that
	if strings.Contains(path, "../") || strings.Contains(path, "/..") {
		return false, nil
	}

	return true, nil
}

func (fs FS) IsMultiline(dir, filename string) (bool, error) {
	path := fs.UnsafePath(dir, filename)
	stat, err := fs.backend.Stat(path)
	if err != nil {
		return false, fmt.Errorf("can't check for multiline: %w", err)
	}

	return stat.Size() > 0, nil
}

func (fs FS) md5(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
}

func Filename(title string) string {
	return txt.Ucfirst(title) + ".md"
}

func IsChecklistItem(filename string) bool {
	validChecklistItem := regexp.MustCompile(`^-.*?-(.+)`)

	return validChecklistItem.MatchString(filename)
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
	dirExists, err := fs.Exists(DirRoot, query)
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

	// Find match by notes directory name
	var searchInDirs []string
	notesDirs, err := fs.FilesAndDirs(DirRoot)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	notesDirs = OnlyNoteDirs(notesDirs)
	notesDirs = append(notesDirs, NewFile(DirRoot, "", DirRoot, 0, false, true, ""))
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
	notes = SortByCtimeDesc(notes)

	var matchedNotes []File
	for _, note := range notes {
		isWildcard := len(search) == 0
		isSubstring := strings.Contains(strings.ToLower(note.Title), search)
		isSimilar := txt.Similar(strings.ToLower(note.Title), search) > minSearchSimilarity
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
	exists, err := fs.Exists(dir, filename)
	if err != nil {
		return fmt.Errorf("touch: %w", err)
	}
	if exists {
		err = fs.backend.Chtimes(fs.UnsafePath(dir, filename), time.Now(), time.Now())
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

// UnsafePath builds a user-specific path.
// It'S NOT SAFE to use this method with user input.
// Sanitize Early, call SanitizeFilename
// as soon as you get on dir and filename from user input
func (fs FS) UnsafePath(dir, filename string) string {
	p := path.Join(fs.rootPath, dir, filename)

	return p
}

func SanitizeFilename(filename string) string {
	// Under Linux and other Unix-related systems,
	// there are only two characters that cannot
	// appear in the name of a file or directory,
	// and those are NUL '\0' and slash '/'.
	// For Windows we only handle '\',
	// consider sanitazing other characters
	filename = strings.ReplaceAll(filename, "\x00", "")
	filename = strings.ReplaceAll(filename, "/", escapedForwardSlash)
	filename = strings.ReplaceAll(filename, "\\", escapedBackwardSlash)

	// colon is a reserved character in Windows, so we need to replace it with Modifier Letter Colon (U+A789)
	filename = strings.ReplaceAll(filename, ":", "꞉")

	return filename
}

func UnsanitizeFilename(filename string) string {
	filename = strings.ReplaceAll(filename, escapedForwardSlash, "/")
	filename = strings.ReplaceAll(filename, escapedBackwardSlash, "\\")

	return filename
}

func Title(filename string) string {
	// Once we move our items from checklists to archive,
	// they got named like -checklist-itemName
	stripChecklistChars := regexp.MustCompile(`^-.*?-(.+)`)
	title := stripChecklistChars.ReplaceAllString(filename, "$1")
	title = strings.TrimPrefix(strings.TrimSuffix(title, "-"), "-")
	title = txt.Ucfirst(strings.TrimSuffix(strings.TrimSpace(title), ".md"))

	return title
}

func Hash(filename string) string {
	hash := md5.Sum([]byte(filename))
	return hex.EncodeToString(hash[:])[:11]
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
		if slices.Contains([]string{DirImg, DirArchive, DirJournal, DirInsights}, dir.Name) {
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

func ExcludeConfig(files []File) []File {
	var newFiles []File
	for _, file := range files {
		if file.Name == FileConfig && file.ParentDir == DirRoot {
			continue
		}

		newFiles = append(newFiles, file)
	}

	return newFiles
}

func OnlyNoteDirs(dirs []File) []File {
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

func SortByCtimeDesc(entries []File) []File {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Ctime > entries[j].Ctime
	})

	return entries
}

func Exists(path string) (bool, error) {
	return afero.Exists(DefaultBackend, path)
}

// TODO fix permissions?
// TODO defaultBackend?
func WriteFile(filename string, data []byte) error {
	return afero.WriteFile(DefaultBackend, filename, data, 0o644)
}
