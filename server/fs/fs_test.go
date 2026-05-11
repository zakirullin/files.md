package fs

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func init() {
	Ctime = func(fi os.FileInfo) int64 {
		return 0
	}
	Mtime = func(fi os.FileInfo) int64 { return 0 }
}

func TestIsChecklistItem(t *testing.T) {
	r := require.New(t)

	r.False(IsChecklistItem("-checklist-"))
	r.True(IsChecklistItem("-checklist-item"))
}

func TestDisplayName(t *testing.T) {
	r := require.New(t)

	title := DisplayName("filename")
	r.Equal("Filename", title)
}

func TestDisplayNameWithSpace(t *testing.T) {
	r := require.New(t)

	title := DisplayName(" filename ")
	r.Equal("Filename", title)
}

func TestMD5(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	res := fs.md5("First task.md")

	r.Equal("0824149b387", res)
}

func TestExcludeChecklists(t *testing.T) {
	r := require.New(t)

	noChecklists := ExcludeChecklists([]File{{Name: "not-a-checklist"}, {Name: "_checklist_"}})

	r.Equal([]File{{Name: "not-a-checklist"}}, noChecklists)
}

func TestExcludeSystemDirs(t *testing.T) {
	r := require.New(t)

	noChecklists := ExcludeSystemDirs([]File{{Name: "not-a-system-dir"}, {Name: "media"}, {Name: "archive"}, {Name: "journal"}})

	r.Equal([]File{{Name: "not-a-system-dir"}}, noChecklists)
}

func TestIsMultiline(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "First task.md", "")
	r.NoError(err)

	isMultiline, err := fs.IsMultiline("today", "First task.md")
	r.NoError(err)
	r.False(isMultiline)

	err = fs.Write("today", "Second task.md", "c")
	r.NoError(err)

	isMultiline, err = fs.IsMultiline("today", "Second task.md")
	r.NoError(err)
	r.True(isMultiline)

	err = fs.Write("today", "Third task.md", " \n ")
	r.NoError(err)

	isMultiline, err = fs.IsMultiline("today", "Third task.md")
	r.NoError(err)
	r.False(isMultiline)
}

func TestGetFilesInDir(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "First task.md", "")
	r.NoError(err)

	files, err := fs.FilesAndDirs("today")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("First task.md", files[0].Name)
}

func TestCreateBaseDirs(t *testing.T) {
	r := require.New(t)

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)
	r.NoError(fs.CreateSystemDirs())

	err = fs.CreateSystemDirs()
	r.NoError(err)

	dirs, err := fs.FilesAndDirs("/")
	r.NoError(err)
	dirs = OnlyDirs(dirs)
	dirNames := OnlyFilenames(dirs)

	r.ElementsMatch([]string{"archive", "media", "journal"}, dirNames)
}

func TestSortByCtimeDesc(t *testing.T) {
	r := require.New(t)

	saved := Ctime
	defer func() {
		Ctime = saved
	}()
	Ctime = func(fi os.FileInfo) int64 {
		if fi.Name() == "b.md" {
			return 1
		}

		return 2
	}

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "b.md", "")
	r.NoError(err)

	err = fs.Write("today", "a.md", "")
	r.NoError(err)

	entries, err := fs.FilesAndDirs("today")
	r.NoError(err)

	r.Equal([]string{"a.md", "b.md"}, OnlyFilenames(SortByCtimeDesc(entries)))
}

func TestExcludeEverythingButUserDirs(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("", "a.md", "")
	r.NoError(err)

	err = fs.MakeDir("dir")
	r.NoError(err)

	entries, err := fs.FilesAndDirs("/")
	r.NoError(err)

	dirs := OnlyDirs(ExcludeSystemDirs(entries))
	r.Len(dirs, 1)
	r.Equal("dir", dirs[0].Name)
}

func TestOnlyFiles(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("", "a.md", "")
	r.NoError(err)

	err = fs.MakeDir("dir")
	r.NoError(err)

	entries, err := fs.FilesAndDirs("/")
	r.NoError(err)

	dirs := OnlyUserMDFiles(entries)
	r.Len(dirs, 1)
	r.Equal("a.md", dirs[0].Name)
}

func TestOnlyChecklists(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	r.NoError(fs.Write("today", "a.md", ""))
	r.NoError(fs.Write("/", "list_.md", ""))
	// A non-markdown file whose name happens to end in "_" must NOT be
	// treated as a checklist.
	r.NoError(fs.Write("/", "Molchanov_.mobi", ""))

	entries, err := fs.FilesAndDirs("/")
	r.NoError(err)

	dirs := OnlyChecklists(entries)
	r.Len(dirs, 1)
	r.Equal("list_.md", dirs[0].Name)
}

