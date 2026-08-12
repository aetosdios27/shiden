package command

import (
	"reflect"
	"testing"

	"github.com/aetosdios27/shiden/internal/resp"
	"github.com/aetosdios27/shiden/internal/store"
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
			database := store.New()
			if got := Execute(test.cmd, database); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Execute() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSetGetAndOverwrite(t *testing.T) {
	t.Parallel()

	database := store.New()
	binaryValue := string([]byte{0x00, 0xff, '\r', '\n'})

	if got := Execute(Command{Name: "SET", Args: []string{"key", "old"}}, database); !reflect.DeepEqual(got, resp.SimpleString("OK")) {
		t.Fatalf("first SET response = %#v, want OK", got)
	}
	if got := Execute(Command{Name: "SET", Args: []string{"key", binaryValue}}, database); !reflect.DeepEqual(got, resp.SimpleString("OK")) {
		t.Fatalf("second SET response = %#v, want OK", got)
	}
	if got := Execute(Command{Name: "GET", Args: []string{"key"}}, database); !reflect.DeepEqual(got, resp.BulkString(binaryValue)) {
		t.Fatalf("GET response = %#v, want binary bulk string", got)
	}
}

func TestGetDistinguishesEmptyAndMissing(t *testing.T) {
	t.Parallel()

	database := store.New()
	Execute(Command{Name: "SET", Args: []string{"empty", ""}}, database)

	if got := Execute(Command{Name: "GET", Args: []string{"empty"}}, database); !reflect.DeepEqual(got, resp.BulkString("")) {
		t.Fatalf("GET empty response = %#v, want empty bulk string", got)
	}
	if got := Execute(Command{Name: "GET", Args: []string{"missing"}}, database); !reflect.DeepEqual(got, resp.NullBulkString()) {
		t.Fatalf("GET missing response = %#v, want null bulk string", got)
	}
}

func TestDeleteCommand(t *testing.T) {
	t.Parallel()

	database := store.New()
	database.Set("a", []byte("1"))
	database.Set("b", []byte("2"))

	command := Command{Name: "DEL", Args: []string{"a", "b", "missing", "a"}}
	if got := Execute(command, database); !reflect.DeepEqual(got, resp.Integer(2)) {
		t.Fatalf("DEL response = %#v, want integer 2", got)
	}
}

func TestExpireCommand(t *testing.T) {
	t.Parallel()

	t.Run("existing key", func(t *testing.T) {
		t.Parallel()

		database := store.New()
		database.Set("session", []byte("alive"))
		if got := Execute(Command{Name: "EXPIRE", Args: []string{"session", "10"}}, database); !reflect.DeepEqual(got, resp.Integer(1)) {
			t.Fatalf("EXPIRE response = %#v, want integer 1", got)
		}
		if got := Execute(Command{Name: "GET", Args: []string{"session"}}, database); !reflect.DeepEqual(got, resp.BulkString("alive")) {
			t.Fatalf("GET response before expiry = %#v, want bulk string", got)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		t.Parallel()

		database := store.New()
		if got := Execute(Command{Name: "EXPIRE", Args: []string{"missing", "10"}}, database); !reflect.DeepEqual(got, resp.Integer(0)) {
			t.Fatalf("EXPIRE response = %#v, want integer 0", got)
		}
	})

	for _, seconds := range []string{"0", "-1"} {
		t.Run("immediate deletion "+seconds, func(t *testing.T) {
			t.Parallel()

			database := store.New()
			database.Set("session", []byte("alive"))
			if got := Execute(Command{Name: "EXPIRE", Args: []string{"session", seconds}}, database); !reflect.DeepEqual(got, resp.Integer(1)) {
				t.Fatalf("EXPIRE response = %#v, want integer 1", got)
			}
			if got := Execute(Command{Name: "GET", Args: []string{"session"}}, database); !reflect.DeepEqual(got, resp.NullBulkString()) {
				t.Fatalf("GET response after immediate expiry = %#v, want null bulk string", got)
			}
		})
	}

	for _, seconds := range []string{"nope", "9223372037", "9223372036854775808"} {
		t.Run("invalid seconds "+seconds, func(t *testing.T) {
			t.Parallel()

			database := store.New()
			database.Set("session", []byte("alive"))
			want := resp.Error("ERR value is not an integer or out of range")
			if got := Execute(Command{Name: "EXPIRE", Args: []string{"session", seconds}}, database); !reflect.DeepEqual(got, want) {
				t.Fatalf("EXPIRE response = %#v, want %#v", got, want)
			}
			if got := Execute(Command{Name: "GET", Args: []string{"session"}}, database); !reflect.DeepEqual(got, resp.BulkString("alive")) {
				t.Fatalf("GET after invalid EXPIRE = %#v, want unchanged value", got)
			}
		})
	}
}

func TestWriteCommandArgumentErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  Command
		want resp.Value
	}{
		{
			name: "SET without arguments",
			cmd:  Command{Name: "SET"},
			want: resp.Error("ERR wrong number of arguments for 'set' command"),
		},
		{
			name: "SET without value",
			cmd:  Command{Name: "SET", Args: []string{"key"}},
			want: resp.Error("ERR wrong number of arguments for 'set' command"),
		},
		{
			name: "SET with excess arguments",
			cmd:  Command{Name: "SET", Args: []string{"key", "value", "extra"}},
			want: resp.Error("ERR wrong number of arguments for 'set' command"),
		},
		{
			name: "GET without arguments",
			cmd:  Command{Name: "GET"},
			want: resp.Error("ERR wrong number of arguments for 'get' command"),
		},
		{
			name: "GET with excess arguments",
			cmd:  Command{Name: "GET", Args: []string{"key", "extra"}},
			want: resp.Error("ERR wrong number of arguments for 'get' command"),
		},
		{
			name: "DEL without arguments",
			cmd:  Command{Name: "DEL"},
			want: resp.Error("ERR wrong number of arguments for 'del' command"),
		},
		{
			name: "EXPIRE without arguments",
			cmd:  Command{Name: "EXPIRE"},
			want: resp.Error("ERR wrong number of arguments for 'expire' command"),
		},
		{
			name: "EXPIRE without seconds",
			cmd:  Command{Name: "EXPIRE", Args: []string{"key"}},
			want: resp.Error("ERR wrong number of arguments for 'expire' command"),
		},
		{
			name: "EXPIRE with excess arguments",
			cmd:  Command{Name: "EXPIRE", Args: []string{"key", "1", "extra"}},
			want: resp.Error("ERR wrong number of arguments for 'expire' command"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			database := store.New()
			if got := Execute(test.cmd, database); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Execute() = %#v, want %#v", got, test.want)
			}
		})
	}
}
