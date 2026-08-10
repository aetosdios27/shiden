package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Type identifies a RESP2 value type by its wire prefix.
type Type byte

const (
	// TypeSimpleString identifies a RESP2 simple string.
	TypeSimpleString Type = '+'
	// TypeError identifies a RESP2 error.
	TypeError Type = '-'
	// TypeInteger identifies a RESP2 integer.
	TypeInteger Type = ':'
	// TypeBulkString identifies a RESP2 bulk string.
	TypeBulkString Type = '$'
	// TypeArray identifies a RESP2 array.
	TypeArray Type = '*'
)

const (
	maxLineLength       = 64 * 1024
	maxBulkStringLength = 512 * 1024 * 1024
	maxArrayElements    = 1 << 20
	maxNestingDepth     = 128
)

var errMalformedCRLF = errors.New("resp: malformed CRLF")

// Value is one decoded RESP2 value. Text stores simple strings, errors, and
// bulk strings; Integer stores integers; Elements stores arrays. Null is valid
// only for bulk strings and arrays.
type Value struct {
	Type     Type
	Text     string
	Integer  int64
	Elements []Value
	Null     bool
}

// SimpleString constructs a RESP2 simple string.
func SimpleString(value string) Value {
	return Value{Type: TypeSimpleString, Text: value}
}

// Error constructs a RESP2 error.
func Error(message string) Value {
	return Value{Type: TypeError, Text: message}
}

// Integer constructs a RESP2 integer.
func Integer(value int64) Value {
	return Value{Type: TypeInteger, Integer: value}
}

// BulkString constructs a RESP2 bulk string.
func BulkString(value string) Value {
	return Value{Type: TypeBulkString, Text: value}
}

// NullBulkString constructs a null RESP2 bulk string.
func NullBulkString() Value {
	return Value{Type: TypeBulkString, Null: true}
}

// Array constructs a RESP2 array.
func Array(values ...Value) Value {
	return Value{Type: TypeArray, Elements: values}
}

// NullArray constructs a null RESP2 array.
func NullArray() Value {
	return Value{Type: TypeArray, Null: true}
}

// Decoder reads complete RESP2 values from a buffered byte stream.
type Decoder struct {
	reader *bufio.Reader
}

// NewDecoder constructs a decoder over reader. Bytes beyond one decoded frame
// remain buffered for the next call to Decode.
func NewDecoder(reader *bufio.Reader) *Decoder {
	return &Decoder{reader: reader}
}

// Decode reads exactly one RESP2 value from the stream.
func (d *Decoder) Decode() (Value, error) {
	return d.decode(0)
}

func (d *Decoder) decode(depth int) (Value, error) {
	if depth > maxNestingDepth {
		return Value{}, fmt.Errorf("resp: maximum nesting depth %d exceeded", maxNestingDepth)
	}

	prefix, err := d.reader.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch Type(prefix) {
	case TypeSimpleString:
		text, err := d.readLine()
		if err != nil {
			return Value{}, fmt.Errorf("resp: read simple string: %w", err)
		}
		return SimpleString(text), nil
	case TypeError:
		message, err := d.readLine()
		if err != nil {
			return Value{}, fmt.Errorf("resp: read error: %w", err)
		}
		return Error(message), nil
	case TypeInteger:
		text, err := d.readLine()
		if err != nil {
			return Value{}, fmt.Errorf("resp: read integer: %w", err)
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return Value{}, fmt.Errorf("resp: invalid integer %q: %w", text, err)
		}
		return Integer(value), nil
	case TypeBulkString:
		return d.decodeBulkString()
	case TypeArray:
		return d.decodeArray(depth)
	default:
		return Value{}, fmt.Errorf("resp: invalid type prefix %q", prefix)
	}
}

func (d *Decoder) decodeBulkString() (Value, error) {
	length, err := d.readLength("bulk string")
	if err != nil {
		return Value{}, err
	}
	if length == -1 {
		return NullBulkString(), nil
	}
	if length < -1 {
		return Value{}, fmt.Errorf("resp: invalid bulk string length %d", length)
	}

	if length > maxBulkStringLength {
		return Value{}, fmt.Errorf("resp: invalid bulk string length %d: maximum is %d", length, maxBulkStringLength)
	}

	payload := make([]byte, int(length)+2)
	if _, err := io.ReadFull(d.reader, payload); err != nil {
		return Value{}, fmt.Errorf("resp: read bulk string payload: %w", err)
	}
	if payload[len(payload)-2] != '\r' || payload[len(payload)-1] != '\n' {
		return Value{}, fmt.Errorf("resp: bulk string payload missing CRLF")
	}

	return BulkString(string(payload[:len(payload)-2])), nil
}

