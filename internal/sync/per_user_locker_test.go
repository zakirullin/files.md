package sync

import (
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func BenchmarkLocker(b *testing.B) {
	const M = 4
	b.ReportAllocs()
	r := require.New(b)
	var wg sync.WaitGroup
	l := NewPerUserLocker()
	wg.Add(b.N * M)
	for i := 0; i < b.N; i++ {
		for j := 0; j < M; j++ {
			go func(i int64) { l.Lock(i); l.Unlock(i); wg.Done() }(int64(i))
		}
	}
	wg.Wait()
	r.Equal(0, l.Len())
}

func TestLocker(t *testing.T) {
	r := require.New(t)
	l := NewPerUserLocker()
	r.Equal(0, l.Len())
	l.Lock(1)
	r.False(l.TryLock(1))
	l.Unlock(1)
	r.True(l.TryLock(1))
	l.Unlock(1)
	r.Equal(0, l.Len())
}

func TestLockerQueue(t *testing.T) {
	const queueLen = 10
	const userId = 42
	r := require.New(t)
	l := NewPerUserLocker()
	for i := 0; i < queueLen; i++ {
		go func(i int64) { l.Lock(userId) }(int64(i))
	}
	for i := 0; i < queueLen; i++ {
		time.Sleep(time.Millisecond) // wait for some goroutines to grab the lock
		r.Equal(1, l.Len())
		l.Unlock(userId)
	}
	r.Zero(l.Len())

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("The code did not panic")
			}
			t.Logf("The code panicked as expected, message: %v", r)
		}()
		l.Unlock(userId)
	}()

}
