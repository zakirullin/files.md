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
)

// DB Do we need a type at all?
type DB struct{}

func NewDB() *DB {
	return &DB{}
}

// TODO add locks
func (db *DB) LastKeyboardMsgID(userID int64) (int, bool) {
	msgIDStr, err := os.ReadFile(tmpFilePath(userID, "msgid"))
	if err != nil {
		return 0, false
	}

	msgID, err := strconv.Atoi(string(msgIDStr))
	if err != nil {
		return 0, false
	}

	return msgID, true
}

func (db *DB) SetLastKeyboardMsgID(userID int64, ID int) {
	_ = os.WriteFile(tmpFilePath(userID, "msgid"), []byte(strconv.Itoa(ID)), 0o644)
}

func (db *DB) DelLastKeyboardMsgID(userID int64) {
	_ = os.Remove(tmpFilePath(userID, "msgid"))
}

func (db *DB) InputExpectation(userID int64) *tg.Cmd {
	val, ok := inputExpectations.Load(inputExpectationKey(userID))
	if !ok {
		return nil
	}

	cmd := val.(tg.Cmd)
	return &cmd
}

func (db *DB) SetInputExpectation(userID int64, cmd tg.Cmd) {
	inputExpectations.Store(inputExpectationKey(userID), cmd)
}

func (db *DB) DelInputExpectation(userID int64) {
	inputExpectations.Delete(inputExpectationKey(userID))
}

func (db *DB) FilenameByMsgID(userID int64, msgID int) (string, bool) {
	filename, ok := filenameByMsgID.Load(filenameByMsgIDKey(userID, msgID))
	if !ok {
		return "", false
	}

	return filename.(string), true
}

func (db *DB) DirByMsgID(userID int64, msgID int) (string, bool) {
	filename, ok := dirByMsgID.Load(dirByMsgIDKey(userID, msgID))
	if !ok {
		return "", false
	}

	return filename.(string), true
}

func (db *DB) SetFilenameByMsgID(userID int64, msgID int, filename string) {
	filenameByMsgID.Store(filenameByMsgIDKey(userID, msgID), filename)
}

func (db *DB) SetDirByMsgID(userID int64, msgID int, filename string) {
	dirByMsgID.Store(dirByMsgIDKey(userID, msgID), filename)
}

func (db *DB) RecentCommand(userID int64) (string, bool) {
	cmd, ok := recentCommands.Load(userID)
	if !ok {
		return "", false
	}

	return cmd.(string), true
}

func (db *DB) SetRecentCommand(userID int64, cmd string) {
	recentCommands.Store(userID, cmd)
}

func (db *DB) RecentCommandParams(userID int64) ([]string, bool) {
	params, ok := recentCommandsTargets.Load(userID)
	if !ok {
		return nil, false
	}

	return params.([]string), true
}

func (db *DB) SetRecentCommandParams(userID int64, params []string) {
	recentCommandsTargets.Store(userID, params)
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
