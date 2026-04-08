package domain

// Command represents a single parsed RESP command.
type Command struct {
	Name string   // upper-cased command name, e.g. "PING", "ECHO"
	Args []string // positional arguments after the command name
}
