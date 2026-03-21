// Package db provides an in-memory database for storing user-specific temporary data.
package db

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"zakirullin/stuffbot/pkg/tg"
)

// In-memory database
var (
	filenameByMsgID       sync.Map
	dirByMsgID            sync.Map
	inputExpectations     sync.Map
	recentCommands        sync.Map
	recentCommandsTargets sync.Map
	sentPhotoMsgIDs       sync.Map
)

// DB Do we need a type at all?
type DB struct {
	UserID int64
}

func NewDB(userID int64) *DB {
	return &DB{UserID: userID}
}

// TODO add locks
func (db *DB) LastKeyboardMsgID() (int, bool) {
	msgIDStr, err := os.ReadFile(tmpFilePath(db.UserID, "msgid"))
	if err != nil {
		return 0, false
	}

	msgID, err := strconv.Atoi(string(msgIDStr))
	if err != nil {
		return 0, false
	}

	return msgID, true
}

func (db *DB) SetLastKeyboardMsgID(ID int) {
	_ = os.WriteFile(tmpFilePath(db.UserID, "msgid"), []byte(strconv.Itoa(ID)), 0o644)
}

func (db *DB) DelLastKeyboardMsgID() {
	_ = os.Remove(tmpFilePath(db.UserID, "msgid"))
}

func (db *DB) InputExpectation() *tg.Cmd {
	val, ok := inputExpectations.Load(inputExpectationKey(db.UserID))
	if !ok {
		return nil
	}

	cmd := val.(tg.Cmd)
	return &cmd
}

func (db *DB) SetInputExpectation(cmd tg.Cmd) {
	inputExpectations.Store(inputExpectationKey(db.UserID), cmd)
}

func (db *DB) DelInputExpectation() {
	inputExpectations.Delete(inputExpectationKey(db.UserID))
}

func (db *DB) FilenameByMsgID(msgID int) (string, bool) {
	filename, ok := filenameByMsgID.Load(filenameByMsgIDKey(db.UserID, msgID))
	if !ok {
		return "", false
	}

	return filename.(string), true
}

func (db *DB) DirByMsgID(msgID int) (string, bool) {
	filename, ok := dirByMsgID.Load(dirByMsgIDKey(db.UserID, msgID))
	if !ok {
		return "", false
	}

	return filename.(string), true
}

func (db *DB) SetRecentFilenameByMsgID(msgID int, filename string) {
	filenameByMsgID.Store(filenameByMsgIDKey(db.UserID, msgID), filename)
}

func (db *DB) SetRecentDirByMsgID(msgID int, filename string) {
	dirByMsgID.Store(dirByMsgIDKey(db.UserID, msgID), filename)
}

func (db *DB) RecentCommand() (string, bool) {
	cmd, ok := recentCommands.Load(db.UserID)
	if !ok {
		return "", false
	}

	return cmd.(string), true
}

func (db *DB) SetRecentCommand(cmd string) {
	recentCommands.Store(db.UserID, cmd)
}

func (db *DB) RecentCommandParams() ([]string, bool) {
	params, ok := recentCommandsTargets.Load(db.UserID)
	if !ok {
		return nil, false
	}

	return params.([]string), true
}

func (db *DB) SetRecentCommandParams(params []string) {
	recentCommandsTargets.Store(db.UserID, params)
}

func (db *DB) AddImgMsgID(msgID int) {
	key := photoMsgIDKey(db.UserID)
	if val, ok := sentPhotoMsgIDs.Load(key); ok {
		ids := val.([]int)
		sentPhotoMsgIDs.Store(key, append(ids, msgID))
	} else {
		sentPhotoMsgIDs.Store(key, []int{msgID})
	}
}

func (db *DB) ImgMsgID() ([]int, bool) {
	key := photoMsgIDKey(db.UserID)
	val, ok := sentPhotoMsgIDs.Load(key)
	if !ok {
		return nil, false
	}

	ids, _ := val.([]int)
	return ids, true
}

func (db *DB) DelImgMsgID() {
	key := photoMsgIDKey(db.UserID)
	sentPhotoMsgIDs.Delete(key)
}

func photoMsgIDKey(userID int64) string {
	return fmt.Sprintf("%d:sentPhotoMsgIDs", userID)
}

func inputExpectationKey(userID int64) string {
	return fmt.Sprintf("%d:inputExpectations", userID)
}

func dirByMsgIDKey(userID int64, msgID int) string {
	return fmt.Sprintf("%d:dirByMsgID:%d", userID, msgID)
}

func filenameByMsgIDKey(userID int64, msgID int) string {
	return fmt.Sprintf("%d:filenameByMsgID:%d", userID, msgID)
}

func tmpFilePath(userID int64, name string) string {
	return fmt.Sprintf("%s/%d.%s", os.TempDir(), userID, name)
}
