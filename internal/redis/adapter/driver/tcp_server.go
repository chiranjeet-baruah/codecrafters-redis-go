package driver

import (
	"fmt"
	"net"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/service"
)

// Server is a TCP server that dispatches connections to a Handler.
type Server struct {
	addr    string
	handler service.Handler
}

// NewServer constructs a Server for the given address.
func NewServer(addr string, handler service.Handler) *Server {
	return &Server{addr: addr, handler: handler}
}

// ListenAndServe binds the port and blocks, accepting connections forever.
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %w", s.addr, err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("error accepting connection:", err)
			continue
		}
		go handleConn(conn, s.handler)
	}
}
