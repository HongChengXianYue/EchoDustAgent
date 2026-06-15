package agent

import (
	"sort"
	"sync"
)

type targetLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newTargetLocks() *targetLocks {
	return &targetLocks{locks: map[string]*sync.Mutex{}}
}

func (l *targetLocks) lock(targets []string) func() {
	if len(targets) == 0 {
		return func() {}
	}
	targets = append([]string(nil), targets...)
	sort.Strings(targets)
	held := make([]*sync.Mutex, 0, len(targets))
	for _, target := range targets {
		lock := l.lockFor(target)
		lock.Lock()
		held = append(held, lock)
	}
	return func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Unlock()
		}
	}
}

func (l *targetLocks) lockFor(target string) *sync.Mutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	lock := l.locks[target]
	if lock == nil {
		lock = &sync.Mutex{}
		l.locks[target] = lock
	}
	return lock
}
