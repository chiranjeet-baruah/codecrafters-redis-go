package driven

import (
	"sync"
	"time"
)

// MemoryStore is a goroutine-safe in-memory key-value store.
// It implements domain.Store.
type MemoryStore struct {
	mu        sync.RWMutex
	data      map[string]string
	rPushData map[string][]string
}

// NewMemoryStore allocates an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:      make(map[string]string),
		rPushData: make(map[string][]string), // PERF: eager init — eliminates nil guard branch inside the locked RPush critical section
	}
}

// Set stores a value under the key.
func (m *MemoryStore) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
}

// Get retrieves a value by key, returning false if absent.
func (m *MemoryStore) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok
}

// Delete removes a key from the store.
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

// setWithTTLInternal is a helper that sets a key with a TTL and schedules deletion.
func (m *MemoryStore) setWithTTLInternal(key, value string, d time.Duration) {
	m.Set(key, value)
	// PERF: time.AfterFunc uses the runtime's timer heap — eliminates per-key goroutine stack (2–8KB each) and scheduler overhead at scale
	time.AfterFunc(d, func() { m.Delete(key) })
}

// SetWithTTLEx sets a value under the key with an expiration time in seconds.
func (m *MemoryStore) SetWithTTLEx(key, value string, ttlSeconds int) {
	if ttlSeconds < 0 {
		return // or could return an error, but Redis ignores negative TTL
	}
	m.setWithTTLInternal(key, value, time.Duration(ttlSeconds)*time.Second)
}

// SetWithTTLPx sets a value under the key with an expiration time in milliseconds.
func (m *MemoryStore) SetWithTTLPx(key, value string, ttlMilliseconds int) {
	if ttlMilliseconds < 0 {
		return // or could return an error, but Redis ignores negative TTL
	}
	m.setWithTTLInternal(key, value, time.Duration(ttlMilliseconds)*time.Millisecond)
}

func (m *MemoryStore) RPush(key string, value string) int {
	if len(value) == 0 {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// PERF: nil check removed — rPushData is always initialized in NewMemoryStore
	m.rPushData[key] = append(m.rPushData[key], value)
	return len(m.rPushData[key])
}
