package driver

import (
	"fmt"
	"net"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/service"
)

// Server listens for TCP connections and dispatches each to a Handler in its own goroutine.
type Server struct {
	addr    string
	handler service.Handler
}

// NewServer constructs a Server that will listen on addr (e.g. "0.0.0.0:6379").
func NewServer(addr string, handler service.Handler) *Server {
	return &Server{addr: addr, handler: handler}
}

// ListenAndServe binds addr and blocks forever, accepting and handling connections.
// Each connection runs in its own goroutine. Returns a non-nil error only if the
// initial bind fails.
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
