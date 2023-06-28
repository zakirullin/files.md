package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"zakirullin/stuffbot/pkg/tg"

	"github.com/alicebob/miniredis/v2"
)

const (
	redisLastKeyboardMsgID               = "last_keyboard"
	redisReplaceWithDefaultKeyboardMsgID = "candidate_message_id"
	redisSchedule                        = "schedule"
	redisInputExpectation                = "input_expectation"
)

// DB Maybe user ID here?
type DB struct {
	redis *miniredis.Miniredis
}

func NewDB(redis *miniredis.Miniredis) *DB {
	return &DB{redis}
}

func (db *DB) LastKeyboardMsgID(userID int64) (*int, error) {
	val, err := db.redis.Get(db.key(userID, redisLastKeyboardMsgID))
	if errors.Is(err, miniredis.ErrKeyNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("can't get last keyboard msg ID: %w", err)
	}

	i, err := strconv.Atoi(val)

	return &i, err
}

func (db *DB) SetLastKeyboardMsgID(userID int64, ID int) error {
	return db.redis.Set(db.key(userID, redisLastKeyboardMsgID), strconv.Itoa(ID))
}

func (db *DB) DelLastKeyboardMsgID(userID int64) error {
	db.redis.Del(db.key(userID, redisLastKeyboardMsgID))
	//if err {
	//	return fmt.Errorf("db.DelLastKeyboardMsgID: %w", err)
	//}

	return nil
}

func (db *DB) InputExpectation(userID int64) (*tg.Cmd, error) {
	js, err := db.redis.Get(db.key(userID, redisInputExpectation))
	if errors.Is(err, miniredis.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db can't get input expectation: %w", err)
	}

	cmd := new(tg.Cmd)
	err = json.Unmarshal([]byte(js), &cmd)
	if err != nil {
		return nil, fmt.Errorf("db can't unmarshall input expectation: %w", err)
	}

	return cmd, nil
}

func (db *DB) SetInputExpectation(userID int64, cmd tg.Cmd) error {
	js, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("db can't set input expectation: %w", err)
	}

	err = db.redis.Set(db.key(userID, redisInputExpectation), string(js))
	if err != nil {
		return fmt.Errorf("db set input expectation can't save to redis: %w", err)
	}

	return nil
}

func (db *DB) DelInputExpectation(userID int64) error {
	key := db.key(userID, redisInputExpectation)
	ok := db.redis.Del(key)
	if !ok {
		return errors.New(fmt.Sprintf("db set input expectation: can't del key %s", key))
	}

	return nil
}

// User-namespaced redis key
func (db *DB) key(userID int64, key string) string {
	return fmt.Sprintf("%s:%d", key, userID)
}
