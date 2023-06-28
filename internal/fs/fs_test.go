package fs

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func init() {
	Ctime = func(fi os.FileInfo) int64 {
		return fi.ModTime().Unix()
	}
}

func TestIsChecklistItem(t *testing.T) {
	r := require.New(t)

	r.False(IsChecklistItem("-checklist-"))
	r.True(IsChecklistItem("-checklist-item"))
}

func TestTitle(t *testing.T) {
	r := require.New(t)

	title := Title("filename")
	r.Equal("Filename", title)
}

func TestTitleWithSpace(t *testing.T) {
	r := require.New(t)

	title := Title(" filename ")
	r.Equal("Filename", title)
}

func TestTitleChecklist(t *testing.T) {
	r := require.New(t)

	title := Title("-checklist-")
	r.Equal("Checklist", title)
}

func TestTitleChecklistItem(t *testing.T) {
	r := require.New(t)

	title := Title("-checklist-item")
	r.Equal("Item", title)
}

func TestMD5(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	res := fs.md5("First task.md")

	r.Equal("0824149b387", res)
}

func TestExcludeChecklists(t *testing.T) {
	r := require.New(t)

	noChecklists := ExcludeChecklists([]File{{Name: "not-a-checklist"}, {Name: "-checklist-"}})

	r.Equal([]File{{Name: "not-a-checklist"}}, noChecklists)
}

func TestExcludeSystemDirs(t *testing.T) {
	r := require.New(t)

	noChecklists := ExcludeSystemDirs([]File{{Name: "not-a-system-dir"}, {Name: "img"}, {Name: "_archive_"}, {Name: "journal"}})

	r.Equal([]File{{Name: "not-a-system-dir"}}, noChecklists)
}

func TestExcludeTaskDirs(t *testing.T) {
	r := require.New(t)

	noChecklists := ExcludeTaskDirs([]File{{Name: "not-a-task-dir"}, {Name: "today"}, {Name: "later"}})

	r.Equal([]File{{Name: "not-a-task-dir"}}, noChecklists)
}

func TestIsMultiline(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("today", "First task.md", "")
	r.Nil(err)

	isMultiline, err := fs.IsMultiline("today", "First task.md")
	r.Nil(err)
	r.False(isMultiline)

	err = fs.Put("today", "Second task.md", "c")
	r.Nil(err)

	isMultiline, err = fs.IsMultiline("today", "Second task.md")
	r.Nil(err)
	r.True(isMultiline)
}

func TestGetFilesInDir(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("today", "First task.md", "")
	r.Nil(err)

	files, err := fs.FilesAndDirs("today")
	r.Nil(err)
	r.Len(files, 1)
	r.Equal("First task.md", files[0].Name)
}

func TestRestoreMsgTextFromFilename(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", "File.md", "")
	r.Nil(err)

	msg, err := fs.RestoreContent("", "File.md")
	r.Nil(err)
	r.Equal("File", msg)
}

func TestRestoreMsgTextFromFilenameAndContent(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", "Title.md", "Content")
	r.Nil(err)

	msg, err := fs.RestoreContent("", "Title.md")
	r.Nil(err)
	r.Equal("Title\nContent", msg)
}

func TestRestoreMsgTextFromLongFilenameAndContent(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", "Title....md", "Title and Content")
	r.Nil(err)

	msg, err := fs.RestoreContent("", "Title....md")
	r.Nil(err)
	r.Equal("Title and Content", msg)
}

func TestRestoreMsgTextFromFilenameWithSpaces(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", " File.md ", "")
	r.Nil(err)

	msg, err := fs.RestoreContent("", " File.md ")
	r.Nil(err)
	r.Equal("File", msg)
}

func TestCreateBaseDirs(t *testing.T) {
	r := require.New(t)

	fs, err := NewFS("", afero.NewMemMapFs())
	r.Nil(err)

	err = fs.CreateUserDirs()
	r.Nil(err)

	dirs, err := fs.Dirs()
	r.Nil(err)
	dirs = OnlyDirs(dirs)
	dirNames := OnlyFilenames(dirs)

	r.ElementsMatch([]string{"later", "today", "_archive_"}, dirNames)
}

func TestSortByCtime(t *testing.T) {
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

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("today", "b.md", "")
	r.Nil(err)

	err = fs.Put("today", "a.md", "")
	r.Nil(err)

	entries, err := fs.FilesAndDirs("today")
	r.Nil(err)

	r.Equal([]string{"b.md", "a.md"}, OnlyFilenames(SortByCtime(entries)))
}