func TestFSTouchNew(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	exists, err := fs.Exists("today", "a.md")
	r.NoError(err)
	r.False(exists)

	err = fs.Touch("today", "a.md")
	r.NoError(err)

	exists, err = fs.Exists("today", "a.md")
	r.NoError(err)
	r.True(exists)
}

func TestFSTouchExisting(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "a.md", "A")
	r.NoError(err)

	err = fs.Touch("today", "a.md")
	r.NoError(err)

	content, err := fs.Read("today", "a.md")
	r.NoError(err)
	r.Equal("A", content)
}

func TestFSGetAllNotesInMatchingDir(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.NoError(err)
	err = fs.Touch("today", "b.md")
	r.NoError(err)
	err = fs.Touch("non-matching-dir", "c.md")
	r.NoError(err)

	notes, err := fs.SearchFilesByName("BRAIN")
	r.NoError(err)
	r.Len(notes, 1)
	r.Equal("a.md", notes[0].Name)
}

func TestFSGetAllMatchingNotesInMatchingDir(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")

	r.NoError(err)
	err = fs.Touch("brain", "b.md")
	r.NoError(err)
	err = fs.Touch("today", "c.md")
	r.NoError(err)

	notes, err := fs.SearchFilesByName("BRAIN A")
	r.NoError(err)
	r.Len(notes, 1)
	r.Equal("a.md", notes[0].Name)
}

func TestFSGetAllNotesInAllMatchingDirs(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.NoError(err)
	err = fs.Touch("brain", "b.md")
	r.NoError(err)
	err = fs.Touch("today", "c.md")
	r.NoError(err)

	notes, err := fs.SearchFilesByName("brain")
	r.NoError(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "b.md"}, noteFilenames)
}

func TestFSGetAllMatchingNotesInAllMatchingDirs(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.NoError(err)
	err = fs.Touch("brain", "ab.md")
	r.NoError(err)
	err = fs.Touch("brain", "b.md")
	r.NoError(err)
	err = fs.Touch("today", "c.md")
	r.NoError(err)

	notes, err := fs.SearchFilesByName("brain a")
	r.NoError(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "ab.md"}, noteFilenames)
}

func TestFSGetAllNotesInAllDirsForEmptyQuery(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.NoError(err)
	err = fs.Touch("b", "b.md")
	r.NoError(err)

	notes, err := fs.SearchFilesByName("")
	r.NoError(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "b.md"}, noteFilenames)
}

func TestFSPathTraversalAttack(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/abc", afero.NewMemMapFs())
	fs.rootPath = "/abc"

	path, err := fs.SafePath("../root/.ssh/", "authorized_keys")
	r.Error(err)
	r.Equal("", path)

	path, err = fs.SafePath("note", "../root/.ssh/authorized_keys")
	r.NoError(err)
	r.Equal("/abc/root/.ssh/authorized_keys", path)

	path, err = fs.SafePath("note", "../../root/.ssh/authorized_keys")
	r.Error(err)
	r.Empty(path)
}

func TestFSOnlyUserDirs(t *testing.T) {
	r := require.New(t)

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fs.MakeDir("str")
	r.NoError(err)

	err = fs.MakeDir("123")
	r.NoError(err)

	err = fs.MakeDir("123.56")
	r.NoError(err)

	dirs, _ := fs.FilesAndDirs("/")
	userDirs := OnlyUserDirs(dirs)

	r.Len(userDirs, 1)
	r.Equal("123", userDirs[0].Name)
}

func TestIsSafeWrongRoot(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/a", afero.NewMemMapFs())
	p, err := fs.SafePath("b", "")
	r.NoError(err)
	r.Equal("/a/b", p)

}

func TestIsSafePathTraversalAttack(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/a", afero.NewMemMapFs())
	p, err := fs.SafePath("a/../b", "")
	r.NoError(err)
	r.Equal("/a/b", p)

	p, err = fs.SafePath("/a/../../b", "")
	r.Error(err)
	r.Empty(p)

	p, err = fs.SafePath("./a/../b", "")
	r.NoError(err)
	r.Equal("/a/b", p)

	p, err = fs.SafePath("./a/../../b", "")
	r.Error(err)
	r.Empty(p)
}

