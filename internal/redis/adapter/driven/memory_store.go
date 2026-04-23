package driven

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/constant"
)

// streamEntry holds one entry in a Redis stream: a monotonically increasing ID
// and its field-value payload.
type streamEntry struct {
	ID     string
	Fields map[string]string
}

// MemoryStore is an in-memory key-value store safe for concurrent use.
// It supports string values, list values, and per-key TTL expiry.
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string]any
	pushData map[string][]string
	// streamData maps each stream key to its entries in insertion order.
	// Entries are appended and never reordered, so the slice is always sorted by ID.
	streamData map[string][]streamEntry
	// timers keep a reference to each key's expiry timer so it can be
	// canceled if the key is overwritten or deleted before it fires.
	timers  map[string]*time.Timer
	waiters map[string][]chan string // for BLPOP: key → list of channels waiting for an element to be available
}

// NewMemoryStore returns an empty, ready-to-use store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:       make(map[string]any),
		pushData:   make(map[string][]string),
		streamData: make(map[string][]streamEntry),
		timers:     make(map[string]*time.Timer),
		waiters:    make(map[string][]chan string),
	}
}

// Set stores value under a key and clears any expiry previously set on that key.
// Redis behavior: a plain SET always removes an existing TTL.
func (m *MemoryStore) Set(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer, ok := m.timers[key]; ok {
		timer.Stop() // cancel the old expiry so it won't delete the new value
		delete(m.timers, key)
	}
	m.data[key] = value
}

// Get returns the value stored under the key and whether it exists.
func (m *MemoryStore) Get(key string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	raw, ok := m.data[key]
	if !ok {
		return "", false
	}
	result, ok := raw.(string)
	if !ok {
		return "", false
	}
	return result, true
}

// Delete removes the key from the store and cancels any pending expiry timer for it.
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer, ok := m.timers[key]; ok {
		timer.Stop()
		delete(m.timers, key)
	}
	delete(m.data, key)
}

// setWithTTLInternal stores key→value and schedules its automatic deletion after d.
// If the key already has a timer, it is canceled first so the old expiry cannot
// fire and delete the freshly written value.
// Must be called without m.mu held.
func (m *MemoryStore) setWithTTLInternal(key, value string, dur time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if timer, ok := m.timers[key]; ok {
		timer.Stop()
	}
	m.data[key] = value
	m.timers[key] = time.AfterFunc(dur, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.data, key)
		delete(m.timers, key)
	})
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

// notifyNextWaiter delivers value to the first goroutine waiting on key via BLPOP, if any.
// Reports whether a waiter was notified. Must be called with m.mu held.
func (m *MemoryStore) notifyNextWaiter(key, value string) bool {
	waiterChans := m.waiters[key]
	if len(waiterChans) == 0 {
		return false
	}
	waiterChans[0] <- value
	m.waiters[key] = waiterChans[1:]
	return true
}

// RPush appends value to the tail of the list at a key and returns the new list length.
// An empty value is silently ignored; the current list length is still returned.
// When a BLPOP waiter receives the value, it counts toward the returned length
// because Redis counts it as transiently in the list before the pop.
func (m *MemoryStore) RPush(key string, value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value) == 0 {
		return len(m.pushData[key])
	}
	if m.notifyNextWaiter(key, value) {
		return len(m.pushData[key]) + 1
	}
	m.pushData[key] = append(m.pushData[key], value)
	return len(m.pushData[key])
}

