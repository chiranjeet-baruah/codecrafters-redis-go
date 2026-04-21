package driver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/domain"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/service"
)

var (
	// readerPool and writerPool reuse the 4 KiB internal buffers across connections
	// instead of allocating fresh ones for every connect/disconnect cycle.
	readerPool = sync.Pool{New: func() any { return bufio.NewReaderSize(nil, 4096) }}
	writerPool = sync.Pool{New: func() any { return bufio.NewWriterSize(nil, 4096) }}

	// argBufPool holds the scratch buffer used to read bulk-string bodies off the wire.
	// One buffer per goroutine means zero per-argument heap allocations for the read path.
	argBufPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, 256)
			return &buf
		},
	}
)

// handleConn runs the read → dispatch → write loop for a single client connection.
// It returns (closing the connection) on any I/O error or EOF.
func handleConn(conn net.Conn, handler service.Handler) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("error closing connection:", err)
		}
	}(conn)

	r := readerPool.Get().(*bufio.Reader)
	r.Reset(conn) // discard any stale data from a previous connection and bind to conn
	defer readerPool.Put(r)

	w := writerPool.Get().(*bufio.Writer)
	w.Reset(conn)
	defer writerPool.Put(w)

	for {
		cmd, err := readCommand(r)
		if err != nil {
			return
		}
		if _, err := io.WriteString(w, handler.Handle(cmd)); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
}

// readCommand reads one complete RESP array command from r and returns it.
// Returns an error on malformed input or any underlying I/O failure.
// The returned Command.Name is already uppercased.
func readCommand(r *bufio.Reader) (domain.Command, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return domain.Command{}, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return domain.Command{}, fmt.Errorf("expected RESP array, got %q", line)
	}
	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return domain.Command{}, err
	}

	bp := argBufPool.Get().(*[]byte)
	defer argBufPool.Put(bp) // return to pool; safe because parts holds string copies, not refs to bp

	parts := make([]string, count)
	for i := range parts {
		line, err = r.ReadString('\n')
		if err != nil {
			return domain.Command{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) == 0 || line[0] != '$' {
			return domain.Command{}, fmt.Errorf("expected bulk string header, got %q", line)
		}
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return domain.Command{}, err
		}
		if cap(*bp) < n+2 {
			*bp = make([]byte, n+2) // grow only when the argument is larger than the buffer
		}
		buf := (*bp)[:n+2]
		if _, err = io.ReadFull(r, buf); err != nil {
			return domain.Command{}, err
		}
		parts[i] = string(buf[:n]) // copy out before returning the buffer to the pool
	}
	// Uppercase once here so every handler receives a consistent, case-insensitive name.
	return domain.Command{Name: strings.ToUpper(parts[0]), Args: parts[1:]}, nil
}
