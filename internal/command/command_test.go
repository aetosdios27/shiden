package command

import (
	"reflect"
	"testing"

	"github.com/aetosdios27/shiden/internal/resp"
)

func TestParse(t *testing.T) {
	t.Parallel()

	frame := resp.Array(resp.BulkString("eChO"), resp.BulkString("hello"))
	got, err := Parse(frame)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := Command{Name: "ECHO", Args: []string{"hello"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseRejectsInvalidFrames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		frame resp.Value
	}{
		{name: "non-array", frame: resp.BulkString("PING")},
		{name: "null array", frame: resp.NullArray()},
		{name: "empty array", frame: resp.Array()},
		{name: "non-bulk command", frame: resp.Array(resp.SimpleString("PING"))},
		{name: "null bulk command", frame: resp.Array(resp.NullBulkString())},
		{name: "empty command name", frame: resp.Array(resp.BulkString(""))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(test.frame); err == nil {
				t.Fatal("Parse() error = nil, want invalid command error")
			}
		})
	}
}

func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  Command
		want resp.Value
	}{
		{name: "PING", cmd: Command{Name: "PING"}, want: resp.SimpleString("PONG")},
		{
			name: "PING message",
			cmd:  Command{Name: "PING", Args: []string{"hello"}},
			want: resp.BulkString("hello"),
		},
		{
			name: "PING wrong argument count",
			cmd:  Command{Name: "PING", Args: []string{"one", "two"}},
			want: resp.Error("ERR wrong number of arguments for 'ping' command"),
		},
		{
			name: "ECHO",
			cmd:  Command{Name: "ECHO", Args: []string{"hello"}},
			want: resp.BulkString("hello"),
		},
		{
			name: "ECHO wrong argument count",
			cmd:  Command{Name: "ECHO"},
			want: resp.Error("ERR wrong number of arguments for 'echo' command"),
		},
		{
			name: "unknown command",
			cmd:  Command{Name: "NOPE"},
			want: resp.Error("ERR unknown command 'nope'"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Execute(test.cmd); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Execute() = %#v, want %#v", got, test.want)
			}
		})
	}
}