func TestSafePath(t *testing.T) {
	r := require.New(t)

	memFS := afero.NewMemMapFs()
	memFS.Mkdir("/app-secret", 0o755)

	fs, _ := NewFS("/app", memFS)

	p, err := fs.SafePath("subdir", "file")
	r.NoError(err)
	r.Equal("/app/subdir/file", p)

	p, err = fs.SafePath("subdir", "../file")
	r.NoError(err)
	r.Equal("/app/file", p)

	p, err = fs.SafePath("../app-secret", "file")
	r.Error(err)
	r.Equal("", p)

}

func TestIsSafePathTraversalAttackWithRelativePaths(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS(".", afero.NewMemMapFs())
	p, err := fs.SafePath("./a/../b", "")
	r.NoError(err)
	r.Equal("b", p)

	p, err = fs.SafePath("./a/../../b", "")
	r.Error(err)
	r.Empty(p)
}

func TestUnhashRootDirectory(t *testing.T) {
	r := require.New(t)

	fs, err := NewFS(".", afero.NewMemMapFs())
	r.NoError(err)
	// TODO is it used at all? What a strange behaviour?
	unhashed, err := fs.Unhash("/", "/")
	r.NoError(err)

	r.Equal("/", unhashed)
}

func TestSanitizeFilename(t *testing.T) {
	r := require.New(t)

	r.Equal("ab", SanitizeFilename("a\x00b"))
	r.Equal("a／b", SanitizeFilename("a/b"))
	r.Equal("a＼b", SanitizeFilename("a\\b"))
	r.Equal("a／b＼", SanitizeFilename("\x00a\x00/b\\"))
}

func TestUnsanitizeFilename(t *testing.T) {
	r := require.New(t)

	r.Equal("a/b", UnsanitizeFilename("a／b"))
	r.Equal("a\\b", UnsanitizeFilename("a＼b"))
	r.Equal("a/b\\", UnsanitizeFilename("a／b＼"))
}

func TestExists(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "First task.md", "")
	r.NoError(err)

	exists, err := fs.Exists("today", "First task.md")
	r.NoError(err)
	r.True(exists)
}

func TestExistsRoot(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "First task.md", "")
	r.NoError(err)

	exists, err := fs.Exists("/", "")
	r.NoError(err)
	r.True(exists)
}

func TestWriteAndReadFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "test.md", "Test content")
	r.NoError(err)

	content, err := fs.Read("today", "test.md")
	r.NoError(err)
	r.Equal("Test content", content)
}

func TestWriteUnsafePath(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/1", afero.NewMemMapFs())
	err := fs.Write("../unsafe", "test.md", "Test content")
	r.Error(err)
	r.Contains(err.Error(), "unsafe filePath")
}

func TestReadNonExistentFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	content, err := fs.Read("today", "nonexistent.md")
	r.Error(err)
	r.Equal("", content)
}

func TestDelFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "delete.md", "To be deleted")
	r.NoError(err)

	err = fs.Del("today", "delete.md")
	r.NoError(err)

	exists, err := fs.Exists("today", "delete.md")
	r.NoError(err)
	r.False(exists)
}

func TestDelNonExistentFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Del("today", "nonexistent.md")
	r.Error(err)
	r.Contains(err.Error(), "can't remove")
}

func TestRenameFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Write("today", "oldname.md", "Old content")
	r.NoError(err)

	err = fs.Rename("today", "oldname.md", "today", "newname.md")
	r.NoError(err)

	exists, err := fs.Exists("today", "newname.md")
	r.NoError(err)
	r.True(exists)

	content, err := fs.Read("today", "newname.md")
	r.NoError(err)
	r.Equal("Old content", content)
}

func TestRenameUnsafePath(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/1", afero.NewMemMapFs())
	err := fs.Write("today", "oldname.md", "Old content")
	r.NoError(err)

	err = fs.Rename("../unsafe", "oldname.md", "today", "newname.md")
	r.Error(err)
	r.Contains(err.Error(), "unsafe path")
}

func TestMakeDir(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.MakeDir("newdir")
	r.NoError(err)

	exists, err := fs.Exists("newdir", "")
	r.NoError(err)
	r.True(exists)
}

func TestMakeDirUnsafePath(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/1", afero.NewMemMapFs())
	err := fs.MakeDir("../unsafe")
	r.Error(err)
	r.Contains(err.Error(), "unsafe path")
}

func TestUnsafePathSanitization(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/1", afero.NewMemMapFs())

	p, err := fs.SafePath("../", "test.md")
	r.Error(err)
	r.Empty(p)

	p, err = fs.SafePath("safe", "../unsafe.md")
	r.NoError(err)
	r.Equal("/1/unsafe.md", p)
}