// RPushMultiple appends all non-empty values to the tail of the list at a key
// and returns the new list length.
// Values are distributed to waiting BLPOP clients first (one per waiter, in order);
// any remaining values are appended to the list.
func (m *MemoryStore) RPushMultiple(key string, values []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	consumed := 0
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		if m.notifyNextWaiter(key, value) {
			consumed++
		} else {
			m.pushData[key] = append(m.pushData[key], value)
		}
	}
	return len(m.pushData[key]) + consumed
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
	listLen := len(values)
	// Resolve negative indices: -1 → last element, -2 → second to last, etc.
	if start < 0 {
		start = listLen + start
	}
	if stop < 0 {
		stop = listLen + stop
	}
	// Clamp to the valid index range.
	if start < 0 {
		start = 0
	}
	if start >= listLen {
		return nil
	}
	if stop >= listLen {
		stop = listLen - 1 // stop is inclusive; listLen-1 is the last valid index
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
// When a BLPOP waiter receives the value, it counts toward the returned length
// because Redis counts it as transiently in the list before the pop.
func (m *MemoryStore) LPush(key string, value string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(value) == 0 {
		return len(m.pushData[key])
	}
	if m.notifyNextWaiter(key, value) {
		return len(m.pushData[key]) + 1
	}
	existing := m.pushData[key]
	// Capacity covers both the new element and the existing tail, so append won't reallocate.
	newList := make([]string, 1, 1+len(existing))
	newList[0] = value
	m.pushData[key] = append(newList, existing...)
	return len(m.pushData[key])
}

// LPushMultiple prepends all non-empty values to the head of the list at a key
// and returns the new list length.
// Values are distributed to waiting BLPOP clients first (one per waiter, in order);
// any remaining values are prepended to the list in LPUSH order (last element ends up at index 0).
func (m *MemoryStore) LPushMultiple(key string, values []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Satisfy waiting BLPOP clients first, then collect the rest for the list prepend.
	// Pre-allocate for the common case where no waiters are present and all values go to the list.
	toAdd := make([]string, 0, len(values))
	consumed := 0
	for _, value := range values {
		if len(value) == 0 {
			continue
		}
		if m.notifyNextWaiter(key, value) {
			consumed++
		} else {
			toAdd = append(toAdd, value)
		}
	}

	if len(toAdd) > 0 {
		// LPUSH semantics: each element is pushed to the head individually, so the
		// last-specified element ends up at index 0. Build the prefix in reverse
		// to reproduce that ordering in a single pass, then append the existing tail.
		prefix := make([]string, len(toAdd))
		for i, value := range toAdd {
			prefix[len(toAdd)-1-i] = value
		}
		m.pushData[key] = append(prefix, m.pushData[key]...)
	}
	return len(m.pushData[key]) + consumed
}

// LLen returns the length of the list at a key, or 0 if the key does not exist.
func (m *MemoryStore) LLen(key string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pushData[key])
}

// LPop removes and returns the first element of the list at a key
// or an empty string if the key does not exist or the list is empty.
func (m *MemoryStore) LPop(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pushData[key]) == 0 {
		return ""
	}
	result := m.pushData[key][0]
	m.pushData[key][0] = "" // release the string so it can be GC'd before narrowing the slice
	m.pushData[key] = m.pushData[key][1:]
	return result
}

// LPopMultiple removes and returns up to count elements from the head of the list at a key.
// Returns nil if the key does not exist or the list is empty.
func (m *MemoryStore) LPopMultiple(key string, count int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pushData[key]) == 0 {
		return nil
	}
	if count < 0 || count > len(m.pushData[key]) {
		count = len(m.pushData[key])
	}
	// Copy to avoid pinning the backing array; LRange already does the same.
	result := make([]string, count)
	copy(result, m.pushData[key])
	m.pushData[key] = m.pushData[key][count:]
	return result
}

// BLPop removes and returns the first element of the list.
// If the list is empty, it blocks until an element is pushed or timeout expires.
// A timeout of 0 waits indefinitely.
func (m *MemoryStore) BLPop(key string, timeout time.Duration) []string {
	m.mu.Lock()

	if list, ok := m.pushData[key]; ok && len(list) > 0 {
		element := list[0]
		m.pushData[key] = list[1:]
		m.mu.Unlock()
		return []string{key, element}
	}

	notifyCh := make(chan string, 1)
	m.waiters[key] = append(m.waiters[key], notifyCh)
	m.mu.Unlock()

	// timeout == 0 means wait indefinitely; block directly on the channel.
	if timeout == 0 {
		return []string{key, <-notifyCh}
	}

	// NewTimer + Stop prevents the timer goroutine from lingering when an element
	// arrives before the deadline (time.After has no way to cancel early).
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case element := <-notifyCh:
		return []string{key, element}
	case <-timer.C:
		m.cleanupWaiter(key, notifyCh)
		return nil
	}
}

// cleanupWaiter removes notifyCh from the waiter list for key after a BLPop timeout.
// Deletes the map entry entirely when no waiters remain, preventing unbounded map growth.
func (m *MemoryStore) cleanupWaiter(key string, notifyCh chan string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	waiterChans := m.waiters[key]
	for i, ch := range waiterChans {
		if ch == notifyCh {
			remaining := append(waiterChans[:i], waiterChans[i+1:]...)
			if len(remaining) == 0 {
				delete(m.waiters, key)
			} else {
				m.waiters[key] = remaining
			}
			break
		}
	}
}

// Type returns the Redis type name for the value stored at key: "string", "list", "stream", or "none".
func (m *MemoryStore) Type(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.data[key]; ok {
		return constant.String
	}

	if _, ok := m.pushData[key]; ok {
		return constant.List
	}

	if _, ok := m.streamData[key]; ok {
		return constant.Stream
	}

	return constant.None
}

var (
	errXAddIDZero  = errors.New("The ID specified in XADD must be greater than 0-0")
	errXAddIDSmall = errors.New("The ID specified in XADD is equal or smaller than the target stream top item")
	errInvalidID   = errors.New("invalid stream ID")
)