func (d *Decoder) decodeArray(depth int) (Value, error) {
	length, err := d.readLength("array")
	if err != nil {
		return Value{}, err
	}
	if length == -1 {
		return NullArray(), nil
	}
	if length < -1 {
		return Value{}, fmt.Errorf("resp: invalid array length %d", length)
	}
	if length > maxArrayElements {
		return Value{}, fmt.Errorf("resp: invalid array length %d: maximum is %d", length, maxArrayElements)
	}

	var values []Value
	if length > 0 {
		values = make([]Value, 0, int(length))
	}
	for range length {
		value, err := d.decode(depth + 1)
		if err != nil {
			return Value{}, fmt.Errorf("resp: read array element: %w", err)
		}
		values = append(values, value)
	}

	return Array(values...), nil
}

func (d *Decoder) readLength(kind string) (int64, error) {
	text, err := d.readLine()
	if err != nil {
		return 0, fmt.Errorf("resp: read %s length: %w", kind, err)
	}
	length, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("resp: invalid %s length %q: %w", kind, text, err)
	}
	return length, nil
}

func (d *Decoder) readLine() (string, error) {
	var line []byte
	for {
		fragment, err := d.reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxLineLength+2 {
			return "", fmt.Errorf("resp: line exceeds maximum length of %d bytes", maxLineLength)
		}
		line = append(line, fragment...)

		switch {
		case err == nil:
			if len(line) < 2 || line[len(line)-2] != '\r' {
				return "", errMalformedCRLF
			}
			text := line[:len(line)-2]
			if bytesContainCRLF(text) {
				return "", errMalformedCRLF
			}
			return string(text), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(line) > 0:
			return "", io.ErrUnexpectedEOF
		default:
			return "", err
		}
	}
}

func bytesContainCRLF(value []byte) bool {
	for _, character := range value {
		if character == '\r' || character == '\n' {
			return true
		}
	}
	return false
}

// Encoder writes RESP2 values to a buffered byte stream. Encode does not flush
// the writer; the connection owner controls response boundaries and flushing.
type Encoder struct {
	writer *bufio.Writer
}

// NewEncoder constructs an encoder over writer.
func NewEncoder(writer *bufio.Writer) *Encoder {
	return &Encoder{writer: writer}
}

// Encode writes one RESP2 value.
func (e *Encoder) Encode(value Value) error {
	switch value.Type {
	case TypeSimpleString, TypeError:
		if value.Null {
			return fmt.Errorf("resp: null is invalid for type %q", value.Type)
		}
		if strings.ContainsAny(value.Text, "\r\n") {
			return fmt.Errorf("resp: type %q cannot contain CR or LF", value.Type)
		}
		if err := e.writer.WriteByte(byte(value.Type)); err != nil {
			return fmt.Errorf("resp: write prefix: %w", err)
		}
		if _, err := e.writer.WriteString(value.Text); err != nil {
			return fmt.Errorf("resp: write line: %w", err)
		}
		if _, err := e.writer.WriteString("\r\n"); err != nil {
			return fmt.Errorf("resp: write line terminator: %w", err)
		}
		return nil
	case TypeInteger:
		if value.Null {
			return fmt.Errorf("resp: null is invalid for integer")
		}
		if err := e.writeNumber(TypeInteger, value.Integer); err != nil {
			return fmt.Errorf("resp: write integer: %w", err)
		}
		return nil
	case TypeBulkString:
		if value.Null {
			if err := e.writeNumber(TypeBulkString, -1); err != nil {
				return fmt.Errorf("resp: write null bulk string: %w", err)
			}
			return nil
		}
		if err := e.writeNumber(TypeBulkString, int64(len(value.Text))); err != nil {
			return fmt.Errorf("resp: write bulk string length: %w", err)
		}
		if _, err := e.writer.WriteString(value.Text); err != nil {
			return fmt.Errorf("resp: write bulk string payload: %w", err)
		}
		if _, err := e.writer.WriteString("\r\n"); err != nil {
			return fmt.Errorf("resp: write bulk string terminator: %w", err)
		}
		return nil
	case TypeArray:
		if value.Null {
			if err := e.writeNumber(TypeArray, -1); err != nil {
				return fmt.Errorf("resp: write null array: %w", err)
			}
			return nil
		}
		if err := e.writeNumber(TypeArray, int64(len(value.Elements))); err != nil {
			return fmt.Errorf("resp: write array length: %w", err)
		}
		for _, element := range value.Elements {
			if err := e.Encode(element); err != nil {
				return fmt.Errorf("resp: write array element: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("resp: cannot encode type %q", value.Type)
	}
}

func (e *Encoder) writeNumber(prefix Type, number int64) error {
	if err := e.writer.WriteByte(byte(prefix)); err != nil {
		return err
	}

	var buffer [20]byte
	digits := strconv.AppendInt(buffer[:0], number, 10)
	if _, err := e.writer.Write(digits); err != nil {
		return err
	}
	if _, err := e.writer.WriteString("\r\n"); err != nil {
		return err
	}
	return nil
}
