package runtime

import (
	"sync"
	"testing"
	"time"
)

func TestKeyedLockerReclaimsUniqueKeysAfterRelease(t *testing.T) {
	locker := newKeyedLocker()
	for index := 0; index < 100; index++ {
		release := locker.Acquire(string(rune(index + 1)))
		release()
	}
	if size := locker.size(); size != 0 {
		t.Fatalf("retained lock entries=%d, want 0", size)
	}
}

func TestKeyedLockerSerializesSameKey(t *testing.T) {
	locker := newKeyedLocker()
	releaseFirst := locker.Acquire("room")
	acquiredSecond := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		releaseSecond := locker.Acquire("room")
		close(acquiredSecond)
		releaseSecond()
	}()
	select {
	case <-acquiredSecond:
		t.Fatal("same key acquired concurrently")
	case <-time.After(10 * time.Millisecond):
	}
	releaseFirst()
	wait.Wait()
	if locker.size() != 0 {
		t.Fatalf("lock entry was not reclaimed")
	}
}
