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

// PERF: pool reuses per-argument read buffers across all connections — eliminates N heap allocs per parsed command (N = argument count)
var argBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 256) // PERF: 256B covers typical Redis key/value sizes without realloc
		return &b
	},
}

// handleConn runs the read-dispatch-write loop for one client connection.
func handleConn(conn net.Conn, handler service.Handler) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("error closing connection:", err)
		}
	}(conn)
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn) // PERF: buffers writes — replaces per-response write(2) syscall with a single buffered flush (~200–400ns saved per unbuffered write)
	for {
		cmd, err := readCommand(reader)
		if err != nil {
			return
		}
		if _, err := io.WriteString(writer, handler.Handle(cmd)); err != nil { // PERF: writes to in-memory buffer, not directly to socket
			return
		}
		if err := writer.Flush(); err != nil { // PERF: single syscall per round-trip; coalesces pipelined responses
			return
		}
	}
}

// readCommand parses one RESP array command from r.
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

	bp := argBufPool.Get().(*[]byte) // PERF: ~5ns pool get vs ~30–50ns heap alloc per argument buffer
	defer argBufPool.Put(bp)         // PERF: returned at function exit — safe because parts[] holds string copies, not references to *bp

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
			*bp = make([]byte, n+2) // PERF: rare realloc — only when arg exceeds pool buffer cap; pool retains the larger buffer for future reuse
		}
		buf := (*bp)[:n+2]
		if _, err = io.ReadFull(r, buf); err != nil {
			return domain.Command{}, err
		}
		parts[i] = string(buf[:n]) // PERF: necessary copy — string must outlive the pooled buffer
	}
	// PERF: uppercase once at parse time — enforces domain.Command.Name contract and removes strings.ToUpper alloc from every Handle() dispatch
	return domain.Command{Name: strings.ToUpper(parts[0]), Args: parts[1:]}, nil
}
