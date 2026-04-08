package driver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/codecrafters-io/redis-starter-go/internal/redis/domain"
	"github.com/codecrafters-io/redis-starter-go/internal/redis/service"
)

// handleConn runs the read-dispatch-write loop for one client connection.
func handleConn(conn net.Conn, handler service.Handler) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("error closing connection:", err)
		}
	}(conn)
	reader := bufio.NewReader(conn)
	for {
		cmd, err := readCommand(reader)
		if err != nil {
			return
		}
		if _, err := io.WriteString(conn, handler.Handle(cmd)); err != nil {
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
		buf := make([]byte, n+2)
		if _, err = io.ReadFull(r, buf); err != nil {
			return domain.Command{}, err
		}
		parts[i] = string(buf[:n])
	}
	return domain.Command{Name: parts[0], Args: parts[1:]}, nil
}