func TestUnhash(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("today", "hashedfile.md")
	r.NoError(err)

	hash := fs.md5("hashedfile.md")
	unhashed, err := fs.Unhash("today", hash)
	r.NoError(err)
	r.Equal("hashedfile.md", unhashed)
}

func TestUnhashNonExistentFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	_, err := fs.Unhash("today", "nonexistenthash")
	r.Error(err)
	r.Contains(err.Error(), "can't unhash")
}

func TestSanitizeAndUnsanitizeFilename(t *testing.T) {
	r := require.New(t)

	sanitized := SanitizeFilename("test/file:name\\with/special\\chars")
	r.Equal("test／file꞉name＼with／special＼chars", sanitized)

	unsanitized := UnsanitizeFilename(sanitized)
	r.Equal("test/file:name\\with/special\\chars", unsanitized)
}

func TestFilesAndDirs(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.MakeDir("dir1")
	r.NoError(err)

	err = fs.Write("dir1", "file1.md", "File content")
	r.NoError(err)

	files, err := fs.FilesAndDirs("dir1")
	r.NoError(err)
	r.Len(files, 1)
	r.Equal("file1.md", files[0].Name)
}

func TestDirs(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.MakeDir("dir1")
	r.NoError(err)

	err = fs.Write("dir1", "file1.md", "File content")
	r.NoError(err)

	dirs, err := fs.Dirs()
	r.NoError(err)
	r.Len(dirs, 1)
	r.Equal("dir1", dirs[0].Name)
}

func TestTouchFile(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("/", afero.NewMemMapFs())
	err := fs.Touch("today", "touchfile.md")
	r.NoError(err)

	exists, err := fs.Exists("today", "touchfile.md")
	r.NoError(err)
	r.True(exists)
}

func FuzzWrite(f *testing.F) {
	f.Add("valid_dir", "valid_file.txt", "This is valid content")
	f.Add("valid_dir", "../../unsafe_file.txt", "Unsafe path content")
	f.Add("valid_dir/subdir", "valid_file.md", "Nested content")
	f.Add("invalid<>|*?dir", "invalid_file.txt", "Invalid dir")
	f.Add("valid_dir", "file_with_emoji_🀀.txt", "Unicode content")
	f.Add("../", "file.md", "content")
	f.Add("../../", "file.md", "content")
	f.Add("../../../", "file.md", "content")
	f.Add("dir", "../file.md", "content")
	f.Add("dir", "../../file.md", "content")
	f.Add("dir", "../../../file.md", "content")

	f.Fuzz(func(t *testing.T, dir, filename, content string) {
		filename = filename + ".md"

		r := require.New(t)

		fs := afero.NewMemMapFs()
		_ = fs.Mkdir("/user", 0o755)

		userFS, err := NewFS("/user", fs)
		r.NoError(err)

		err = userFS.Write(dir, filename, content)

		unsafePath := path.Join("/user/", dir, filename)
		otherUserDir := !strings.HasPrefix(unsafePath, "/user/")
		if unsafePath == "/user" {
			otherUserDir = false
		}
		if otherUserDir || strings.Contains(unsafePath, "../") {
			if !errors.Is(err, ErrUnsafePath) {
				t.Errorf("Expected unsafe path error for dir: '%s', filename: '%s', calculated path: '%s', got: '%v'", dir, filename, unsafePath, err)
			}
			return
		}

		r.NoError(err, "Unexpected error for valid inputs dir: '%s', filename: '%s'", dir, filename)

		filePath, err := userFS.SafePath(dir, filename)
		r.NoError(err)

		actualContent, readErr := afero.ReadFile(userFS.backend, filePath)
		r.NoError(readErr, "Error reading file: %s", filePath)
		r.Equal(content, string(actualContent), "Content mismatch for file: %s", filePath)

		// Check that a file is indeed created in user fs, and not outside
		files, err := userFS.FilesAndDirs("/")
		r.NoError(err, "Unexpected error for valid inputs dir: '%s', filename: '%s', calculated path: '%s", dir, filename, unsafePath)
		r.Len(files, 1, "File has been written outside of user dir. Provided dir: '%s', filename: '%s', unsafe path: '%s", dir, filename, filePath)
	})
}

