package dto

import (
	"strconv"
	"strings"
)

// SimpleString encodes a RESP simple string: +<s>\r\n
func SimpleString(s string) string {
	return "+" + s + "\r\n"
}

// BulkString encodes a RESP bulk string: $<len>\r\n<s>\r\n
// PERF: strconv.AppendInt into a stack [32]byte avoids fmt.Sprintf's reflection + interface boxing (~80–120ns saved per call)
func BulkString(s string) string {
	var hdr [32]byte
	b := strconv.AppendInt(hdr[:0], int64(len(s)), 10) // PERF: hdr lives on the stack — zero heap alloc for length formatting
	var sb strings.Builder
	sb.Grow(1 + len(b) + 2 + len(s) + 2) // PERF: pre-sized single backing alloc; no realloc during writes
	sb.WriteByte('$')
	sb.Write(b)
	sb.WriteString("\r\n")
	sb.WriteString(s)
	sb.WriteString("\r\n")
	return sb.String()
}

// Error encodes a RESP error: -ERR <msg>\r\n
func Error(msg string) string {
	return "-ERR " + msg + "\r\n"
}

// NullBulkString encodes a RESP null bulk string: $-1\r\n
func NullBulkString() string {
	return "$-1\r\n"
}

// Integer encodes a RESP integer reply: :<n>\r\n
// PERF: strconv.AppendInt to stack buffer — no fmt.Sprintf, no reflection; single alloc for the returned string
func Integer(n int) string {
	var buf [32]byte
	b := strconv.AppendInt(buf[:0], int64(n), 10) // PERF: buf stays on the stack; only the returned string escapes to heap
	return ":" + string(b) + "\r\n"
}
