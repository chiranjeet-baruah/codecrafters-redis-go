package domain

// Store is the port interface for the key-value storage layer.
// Implementations must be safe for concurrent use.
type Store interface {
	// Set stores value under a key, clearing any existing TTL on that key.
	Set(key, value string)

	// Get returns the value for the key and whether it was found.
	Get(key string) (string, bool)

	// SetWithTTLEx stores value under a key and schedules its deletion after ttlSeconds seconds.
	// A non-positive ttlSeconds is ignored.
	SetWithTTLEx(key, value string, ttlSeconds int)

	// SetWithTTLPx stores a value under a key and schedules its deletion after ttlMilliseconds milliseconds.
	// A non-positive ttlMilliseconds is ignored.
	SetWithTTLPx(key, value string, ttlMilliseconds int)

	// Delete removes the key from the store and cancels any pending TTL timer for it.
	Delete(key string)

	// RPush appends value to the tail of the list stored at a key and returns the new list length.
	// An empty value is silently ignored.
	RPush(key string, value string) int

	// RPushMultiple appends all non-empty values to the tail of the list at a key
	// and returns the new list length.
	RPushMultiple(key string, values []string) int

	// LRange returns the elements of the list at a key between start and stop (both inclusive).
	// Negative indices count from the tail: -1 is the last element, -2 the second to last, etc.
	// Returns nil if the key does not exist or the range is empty.
	LRange(key string, start, stop int) []string
}
