package dto

import "fmt"

// SimpleString encodes a RESP simple string: +<s>\r\n
func SimpleString(s string) string {
	return "+" + s + "\r\n"
}

// BulkString encodes a RESP bulk string: $<len>\r\n<s>\r\n
func BulkString(s string) string {
	return fmt.Sprintf("$%d\r\n%s\r\n", len(s), s)
}

// Error encodes a RESP error: -ERR <msg>\r\n
func Error(msg string) string {
	return "-ERR " + msg + "\r\n"
}
