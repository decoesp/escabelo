package server

import (
	"bufio"
	"fmt"
	"strings"
)

// Command represents a parsed command
type Command struct {
	Type   string
	Key    string
	Value  []byte
	Prefix string
}

// CommandType constants
const (
	CmdRead   = "read"
	CmdWrite  = "write"
	CmdDelete = "delete"
	CmdStatus = "status"
	CmdKeys   = "keys"
	CmdReads  = "reads"
)

// ParseCommand parses a command from the protocol
// Format: "read <key>" | "write <key>|<value>" | "delete <key>" | "status" | "keys" | "reads <prefix>"
func ParseCommand(line string) (*Command, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty command")
	}

	parts := strings.SplitN(line, " ", 2)
	cmdType := strings.ToLower(parts[0])

	switch cmdType {
	case CmdStatus:
		return &Command{Type: CmdStatus}, nil

	case CmdKeys:
		return &Command{Type: CmdKeys}, nil

	case CmdRead:
		if len(parts) < 2 {
			return nil, fmt.Errorf("read requires a key")
		}
		key := strings.TrimSpace(parts[1])
		if !isValidKey(key) {
			return nil, fmt.Errorf("invalid key format")
		}
		return &Command{Type: CmdRead, Key: key}, nil

	case CmdWrite:
		if len(parts) < 2 {
			return nil, fmt.Errorf("write requires key and value")
		}
		// Split by pipe: "key|value"
		kvParts := strings.SplitN(parts[1], "|", 2)
		if len(kvParts) < 2 {
			return nil, fmt.Errorf("write format: write <key>|<value>")
		}
		key := strings.TrimSpace(kvParts[0])
		value := kvParts[1] // Don't trim value, preserve whitespace

		if !isValidKey(key) {
			return nil, fmt.Errorf("invalid key format")
		}
		return &Command{Type: CmdWrite, Key: key, Value: []byte(value)}, nil

	case CmdDelete:
		if len(parts) < 2 {
			return nil, fmt.Errorf("delete requires a key")
		}
		key := strings.TrimSpace(parts[1])
		if !isValidKey(key) {
			return nil, fmt.Errorf("invalid key format")
		}
		return &Command{Type: CmdDelete, Key: key}, nil

	case CmdReads:
		if len(parts) < 2 {
			return nil, fmt.Errorf("reads requires a prefix")
		}
		prefix := strings.TrimSpace(parts[1])
		if !isValidKey(prefix) {
			return nil, fmt.Errorf("invalid prefix format")
		}
		return &Command{Type: CmdReads, Prefix: prefix}, nil

	default:
		return nil, fmt.Errorf("unknown command: %s", cmdType)
	}
}

// isValidKey validates key format: ([a-z] | [A-Z] | [0-9] | "." | "-" | ":" | "_")+
func isValidKey(key string) bool {
	if len(key) == 0 {
		return false
	}
	for _, ch := range key {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '.' || ch == '-' || ch == ':' || ch == '_') {
			return false
		}
	}
	return true
}

// ParseCommands parses multiple commands separated by \r
func ParseCommands(reader *bufio.Reader) ([]*Command, error) {
	var commands []*Command

	for {
		line, err := reader.ReadString('\r')
		if err != nil {
			if len(commands) > 0 {
				return commands, nil
			}
			return nil, err
		}

		// Remove the \r separator
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			continue
		}

		cmd, err := ParseCommand(line)
		if err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}
}
