package sync

import (
	"fmt"
	"sync"
)

type PerUserLocker interface {
	Lock(userID int64)
	Unlock(userID int64)
	TryLock(userID int64) bool
	Len() int
}

type user struct {
	mutex       *sync.Mutex
	lastRequest interface{}
	requestsInQ int
}

func NewPerUserLocker() PerUserLocker {
	return &locker{
		users: make(map[int64]*user),
	}
}

type locker struct {
	users   map[int64]*user
	mapLock sync.Mutex
}

func (l *locker) TryLock(userID int64) bool {
	l.mapLock.Lock()
	defer l.mapLock.Unlock()
	u := l.getUser(userID)
	if u.mutex.TryLock() {
		u.requestsInQ++
		return true
	}
	return false
}

func (l *locker) Lock(userID int64) {
	l.mapLock.Lock()
	u := l.getUser(userID)
	u.requestsInQ++
	l.mapLock.Unlock()
	u.mutex.Lock()
}

func (l *locker) Unlock(userID int64) {
	l.mapLock.Lock()
	defer l.mapLock.Unlock()
	user, ok := l.users[userID]
	if !ok {
		panic(fmt.Sprintf("Unlocking non-existing mutex, userID=%v", userID))
	}
	user.mutex.Unlock()
	user.requestsInQ--
	if user.requestsInQ == 0 {
		delete(l.users, userID)
	}
}

func (l *locker) Len() int {
	l.mapLock.Lock()
	defer l.mapLock.Unlock()
	return len(l.users)
}

// getUser returns user by userID, if user does not exist, it creates new one
// the function is not thread safe, it should be called from thread safe function
func (l *locker) getUser(userID int64) *user {
	u, ok := l.users[userID]
	if !ok {
		u = &user{
			mutex: &sync.Mutex{},
		}
		l.users[userID] = u
	}
	return u
}