func TestExcludeEverythingButUserDirs(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", "a.md", "")
	r.Nil(err)

	err = fs.MakeDir("dir")
	r.Nil(err)

	entries, err := fs.FilesAndDirs("")
	r.Nil(err)

	dirs := OnlyDirs(ExcludeTaskDirs(ExcludeSystemDirs(entries)))
	r.Len(dirs, 1)
	r.Equal("dir", dirs[0].Name)
}

func TestOnlyFiles(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("", "a.md", "")
	r.Nil(err)

	err = fs.MakeDir("dir")
	r.Nil(err)

	entries, err := fs.FilesAndDirs("")
	r.Nil(err)

	dirs := OnlyFiles(entries)
	r.Len(dirs, 1)
	r.Equal("a.md", dirs[0].Name)
}

func TestOnlyChecklists(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("today", "a.md", "")
	r.Nil(err)

	err = fs.MakeDir("-list-")
	r.Nil(err)

	entries, err := fs.FilesAndDirs("")
	r.Nil(err)

	dirs := OnlyChecklists(entries)
	r.Len(dirs, 1)
	r.Equal("-list-", dirs[0].Name)
}

func TestFS_TouchNew(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	exists, err := fs.Exists("today", "a.md")
	r.Nil(err)
	r.False(exists)

	err = fs.Touch("today", "a.md")
	r.Nil(err)

	exists, err = fs.Exists("today", "a.md")
	r.Nil(err)
	r.True(exists)
}

func TestFS_TouchExisting(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Put("today", "a.md", "A")
	r.Nil(err)

	path := fs.Path("today", "a.md")
	fi, err := fs.backend.Stat(path)
	r.Nil(err)
	orig_ctime := Ctime(fi)

	time.Sleep(time.Second)
	err = fs.Touch("today", "a.md")
	r.Nil(err)

	fi, err = fs.backend.Stat(path)
	r.Nil(err)
	r.Less(orig_ctime, Ctime(fi))

	content, err := fs.Content("today", "a.md")
	r.Nil(err)
	r.Equal("A", content)
}

func TestFS_GetAllNotesInMatchingDir(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.Nil(err)
	err = fs.Touch("today", "b.md")
	r.Nil(err)
	err = fs.Touch("non-matching-dir", "c.md")
	r.Nil(err)

	notes, err := fs.SearchNotes("BRAIN")
	r.Nil(err)
	r.Len(notes, 1)
	r.Equal("a.md", notes[0].Name)
}

func TestFS_GetAllMatchingNotesInMatchingDir(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")

	r.Nil(err)
	err = fs.Touch("brain", "b.md")
	r.Nil(err)
	err = fs.Touch("today", "c.md")
	r.Nil(err)

	notes, err := fs.SearchNotes("BRAIN A")
	r.Nil(err)
	r.Len(notes, 1)
	r.Equal("a.md", notes[0].Name)
}

func TestFS_GetAllNotesInAllMatchingDirs(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.Nil(err)
	err = fs.Touch("brain", "b.md")
	r.Nil(err)
	err = fs.Touch("today", "c.md")
	r.Nil(err)

	notes, err := fs.SearchNotes("brain")
	r.Nil(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "b.md"}, noteFilenames)
}

func TestFS_GetAllMatchingNotesInAllMatchingDirs(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.Nil(err)
	err = fs.Touch("brain", "ab.md")
	r.Nil(err)
	err = fs.Touch("brain", "b.md")
	r.Nil(err)
	err = fs.Touch("today", "c.md")
	r.Nil(err)

	notes, err := fs.SearchNotes("brain a")
	r.Nil(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "ab.md"}, noteFilenames)
}

func TestFS_GetAllNotesInAllDirsForEmptyQuery(t *testing.T) {
	r := require.New(t)
	fs, _ := NewFS("", afero.NewMemMapFs())
	err := fs.Touch("brain", "a.md")
	r.Nil(err)
	err = fs.Touch("b", "b.md")
	r.Nil(err)
	err = fs.Touch("today", "c.md")
	r.Nil(err)

	notes, err := fs.SearchNotes("")
	r.Nil(err)
	r.Len(notes, 2)

	var noteFilenames []string
	for _, note := range notes {
		noteFilenames = append(noteFilenames, note.Name)
	}

	r.ElementsMatch([]string{"a.md", "b.md"}, noteFilenames)
}

func TestFS_PathTraversalAttack(t *testing.T) {
	r := require.New(t)

	fs, _ := NewFS("", afero.NewMemMapFs())
	fs.rootPath = ""

	path := fs.Path("../root/.ssh/", "authorized_keys")
	r.Equal("/..|root|.ssh|/authorized_keys", path)

	path = fs.Path("note", "../root/.ssh/authorized_keys")
	r.Equal("/note/..|root|.ssh|authorized_keys", path)
}
