package service

import (
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
		s.store.Set(cmd.Args[0], cmd.Args[1])
		return dto.SimpleString("OK")
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
}
