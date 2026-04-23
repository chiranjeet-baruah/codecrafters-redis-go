package service

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/domain"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/dto"
)

// Handler is the interface the TCP adapter calls for each parsed RESP command.
// Handle must return a complete, RESP-encoded response string.
type Handler interface {
	Handle(cmd domain.Command) string
}

// CommandService routes RESP commands to the store and encodes the responses.
// Supported commands: PING, ECHO, SET (with optional EX/PX), GET, RPUSH, LPUSH, LRANGE, LLEN, LPOP, BLPOP.
type CommandService struct {
	store domain.Store
}

// NewCommandService returns a CommandService backed by the given store.
func NewCommandService(store domain.Store) *CommandService {
	return &CommandService{store: store}
}

// errInvalidTTL is returned by parseTTL when the TTL value is negative.
var errInvalidTTL = errors.New("invalid expiration time")

// encodeRespArray encodes a slice of strings as a RESP array.
// Pre-computes the exact byte count so the Builder never reallocates.
// Each element is encoded as "$<len>\r\n<value>\r\n"; the header is "*<count>\r\n".
func encodeRespArray(values []string) string {
	if values == nil {
		values = []string{}
	}
	var lenBuf [32]byte
	totalSize := 1 + len(strconv.AppendInt(lenBuf[:0], int64(len(values)), 10)) + 2
	for _, value := range values {
		totalSize += 1 + len(strconv.AppendInt(lenBuf[:0], int64(len(value)), 10)) + 2 + len(value) + 2
	}
	var builder strings.Builder
	builder.Grow(totalSize)
	builder.WriteByte('*')
	builder.Write(strconv.AppendInt(lenBuf[:0], int64(len(values)), 10))
	builder.WriteString("\r\n")
	for _, value := range values {
		// Inline RESP bulk-string encoding to avoid a per-element intermediate string.
		builder.WriteByte('$')
		builder.Write(strconv.AppendInt(lenBuf[:0], int64(len(value)), 10))
		builder.WriteString("\r\n")
		builder.WriteString(value)
		builder.WriteString("\r\n")
	}
	return builder.String()
}

// Handle routes cmd to the right handler and returns a RESP-encoded response.
// cmd.Name is already uppercased by the RESP parser.
func (s *CommandService) Handle(cmd domain.Command) string {
	switch cmd.Name {
	case "PING":
		return dto.SimpleString("PONG")
	case "ECHO":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'echo' command")
		}
		return dto.BulkString(cmd.Args[0])
	case "SET":
		if len(cmd.Args) < 2 {
			return dto.Error("wrong number of arguments for 'set' command")
		}
		if len(cmd.Args) == 2 {
			s.store.Set(cmd.Args[0], cmd.Args[1])
			return dto.SimpleString("OK")
		}
		// Handle SET with options (EX/PX)
		if len(cmd.Args) >= 4 {
			option := cmd.Args[2]
			// EqualFold handles "ex"/"EX"/"Ex" etc. without allocating an uppercase copy.
			isEX := strings.EqualFold(option, "EX")
			if !isEX && !strings.EqualFold(option, "PX") {
				return dto.Error("unknown option for 'set' command")
			}

			ttl, err := parseTTL(cmd.Args[3])
			if err != nil {
				return dto.Error("invalid expiration time")
			}

			if isEX {
				s.store.SetWithTTLEx(cmd.Args[0], cmd.Args[1], ttl)
			} else {
				s.store.SetWithTTLPx(cmd.Args[0], cmd.Args[1], ttl)
			}
			return dto.SimpleString("OK")
		}
		return dto.Error("wrong number of arguments for 'set' command with options")
	case "GET":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'get' command")
		}
		storedValue, ok := s.store.Get(cmd.Args[0])
		if !ok {
			return dto.NullBulkString
		}
		return dto.BulkString(storedValue)
	case "RPUSH":
		if len(cmd.Args) < 2 {
			return dto.Error("wrong number of arguments for 'rpush' command")
		}
		length := s.store.RPushMultiple(cmd.Args[0], cmd.Args[1:])
		return dto.Integer(length)
	case "LPUSH":
		if len(cmd.Args) < 2 {
			return dto.Error("wrong number of arguments for 'lpush' command")
		}
		length := s.store.LPushMultiple(cmd.Args[0], cmd.Args[1:])
		return dto.Integer(length)
	case "LRANGE":
		if len(cmd.Args) < 3 {
			return dto.Error("wrong number of arguments for 'lrange' command")
		}
		start, err := strconv.Atoi(cmd.Args[1])
		if err != nil {
			return dto.Error("invalid start index")
		}
		stop, err := strconv.Atoi(cmd.Args[2])
		if err != nil {
			return dto.Error("invalid stop index")
		}
		values := s.store.LRange(cmd.Args[0], start, stop)
		return encodeRespArray(values)
	case "LLEN":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'llen' command")
		}
		length := s.store.LLen(cmd.Args[0])
		return dto.Integer(length)
	case "LPOP":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'lpop' command")
		}
		if len(cmd.Args) == 2 {
			count, err := strconv.Atoi(cmd.Args[1])
			if err != nil {
				return dto.Error("invalid count")
			}
			elements := s.store.LPopMultiple(cmd.Args[0], count)
			return encodeRespArray(elements)
		}
		element := s.store.LPop(cmd.Args[0])
		if element == "" {
			return dto.NullBulkString
		}
		return dto.BulkString(element)
	case "BLPOP":
		if len(cmd.Args) < 2 {
			return dto.Error("wrong number of arguments for 'blpop' command")
		}
		timeoutSeconds, err := strconv.ParseFloat(cmd.Args[1], 64)
		if err != nil || timeoutSeconds < 0 {
			return dto.Error("timeout is not a float or out of range")
		}
		timeout := time.Duration(timeoutSeconds * float64(time.Second))
		result := s.store.BLPop(cmd.Args[0], timeout)
		if result == nil {
			return dto.NullArray
		}
		return encodeRespArray(result)
	case "TYPE":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'type' command")
		}
		keyType := s.store.Type(cmd.Args[0])
		return dto.SimpleString(keyType)
	case "XADD":
		if len(cmd.Args) < 4 {
			return dto.Error("wrong number of arguments for 'xadd' command")
		}
		streamKey := cmd.Args[0]
		entryID := cmd.Args[1]
		fields := cmd.Args[2:]

		if len(fields)%2 != 0 {
			return dto.Error("field-value pairs must be even in number")
		}

		newEntryID, err := s.store.XAdd(streamKey, entryID, fields)
		if err != nil {
			return dto.Error(err.Error())
		}
		return dto.BulkString(newEntryID)

	default:
		return dto.Error("unknown command")
	}
}

// parseTTL parses a TTL string and returns its integer value.
// Returns an error if the value is not a valid non-negative integer.
func parseTTL(ttlStr string) (int, error) {
	ttlValue, err := strconv.Atoi(ttlStr)
	if err != nil {
		return 0, err
	}
	if ttlValue < 0 {
		return 0, errInvalidTTL
	}
	return ttlValue, nil
}
