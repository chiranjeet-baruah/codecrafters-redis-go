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
func encodeRespArray(values []string) string {
	if values == nil {
		values = []string{} // handle nil case gracefully
	}
	// Pre-compute the exact byte count so the Builder never reallocates.
	// Each element is encoded as "$<len>\r\n<v>\r\n"; the header is "*<count>\r\n".
	var buf [32]byte
	sz := 1 + len(strconv.AppendInt(buf[:0], int64(len(values)), 10)) + 2
	for _, v := range values {
		sz += 1 + len(strconv.AppendInt(buf[:0], int64(len(v)), 10)) + 2 + len(v) + 2
	}
	var sb strings.Builder
	sb.Grow(sz)
	sb.WriteByte('*')
	sb.Write(strconv.AppendInt(buf[:0], int64(len(values)), 10))
	sb.WriteString("\r\n")
	for _, v := range values {
		// Inline RESP bulk-string encoding to avoid a per-element intermediate string.
		sb.WriteByte('$')
		sb.Write(strconv.AppendInt(buf[:0], int64(len(v)), 10))
		sb.WriteString("\r\n")
		sb.WriteString(v)
		sb.WriteString("\r\n")
	}
	return sb.String()
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
		val, ok := s.store.Get(cmd.Args[0])
		if !ok {
			return dto.NullBulkString()
		}
		return dto.BulkString(val)
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
			val := s.store.LPopMultiple(cmd.Args[0], count)
			return encodeRespArray(val)
		}
		val := s.store.LPop(cmd.Args[0])
		if val == "" {
			return dto.NullBulkString() // list was empty or key didn't exist
		}
		return dto.BulkString(val)
	case "BLPOP":
		if len(cmd.Args) < 2 {
			return dto.Error("wrong number of arguments for 'blpop' command")
		}
		timeoutSeconds, err := strconv.ParseFloat(cmd.Args[1], 64)
		if err != nil || timeoutSeconds < 0 {
			return dto.Error("timeout is not a float or out of range")
		}
		timeout := time.Duration(timeoutSeconds * float64(time.Second))
		val := s.store.BLPop(cmd.Args[0], timeout)
		if val == nil {
			return dto.NullArray() // timeout expired with no element available
		}
		return encodeRespArray(val)
	default:
		return dto.Error("unknown command")
	}
}

// parseTTL parses a TTL string and returns its integer value.
// Returns an error if the value is not a valid non-negative integer.
func parseTTL(str string) (int, error) {
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	if val < 0 {
		return 0, errInvalidTTL
	}
	return val, nil
}
