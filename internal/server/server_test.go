package server

import (
	"bufio"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/aetosdios27/shiden/internal/resp"
	"github.com/aetosdios27/shiden/internal/store"
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
	go serveConnection(server, store.New())

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

func TestServeConnectionSharesStateAcrossConnections(t *testing.T) {
	t.Parallel()

	database := store.New()
	setResponse := executeRequest(t, database, "*3\r\n$3\r\nSET\r\n$1\r\nx\r\n$5\r\nhello\r\n")
	if want := resp.SimpleString("OK"); !reflect.DeepEqual(setResponse, want) {
		t.Fatalf("SET response = %#v, want %#v", setResponse, want)
	}

	getResponse := executeRequest(t, database, "*2\r\n$3\r\nGET\r\n$1\r\nx\r\n")
	if want := resp.BulkString("hello"); !reflect.DeepEqual(getResponse, want) {
		t.Fatalf("GET response on second connection = %#v, want %#v", getResponse, want)
	}
}

func TestServeConnectionContinuesAfterCommandError(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
	})
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	go serveConnection(server, store.New())

	request := "*1\r\n$3\r\nSET\r\n" +
		"*3\r\n$3\r\nSET\r\n$1\r\nx\r\n$5\r\nhello\r\n" +
		"*2\r\n$3\r\nGET\r\n$1\r\nx\r\n"
	writeResult := make(chan error, 1)
	go func() {
		writer := bufio.NewWriter(client)
		if _, err := writer.WriteString(request); err != nil {
			writeResult <- err
			return
		}
		writeResult <- writer.Flush()
	}()

	decoder := resp.NewDecoder(bufio.NewReader(client))
	want := []resp.Value{
		resp.Error("ERR wrong number of arguments for 'set' command"),
		resp.SimpleString("OK"),
		resp.BulkString("hello"),
	}
	for index, expected := range want {
		got, err := decoder.Decode()
		if err != nil {
			t.Fatalf("response %d Decode() error = %v", index, err)
		}
		if !reflect.DeepEqual(got, expected) {
			t.Fatalf("response %d = %#v, want %#v", index, got, expected)
		}
	}

	if err := <-writeResult; err != nil {
		t.Fatalf("write commands: %v", err)
	}
}

func executeRequest(t *testing.T, database *store.Store, request string) resp.Value {
	t.Helper()

	client, server := net.Pipe()
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	serverDone := make(chan struct{})
	go func() {
		serveConnection(server, database)
		close(serverDone)
	}()

	writeResult := make(chan error, 1)
	go func() {
		writer := bufio.NewWriter(client)
		if _, err := writer.WriteString(request); err != nil {
			writeResult <- err
			return
		}
		writeResult <- writer.Flush()
	}()

	response, err := resp.NewDecoder(bufio.NewReader(client)).Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if err := <-writeResult; err != nil {
		t.Fatalf("write request: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	<-serverDone
	return response
}
