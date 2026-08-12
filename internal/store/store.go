package store

import (
	"bytes"
	"sync"
	"time"
)

// Store owns Shiden's process-wide string keyspace. A single mutex protects the
// map; this is intentionally provisional until Shiden earns a concurrency
// architecture beyond one goroutine per connection.
type Store struct {
	mutex  sync.RWMutex
	values map[string]entry
	now    func() time.Time
}

type entry struct {
	value     []byte
	expiresAt time.Time
}

// New constructs an empty Store.
func New() *Store {
	return newWithClock(time.Now)
}

func newWithClock(now func() time.Time) *Store {
	return &Store{
		values: make(map[string]entry),
		now:    now,
	}
}

// Set stores a copy of value under key and clears any existing expiration.
// The caller retains ownership of value.
func (s *Store) Set(key string, value []byte) {
	ownedValue := bytes.Clone(value)

	s.mutex.Lock()
	s.values[key] = entry{value: ownedValue}
	s.mutex.Unlock()
}

// Get returns a caller-owned copy of the live value under key. The boolean
// reports whether the key exists, distinguishing a missing or expired key from
// an empty value.
func (s *Store) Get(key string) ([]byte, bool) {
	now := s.now()

	s.mutex.RLock()
	stored, exists := s.values[key]
	if !exists {
		s.mutex.RUnlock()
		return nil, false
	}
	if !stored.expiresAt.IsZero() && !stored.expiresAt.After(now) {
		s.mutex.RUnlock()
		s.deleteIfExpired(key)
		return nil, false
	}
	ownedValue := bytes.Clone(stored.value)
	s.mutex.RUnlock()
	return ownedValue, true
}

// Expire sets key's lifetime. A non-positive lifetime deletes a live key
// immediately. The result reports whether the key existed and was live.
func (s *Store) Expire(key string, lifetime time.Duration) bool {
	now := s.now()

	s.mutex.Lock()
	defer s.mutex.Unlock()

	stored, exists := s.values[key]
	if !exists {
		return false
	}
	if !stored.expiresAt.IsZero() && !stored.expiresAt.After(now) {
		delete(s.values, key)
		return false
	}
	if lifetime <= 0 {
		delete(s.values, key)
		return true
	}

	stored.expiresAt = now.Add(lifetime)
	s.values[key] = stored
	return true
}

// Delete removes live keys and returns the number that existed. Repeated keys
// count only once because each key is absent after its first deletion.
func (s *Store) Delete(keys ...string) int {
	now := s.now()

	s.mutex.Lock()
	deleted := 0
	for _, key := range keys {
		stored, exists := s.values[key]
		if !exists {
			continue
		}
		if !stored.expiresAt.IsZero() && !stored.expiresAt.After(now) {
			delete(s.values, key)
			continue
		}
		delete(s.values, key)
		deleted++
	}
	s.mutex.Unlock()
	return deleted
}

// DeleteExpired removes entries whose deadlines have passed and returns the
// number removed.
func (s *Store) DeleteExpired() int {
	now := s.now()

	s.mutex.Lock()
	deleted := 0
	for key, stored := range s.values {
		if stored.expiresAt.IsZero() || stored.expiresAt.After(now) {
			continue
		}
		delete(s.values, key)
		deleted++
	}
	s.mutex.Unlock()
	return deleted
}

func (s *Store) deleteIfExpired(key string) {
	now := s.now()

	s.mutex.Lock()
	stored, exists := s.values[key]
	if exists && !stored.expiresAt.IsZero() && !stored.expiresAt.After(now) {
		delete(s.values, key)
	}
	s.mutex.Unlock()
}
