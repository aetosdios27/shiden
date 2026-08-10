package command

import (
	"fmt"
	"strings"

	"github.com/aetosdios27/shiden/internal/resp"
)

// Command is a normalized Redis command name and its arguments.
type Command struct {
	Name string
	Args []string
}

// Parse extracts a command from a non-null RESP array of non-null bulk strings.
func Parse(frame resp.Value) (Command, error) {
	if frame.Type != resp.TypeArray || frame.Null {
		return Command{}, fmt.Errorf("command must be a non-null array")
	}
	if len(frame.Elements) == 0 {
		return Command{}, fmt.Errorf("command array must not be empty")
	}

	nameElement := frame.Elements[0]
	if nameElement.Type != resp.TypeBulkString || nameElement.Null {
		return Command{}, fmt.Errorf("command element 0 must be a non-null bulk string")
	}
	if nameElement.Text == "" {
		return Command{}, fmt.Errorf("command name must not be empty")
	}

	var args []string
	if len(frame.Elements) > 1 {
		args = make([]string, len(frame.Elements)-1)
	}
	for index, element := range frame.Elements[1:] {
		if element.Type != resp.TypeBulkString || element.Null {
			return Command{}, fmt.Errorf("command element %d must be a non-null bulk string", index+1)
		}
		args[index] = element.Text
	}

	return Command{
		Name: strings.ToUpper(nameElement.Text),
		Args: args,
	}, nil
}

// Execute dispatches one parsed command and returns its RESP response.
func Execute(cmd Command) resp.Value {
	switch cmd.Name {
	case "PING":
		switch len(cmd.Args) {
		case 0:
			return resp.SimpleString("PONG")
		case 1:
			return resp.BulkString(cmd.Args[0])
		default:
			return wrongArgumentCount("ping")
		}
	case "ECHO":
		if len(cmd.Args) != 1 {
			return wrongArgumentCount("echo")
		}
		return resp.BulkString(cmd.Args[0])
	default:
		return resp.Error(fmt.Sprintf("ERR unknown command '%s'", strings.ToLower(cmd.Name)))
	}
}

func wrongArgumentCount(name string) resp.Value {
	return resp.Error(fmt.Sprintf("ERR wrong number of arguments for '%s' command", name))
}
