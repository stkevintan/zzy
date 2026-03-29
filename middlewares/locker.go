package middlewares

import "sync"

type Locker struct {
	mu    sync.Mutex
	owner string
}

func NewLocker() *Locker {
	return &Locker{}
}

func (l *Locker) Lock(owner string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.owner = owner
}

func (l *Locker) Unlock(owner string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owner == owner {
		l.owner = ""
	}
}

func (l *Locker) IsLockedByOther(owner string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.owner != "" && l.owner != owner
}
