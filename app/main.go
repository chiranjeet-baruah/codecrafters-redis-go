package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
)

func main() {
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("Error closing connection: ", err.Error())
		}
	}(conn)

	reader := bufio.NewReader(conn)
	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "PING":
			_, err := io.WriteString(conn, "+PONG\r\n")
			if err != nil {
				return
			}
		}
	}
}

// readCommand reads one RESP array command and returns its arguments.
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if len(line) == 0 || line[0] != '*' {
		return nil, fmt.Errorf("expected RESP array, got %q", line)
	}
	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, err
	}

	args := make([]string, count)
	for i := range args {
		// bulk string header: $<len>\r\n
		line, err = r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) == 0 || line[0] != '$' {
			return nil, fmt.Errorf("expected bulk string header, got %q", line)
		}
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		// read exactly n bytes + \r\n
		buf := make([]byte, n+2)
		if _, err = io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args[i] = string(buf[:n])
	}
	return args, nil
}
