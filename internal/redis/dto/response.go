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
	var lenBuf [32]byte
	lenBytes := strconv.AppendInt(lenBuf[:0], int64(len(s)), 10)
	var builder strings.Builder
	builder.Grow(1 + len(lenBytes) + 2 + len(s) + 2) // pre-size so the builder never reallocates
	builder.WriteByte('$')
	builder.Write(lenBytes)
	builder.WriteString("\r\n")
	builder.WriteString(s)
	builder.WriteString("\r\n")
	return builder.String()
}

// Error encodes msg as a RESP error reply: -ERR <msg>\r\n
func Error(msg string) string {
	return "-ERR " + msg + "\r\n"
}

// NullBulkString is the RESP null bulk string, used to indicate a missing key.
const NullBulkString = "$-1\r\n"

// NullArray is the RESP null array, returned when a blocking command times out with no result.
const NullArray = "*-1\r\n"

// Integer encodes n as a RESP integer reply: :<n>\r\n
func Integer(n int) string {
	var intBuf [32]byte
	lenBytes := strconv.AppendInt(intBuf[:0], int64(n), 10)
	var builder strings.Builder
	builder.Grow(1 + len(lenBytes) + 2) // pre-size so the builder never reallocates
	builder.WriteByte(':')
	builder.Write(lenBytes)
	builder.WriteString("\r\n")
	return builder.String()
}
