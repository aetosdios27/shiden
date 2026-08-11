package store

import (
	"bytes"
	"sync"
)

// Store owns Shiden's process-wide string keyspace. A single mutex protects the
// map; this is intentionally provisional until Shiden earns a concurrency
// architecture beyond one goroutine per connection.
type Store struct {
	mutex  sync.RWMutex
	values map[string][]byte
}

// New constructs an empty Store.
func New() *Store {
	return &Store{values: make(map[string][]byte)}
}

// Set stores a copy of value under key. The caller retains ownership of value.
func (s *Store) Set(key string, value []byte) {
	ownedValue := bytes.Clone(value)

	s.mutex.Lock()
	s.values[key] = ownedValue
	s.mutex.Unlock()
}

// Get returns a caller-owned copy of the value under key. The boolean reports
// whether the key exists, distinguishing a missing key from an empty value.
func (s *Store) Get(key string) ([]byte, bool) {
	s.mutex.RLock()
	value, exists := s.values[key]
	if !exists {
		s.mutex.RUnlock()
		return nil, false
	}
	ownedValue := bytes.Clone(value)
	s.mutex.RUnlock()
	return ownedValue, true
}

// Delete removes keys and returns the number that existed. Repeated keys count
// only once because each key is absent after its first deletion.
func (s *Store) Delete(keys ...string) int {
	s.mutex.Lock()
	deleted := 0
	for _, key := range keys {
		if _, exists := s.values[key]; !exists {
			continue
		}
		delete(s.values, key)
		deleted++
	}
	s.mutex.Unlock()
	return deleted
}
