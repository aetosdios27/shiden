package resp

import (
	"bufio"
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestDecoderValidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Value
	}{
		{name: "simple string", input: "+OK\r\n", want: SimpleString("OK")},
		{name: "error", input: "-ERR failure\r\n", want: Error("ERR failure")},
		{name: "integer", input: ":-42\r\n", want: Integer(-42)},
		{name: "bulk string", input: "$5\r\nhello\r\n", want: BulkString("hello")},
		{name: "empty bulk string", input: "$0\r\n\r\n", want: BulkString("")},
		{name: "null bulk string", input: "$-1\r\n", want: NullBulkString()},
		{name: "array", input: "*2\r\n+OK\r\n:1\r\n", want: Array(SimpleString("OK"), Integer(1))},
		{name: "empty array", input: "*0\r\n", want: Array()},
		{name: "null array", input: "*-1\r\n", want: NullArray()},
		{
			name:  "nested array",
			input: "*2\r\n*2\r\n:1\r\n:2\r\n$3\r\nend\r\n",
			want:  Array(Array(Integer(1), Integer(2)), BulkString("end")),
		},
		{
			name:  "Redis command array",
			input: "*2\r\n$4\r\nECHO\r\n$5\r\nhello\r\n",
			want:  Array(BulkString("ECHO"), BulkString("hello")),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			decoder := NewDecoder(bufio.NewReader(bytes.NewBufferString(test.input)))
			got, err := decoder.Decode()
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Decode() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecoderFragmentedFrame(t *testing.T) {
	t.Parallel()

	input := []byte("*2\r\n$4\r\nECHO\r\n$5\r\nhello\r\n")
	reader := &fragmentReader{data: input, fragmentSize: 1}
	decoder := NewDecoder(bufio.NewReader(reader))

	got, err := decoder.Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	want := Array(BulkString("ECHO"), BulkString("hello"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Decode() = %#v, want %#v", got, want)
	}
}

func TestDecoderPipelinedFrames(t *testing.T) {
	t.Parallel()

	input := "*1\r\n$4\r\nPING\r\n*2\r\n$4\r\nECHO\r\n$5\r\nhello\r\n"
	decoder := NewDecoder(bufio.NewReader(bytes.NewBufferString(input)))

	first, err := decoder.Decode()
	if err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	if want := Array(BulkString("PING")); !reflect.DeepEqual(first, want) {
		t.Fatalf("first Decode() = %#v, want %#v", first, want)
	}

	second, err := decoder.Decode()
	if err != nil {
		t.Fatalf("second Decode() error = %v", err)
	}
	if want := Array(BulkString("ECHO"), BulkString("hello")); !reflect.DeepEqual(second, want) {
		t.Fatalf("second Decode() = %#v, want %#v", second, want)
	}

	if _, err := decoder.Decode(); err != io.EOF {
		t.Fatalf("third Decode() error = %v, want io.EOF", err)
	}
}

func TestDecoderMalformedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "invalid prefix", input: "?wat\r\n"},
		{name: "bad integer", input: ":not-a-number\r\n"},
		{name: "non-numeric bulk length", input: "$x\r\n"},
		{name: "negative bulk length", input: "$-2\r\n"},
		{name: "negative array length", input: "*-2\r\n"},
		{name: "incorrect line CRLF", input: "+OK\n"},
		{name: "incorrect payload CRLF", input: "$3\r\nfoo\n"},
		{name: "premature line EOF", input: "+OK"},
		{name: "premature payload EOF", input: "$5\r\nhel"},
		{name: "premature array EOF", input: "*2\r\n:1\r\n"},
		{name: "embedded carriage return", input: "+O\rK\r\n"},
		{name: "oversized line", input: "+" + strings.Repeat("a", maxLineLength+1) + "\r\n"},
		{name: "oversized bulk string", input: "$536870913\r\n"},
		{name: "oversized array", input: "*1048577\r\n"},
		{
			name:  "excessive nesting",
			input: strings.Repeat("*1\r\n", maxNestingDepth+1) + ":1\r\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			decoder := NewDecoder(bufio.NewReader(bytes.NewBufferString(test.input)))
			if _, err := decoder.Decode(); err == nil {
				t.Fatal("Decode() error = nil, want malformed input error")
			}
		})
	}
}

func TestEncoder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value Value
		want  string
	}{
		{name: "simple string", value: SimpleString("PONG"), want: "+PONG\r\n"},
		{name: "error", value: Error("ERR failure"), want: "-ERR failure\r\n"},
		{name: "integer", value: Integer(-42), want: ":-42\r\n"},
		{name: "bulk string", value: BulkString("hello"), want: "$5\r\nhello\r\n"},
		{name: "empty bulk string", value: BulkString(""), want: "$0\r\n\r\n"},
		{name: "null bulk string", value: NullBulkString(), want: "$-1\r\n"},
		{
			name:  "array",
			value: Array(BulkString("hello"), Integer(1)),
			want:  "*2\r\n$5\r\nhello\r\n:1\r\n",
		},
		{name: "null array", value: NullArray(), want: "*-1\r\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			writer := bufio.NewWriter(&output)
			encoder := NewEncoder(writer)
			if err := encoder.Encode(test.value); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if err := writer.Flush(); err != nil {
				t.Fatalf("Flush() error = %v", err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("encoded value = %q, want %q", got, test.want)
			}
		})
	}
}

type fragmentReader struct {
	data         []byte
	fragmentSize int
}

func (r *fragmentReader) Read(target []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}

	count := min(len(r.data), r.fragmentSize, len(target))
	copy(target, r.data[:count])
	r.data = r.data[count:]
	return count, nil
}
