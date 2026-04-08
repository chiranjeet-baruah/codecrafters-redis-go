package domain

// Store is the port interface for the key-value storage layer.
type Store interface {
	Set(key, value string)
	Get(key string) (string, bool)
	SetWithTTLEx(key, value string, ttlSeconds int)
	SetWithTTLPx(key, value string, ttlMilliseconds int)
	Delete(key string)
}