// formatStreamID builds a "<ms>-<seq>" string from numeric parts.
// Stack-allocated buffer avoids a heap allocation; 48 bytes fits any ms-seq pair.
func formatStreamID(ms, seq int64) string {
	var buf [48]byte
	b := strconv.AppendInt(buf[:0], ms, 10)
	b = append(b, '-')
	b = strconv.AppendInt(b, seq, 10)
	return string(b)
}

// generatePartialID returns a full "<ms>-<seq>" stream ID when the millisecond
// part is fixed and the sequence must be auto-generated.
// seq starts at 0 for ms>0, or 1 for ms=0 (since 0-0 is invalid).
// Returns errXAddIDSmall if ms is less than the last entry's millisecond.
func generatePartialID(ms int64, entries []streamEntry) (string, error) {
	seq := int64(0)
	if ms == 0 {
		seq = 1 // 0-0 is invalid; minimum is 0-1
	}

	if len(entries) > 0 {
		lastMs, lastSeq, err := parseStreamID(entries[len(entries)-1].ID)
		if err != nil {
			return "", err
		}
		if ms < lastMs {
			return "", errXAddIDSmall
		}
		if ms == lastMs {
			seq = lastSeq + 1
		}
	}

	return formatStreamID(ms, seq), nil
}

// XAdd appends an entry to the stream at streamKey and returns the entry ID.
// Pass "*" as entryID to have the server auto-generate a "<ms>-<seq>" ID.
// Pass "<ms>-*" to fix the millisecond part and auto-generate only the sequence.
// Returns an error if entryID is not strictly greater than the stream's current last ID.
func (m *MemoryStore) XAdd(streamKey, entryID string, fields []string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries := m.streamData[streamKey]

	if entryID == "*" {
		entryID = generateStreamID(entries)
	} else {
		msStr, seqStr, ok := strings.Cut(entryID, "-")
		if !ok {
			return "", errInvalidID
		}

		ms, err := strconv.ParseInt(msStr, 10, 64)
		if err != nil {
			return "", errInvalidID
		}

		if seqStr == "*" {
			// Partially auto-generated: ms is fixed, sequence is derived from existing entries.
			entryID, err = generatePartialID(ms, entries)
			if err != nil {
				return "", err
			}
		} else {
			seq, err := strconv.ParseInt(seqStr, 10, 64)
			if err != nil {
				return "", errInvalidID
			}
			if ms == 0 && seq == 0 {
				return "", errXAddIDZero
			}
			if len(entries) > 0 {
				lastID := entries[len(entries)-1].ID
				if !streamIDGreaterThan(entryID, lastID) {
					return "", errXAddIDSmall
				}
			}
		}
	}

	// Build the field map once here; the caller passes a flat key-value slice so no
	// intermediate map is created at the call site.
	fieldMap := make(map[string]string, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		fieldMap[fields[i]] = fields[i+1]
	}

	m.streamData[streamKey] = append(entries, streamEntry{ID: entryID, Fields: fieldMap})

	return entryID, nil
}

// generateStreamID returns a "<ms>-<seq>" ID that is strictly greater than the last
// entry in entries. seq resets to 0 each millisecond; if the clock jumps backward
// we stay on the last observed millisecond to preserve monotonicity.
func generateStreamID(entries []streamEntry) string {
	ms := time.Now().UnixMilli()
	seq := int64(0)

	if len(entries) > 0 {
		lastMs, lastSeq, err := parseStreamID(entries[len(entries)-1].ID)
		if err == nil {
			if lastMs == ms {
				seq = lastSeq + 1
			} else if lastMs > ms {
				ms = lastMs
				seq = lastSeq + 1
			}
		}
	}

	return formatStreamID(ms, seq)
}

// parseStreamID splits a "<ms>-<seq>" stream entry ID into its components.
func parseStreamID(id string) (ms, seq int64, err error) {
	msStr, seqStr, ok := strings.Cut(id, "-")
	if !ok {
		return 0, 0, errInvalidID
	}
	ms, err = strconv.ParseInt(msStr, 10, 64)
	if err != nil {
		return
	}
	seq, err = strconv.ParseInt(seqStr, 10, 64)
	return
}

// streamIDGreaterThan reports whether stream entry ID a is strictly greater than b.
// IDs are compared numerically: first by millisecond timestamp, then by sequence number.
func streamIDGreaterThan(a, b string) bool {
	aMs, aSeq, errA := parseStreamID(a)
	bMs, bSeq, errB := parseStreamID(b)
	if errA != nil || errB != nil {
		return false
	}
	if aMs != bMs {
		return aMs > bMs
	}
	return aSeq > bSeq
}
