package runtime

import "sync"

type keyedLockEntry struct {
	mutex sync.Mutex
	refs  int
}

type keyedLocker struct {
	mutex   sync.Mutex
	entries map[string]*keyedLockEntry
}

func newKeyedLocker() *keyedLocker {
	return &keyedLocker{entries: make(map[string]*keyedLockEntry)}
}

func (l *keyedLocker) Acquire(key string) func() {
	l.mutex.Lock()
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedLockEntry{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mutex.Unlock()

	entry.mutex.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			entry.mutex.Unlock()
			l.mutex.Lock()
			entry.refs--
			if entry.refs == 0 && l.entries[key] == entry {
				delete(l.entries, key)
			}
			l.mutex.Unlock()
		})
	}
}

func (l *keyedLocker) size() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return len(l.entries)
}
