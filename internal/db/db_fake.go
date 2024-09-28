package db

import (
	"zakirullin/stuffbot/pkg/tg"
)

// FakeDB is a fake database, used for testing
// We don't have to clear it after each test
type FakeDB struct {
	DirByMessageID      string
	FilenameByMessageID string
	InputExpectationCMD *tg.Cmd
	LastKeyboardMID     int
	RecentCMD           string
	RecentCMDParams     []string
}

func NewFakeDB() *FakeDB {
	return &FakeDB{LastKeyboardMID: -1}
}

func (db *FakeDB) LastKeyboardMsgID(userID int64) (int, bool) {
	if db.LastKeyboardMID == -1 {
		return 0, false
	}

	return db.LastKeyboardMID, true
}

func (db *FakeDB) SetLastKeyboardMsgID(userID int64, msgID int) {
	db.LastKeyboardMID = msgID
}

func (db *FakeDB) DelLastKeyboardMsgID(userID int64) {
	db.LastKeyboardMID = -1
}

func (db *FakeDB) InputExpectation(userID int64) *tg.Cmd {
	return db.InputExpectationCMD
}

func (db *FakeDB) SetInputExpectation(userID int64, cmd tg.Cmd) {
	db.InputExpectationCMD = &cmd
}

func (db *FakeDB) DelInputExpectation(userID int64) {
	db.InputExpectationCMD = nil
}

func (db *FakeDB) SetFilenameByMsgID(userID int64, msgID int, filename string) {
	db.FilenameByMessageID = filename
}

func (db *FakeDB) FilenameByMsgID(userID int64, msgID int) (string, bool) {
	return db.FilenameByMessageID, db.FilenameByMessageID != ""
}

func (db *FakeDB) DirByMsgID(userID int64, msgID int) (string, bool) {
	return db.DirByMessageID, db.DirByMessageID != ""
}

func (db *FakeDB) SetDirByMsgID(userID int64, msgID int, dir string) {
	db.DirByMessageID = dir
}

func (db *FakeDB) RecentCommand(_ int64) (string, bool) {
	return db.RecentCMD, db.RecentCMD != ""
}

func (db *FakeDB) SetRecentCommand(_ int64, cmd string) {
	db.RecentCMD = cmd
}

func (db *FakeDB) RecentCommandParams(_ int64) ([]string, bool) {
	return db.RecentCMDParams, len(db.RecentCMDParams) > 0
}

func (db *FakeDB) SetRecentCommandParams(_ int64, params []string) {
	db.RecentCMDParams = params
}
