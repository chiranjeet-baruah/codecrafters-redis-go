package dto

import (
	"strconv"
	"strings"
)

// SimpleString encodes s as a RESP simple string: +<s>\r\n
func SimpleString(s string) string {
	return "+" + s + "\r\n"
}

// BulkString encodes s as a RESP bulk string: $<len>\r\n<s>\r\n
func BulkString(s string) string {
	var hdr [32]byte
	b := strconv.AppendInt(hdr[:0], int64(len(s)), 10)
	var sb strings.Builder
	sb.Grow(1 + len(b) + 2 + len(s) + 2) // pre-size so the builder never reallocates
	sb.WriteByte('$')
	sb.Write(b)
	sb.WriteString("\r\n")
	sb.WriteString(s)
	sb.WriteString("\r\n")
	return sb.String()
}

// Error encodes msg as a RESP error reply: -ERR <msg>\r\n
func Error(msg string) string {
	return "-ERR " + msg + "\r\n"
}

// NullBulkString returns the RESP null bulk string ($-1\r\n),
// used to indicate a missing key.
func NullBulkString() string {
	return "$-1\r\n"
}

// NullArray returns the RESP null array (*-1\r\n), used when a blocking command times out with no result.
func NullArray() string {
	return "*-1\r\n"
}

// Integer encodes n as a RESP integer reply: :<n>\r\n
func Integer(n int) string {
	var buf [32]byte
	b := strconv.AppendInt(buf[:0], int64(n), 10)
	var sb strings.Builder
	sb.Grow(1 + len(b) + 2) // pre-size so the builder never reallocates
	sb.WriteByte(':')
	sb.Write(b)
	sb.WriteString("\r\n")
	return sb.String()
}