func TestCtimes(t *testing.T) {
	r := require.New(t)

	saved := Mtime
	defer func() {
		Mtime = saved
	}()

	Mtime = func(fi os.FileInfo) int64 {
		switch fi.Name() {
		case "file1.md":
			return 1000
		case "file2.md":
			return 2000
		case "nested.md":
			return 3000
		default:
			return 0
		}
	}

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fs.Write("/", "file1.md", "content1")
	r.NoError(err)
	err = fs.Write("today", "file2.md", "content2")
	r.NoError(err)
	err = fs.Write("/", "not-markdown.txt", "should be ignored")
	r.NoError(err)
	err = fs.MakeDir("today/subdir")
	r.NoError(err)
	err = fs.Write("today/subdir", "nested.md", "nested content")
	r.NoError(err)

	// Create hidden file (should be ignored)
	err = fs.Write("today", ".hidden.md", "hidden content")
	r.NoError(err)

	// Test Ctimes
	ctimes, err := fs.Mtimes("/", ".md")
	r.NoError(err)

	fmt.Println(ctimes)
	r.Len(ctimes, 3)
	r.Equal(int64(1000), ctimes["file1.md"])
	r.Equal(int64(2000), ctimes["today/file2.md"])
	r.Equal(int64(3000), ctimes["today/subdir/nested.md"])

	// Should not include non-markdown files or hidden files
	_, exists := ctimes["today/not-markdown.txt"]
	r.False(exists)

	_, exists = ctimes["today/.hidden.md"]
	r.False(exists)
}

func TestMtimesAllExtensions(t *testing.T) {
	r := require.New(t)

	saved := Mtime
	defer func() {
		Mtime = saved
	}()

	Mtime = func(fi os.FileInfo) int64 {
		switch fi.Name() {
		case "file1.md":
			return 1000
		case "file2.md":
			return 2000
		case "nested.md":
			return 3000
		case "file.txt":
			return 3000
		default:
			return 0
		}
	}

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fs.Write("/", "file1.md", "content1")
	r.NoError(err)
	err = fs.Write("today", "file2.md", "content2")
	r.NoError(err)
	err = fs.Write("/", "file.txt", "should be ignored")
	r.NoError(err)
	err = fs.MakeDir("today/subdir")
	r.NoError(err)
	err = fs.Write("today/subdir", "nested.md", "nested content")
	r.NoError(err)

	// Create hidden file (should be ignored)
	err = fs.Write("today", ".hidden.md", "hidden content")
	r.NoError(err)

	ctimes, err := fs.Mtimes("/")
	r.NoError(err)

	r.Len(ctimes, 4)
	r.Equal(int64(1000), ctimes["file1.md"])
	r.Equal(int64(2000), ctimes["today/file2.md"])
	r.Equal(int64(3000), ctimes["today/subdir/nested.md"])
	r.Equal(int64(3000), ctimes["file.txt"])

	// Should not include non-markdown files or hidden files
	_, exists := ctimes["today/not-markdown.txt"]
	r.False(exists)

	_, exists = ctimes["today/.hidden.md"]
	r.False(exists)
}

func TestMtimesInSubDir(t *testing.T) {
	r := require.New(t)

	saved := Mtime
	defer func() {
		Mtime = saved
	}()

	Mtime = func(fi os.FileInfo) int64 {
		switch fi.Name() {
		case "file1.md":
			return 1000
		case "file2.md":
			return 2000
		case "nested.md":
			return 3000
		default:
			return 0
		}
	}

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fs.Write("/", "rootfile.md", "content1")
	r.NoError(err)
	err = fs.Write("today", "file1.md", "content1")
	r.NoError(err)
	err = fs.Write("today", "file2.md", "content2")
	r.NoError(err)
	err = fs.Write("today", "not-markdown.txt", "should be ignored")
	r.NoError(err)
	err = fs.MakeDir("today/subdir")
	r.NoError(err)
	err = fs.Write("today/subdir", "nested.md", "nested content")
	r.NoError(err)

	// Create hidden file (should be ignored)
	err = fs.Write("today", ".hidden.md", "hidden content")
	r.NoError(err)

	// Test Ctimes
	ctimes, err := fs.Mtimes("today", ".md")
	r.NoError(err)

	// Should only include .md files, not .txt or hidden files
	r.Len(ctimes, 3)
	r.Equal(int64(1000), ctimes["file1.md"])
	r.Equal(int64(2000), ctimes["file2.md"])
	r.Equal(int64(3000), ctimes["subdir/nested.md"])

	// Should not include non-markdown files or hidden files
	_, exists := ctimes["today/not-markdown.txt"]
	r.False(exists)

	_, exists = ctimes["today/.hidden.md"]
	r.False(exists)
}

func TestCtimesEmptyDir(t *testing.T) {
	r := require.New(t)

	fs, err := NewFS("/", afero.NewMemMapFs())
	r.NoError(err)

	err = fs.MakeDir("empty")
	r.NoError(err)

	ctimes, err := fs.Mtimes("empty", ".md")
	r.NoError(err)
	r.Empty(ctimes)
}
