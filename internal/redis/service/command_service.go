package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/domain"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/dto"
)

// Handler is the port the TCP adapter calls for each parsed command.
type Handler interface {
	Handle(cmd domain.Command) string
}

// CommandService implements Handler with PING and ECHO logic.
type CommandService struct {
	store domain.Store
}

// NewCommandService constructs the service with its storage dependency.
func NewCommandService(store domain.Store) *CommandService {
	return &CommandService{store: store}
}

// Handle dispatches a command and returns a RESP-encoded response string.
func (s *CommandService) Handle(cmd domain.Command) string {
	switch strings.ToUpper(cmd.Name) {
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
			switch cmd.Args[2] {
			case "EX":
				if ttl, err := parseTTL(cmd.Args[3]); err == nil {
					s.store.SetWithTTLEx(cmd.Args[0], cmd.Args[1], ttl)
					return dto.SimpleString("OK")
				} else {
					return dto.Error("invalid expiration time")
				}
			case "PX":
				if ttl, err := parseTTL(cmd.Args[3]); err == nil {
					s.store.SetWithTTLPx(cmd.Args[0], cmd.Args[1], ttl)
					return dto.SimpleString("OK")
				} else {
					return dto.Error("invalid expiration time")
				}
			default:
				return dto.Error("unknown option for 'set' command")
			}
		}
		// Wrong number of arguments for SET with options
		if len(cmd.Args) > 2 {
			return dto.Error("wrong number of arguments for 'set' command with options")
		}
	case "GET":
		if len(cmd.Args) < 1 {
			return dto.Error("wrong number of arguments for 'get' command")
		}
		val, ok := s.store.Get(cmd.Args[0])
		if !ok {
			return dto.NullBulkString()
		}
		return dto.BulkString(val)
	default:
		return dto.Error("unknown command")
	}
	return dto.Error("unhandled command")
}

// parseTTL converts a string to an integer TTL value, validating it's non-negative.
func parseTTL(str string) (int, error) {
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	if val < 0 {
		return 0, fmt.Errorf("invalid expiration time")
	}
	return val, nil
}
