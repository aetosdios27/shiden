package server

import (
	"bufio"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/aetosdios27/shiden/internal/resp"
)

func TestServeConnectionHandlesPipelinedCommands(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
	})
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	go serveConnection(server)

	writeResult := make(chan error, 1)
	go func() {
		writer := bufio.NewWriter(client)
		if _, err := writer.WriteString("*1\r\n$4\r\nPING\r\n*2\r\n$4\r\nECHO\r\n$5\r\nhello\r\n"); err != nil {
			writeResult <- err
			return
		}
		writeResult <- writer.Flush()
	}()

	decoder := resp.NewDecoder(bufio.NewReader(client))
	first, err := decoder.Decode()
	if err != nil {
		t.Fatalf("first Decode() error = %v", err)
	}
	if want := resp.SimpleString("PONG"); !reflect.DeepEqual(first, want) {
		t.Fatalf("first response = %#v, want %#v", first, want)
	}

	second, err := decoder.Decode()
	if err != nil {
		t.Fatalf("second Decode() error = %v", err)
	}
	if want := resp.BulkString("hello"); !reflect.DeepEqual(second, want) {
		t.Fatalf("second response = %#v, want %#v", second, want)
	}

	if err := <-writeResult; err != nil {
		t.Fatalf("write pipelined commands: %v", err)
	}
}
