package driven

import (
	"sync"
	"time"
)

// MemoryStore is an in-memory key-value store safe for concurrent use.
// It supports string values, list values, and per-key TTL expiry.
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string]string
	pushData map[string][]string
	// timers keep a reference to each key's expiry timer so it can be
	// canceled if the key is overwritten or deleted before it fires.
	timers map[string]*time.Timer
}

// NewMemoryStore returns an empty, ready-to-use store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:     make(map[string]string),
		pushData: make(map[string][]string),
		timers:   make(map[string]*time.Timer),
	}
}

// Set stores value under a key and clears any expiry previously set on that key.
// Redis behavior: a plain SET always removes an existing TTL.
func (m *MemoryStore) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[key]; ok {
		t.Stop() // cancel the old expiry so it won't delete the new value
		delete(m.timers, key)
	}
	m.data[key] = value
}

// Get returns the value stored under the key and whether it exists.
func (m *MemoryStore) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok
}

// Delete removes the key from the store and cancels any pending expiry timer for it.
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.timers[key]; ok {
		t.Stop()
		delete(m.timers, key)
	}
	delete(m.data, key)
}

// setWithTTLInternal stores key→value and schedules its automatic deletion after d.
// If the key already has a timer, it is canceled first so the old expiry cannot
// fire and delete the freshly written value.
// Must be called without m.mu held.
func (m *MemoryStore) setWithTTLInternal(key, value string, d time.Duration) {
	m.mu.Lock()
	if t, ok := m.timers[key]; ok {
		t.Stop()
	}
	m.data[key] = value
	m.timers[key] = time.AfterFunc(d, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.data, key)
		delete(m.timers, key)
	})
	m.mu.Unlock()
}

// SetWithTTLEx stores value under a key and deletes it after ttlSeconds seconds.
// A non-positive value is ignored.
func (m *MemoryStore) SetWithTTLEx(key, value string, ttlSeconds int) {
	if ttlSeconds < 0 {
		return
	}
	m.setWithTTLInternal(key, value, time.Duration(ttlSeconds)*time.Second)
}

// SetWithTTLPx stores the value under a key and deletes it after ttlMilliseconds milliseconds.
// A non-positive value is ignored.
func (m *MemoryStore) SetWithTTLPx(key, value string, ttlMilliseconds int) {
	if ttlMilliseconds < 0 {
		return
	}
	m.setWithTTLInternal(key, value, time.Duration(ttlMilliseconds)*time.Millisecond)
}

// RPush appends value to the tail of the list at a key and returns the new list length.
// An empty value is silently ignored; the current list length is still returned.
func (m *MemoryStore) RPush(key string, value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value) > 0 {
		m.pushData[key] = append(m.pushData[key], value)
	}
	return len(m.pushData[key])
}

// RPushMultiple appends all non-empty values to the tail of the list at a key
// and returns the new list length.
func (m *MemoryStore) RPushMultiple(key string, values []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range values {
		if len(v) > 0 {
			m.pushData[key] = append(m.pushData[key], v)
		}
	}
	return len(m.pushData[key])
}

// LRange returns the elements of the list at a key between start and stop (both inclusive).
// Negative indices count from the tail: -1 is the last element, -2 the second to last, etc.
// Returns nil if the key does not exist or the range is empty.
//
// The returned slice is a copy, so it is safe to use even if the list is modified concurrently.
func (m *MemoryStore) LRange(key string, start, stop int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values, ok := m.pushData[key]
	if !ok {
		return nil
	}
	n := len(values)
	// Resolve negative indices: -1 → last element, -2 → second to last, etc.
	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	// Clamp to the valid index range.
	if start < 0 {
		start = 0
	}
	if start >= n {
		return nil
	}
	if stop >= n {
		stop = n - 1 // stop is inclusive; n-1 is the last valid index
	}
	if start > stop {
		return nil
	}
	// Copy the slice so concurrent appends to the list don't affect what the caller sees.
	result := make([]string, stop-start+1)
	copy(result, values[start:stop+1])
	return result
}

// LPush prepends value to the head of the list at a key and returns the new list length.
// An empty value is silently ignored; the current list length is still returned.
func (m *MemoryStore) LPush(key string, value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value) > 0 {
		existing := m.pushData[key]
		// Capacity covers both the new element and the existing tail, so append won't reallocate.
		newList := make([]string, 1, 1+len(existing))
		newList[0] = value
		m.pushData[key] = append(newList, existing...)
	}
	return len(m.pushData[key])
}

// LPushMultiple prepends all non-empty values to the head of the list at a key
// and returns the new list length.
func (m *MemoryStore) LPushMultiple(key string, values []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.pushData[key]
	// LPUSH semantics: each element is pushed to the head individually, so the
	// last-specified element ends up at index 0. Build the prefix in reverse
	// to reproduce that ordering in a single pass, then append the existing tail.
	prefix := make([]string, 0, len(values))
	for i := len(values) - 1; i >= 0; i-- {
		if len(values[i]) > 0 {
			prefix = append(prefix, values[i])
		}
	}
	m.pushData[key] = append(prefix, existing...)
	return len(m.pushData[key])
}

// LLen returns the length of the list at a key, or 0 if the key does not exist.
func (m *MemoryStore) LLen(key string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pushData[key])
}
