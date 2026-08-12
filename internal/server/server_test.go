package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
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

func TestServeAcceptsRealTCPClientsAndSharesState(t *testing.T) {
	t.Parallel()

	running := startTestServer(t)
	if got := executeTCPRequest(t, running.address, "*3\r\n$3\r\nSET\r\n$4\r\nname\r\n$6\r\nshiden\r\n"); !reflect.DeepEqual(got, resp.SimpleString("OK")) {
		t.Fatalf("SET response = %#v, want OK", got)
	}
	if got := executeTCPRequest(t, running.address, "*2\r\n$3\r\nGET\r\n$4\r\nname\r\n"); !reflect.DeepEqual(got, resp.BulkString("shiden")) {
		t.Fatalf("GET response on second TCP connection = %#v, want shiden", got)
	}
}

func TestServeHandlesExpireOverTCP(t *testing.T) {
	t.Parallel()

	running := startTestServer(t)
	if got := executeTCPRequest(t, running.address, "*3\r\n$3\r\nSET\r\n$7\r\nsession\r\n$5\r\nalive\r\n"); !reflect.DeepEqual(got, resp.SimpleString("OK")) {
		t.Fatalf("SET response = %#v, want OK", got)
	}
	if got := executeTCPRequest(t, running.address, "*3\r\n$6\r\nEXPIRE\r\n$7\r\nsession\r\n$1\r\n0\r\n"); !reflect.DeepEqual(got, resp.Integer(1)) {
		t.Fatalf("EXPIRE response = %#v, want integer 1", got)
	}
	if got := executeTCPRequest(t, running.address, "*2\r\n$3\r\nGET\r\n$7\r\nsession\r\n"); !reflect.DeepEqual(got, resp.NullBulkString()) {
		t.Fatalf("GET response after expiration = %#v, want null bulk string", got)
	}
}

func TestServeCancellationClosesIdleConnections(t *testing.T) {
	t.Parallel()

	running := startTestServer(t)
	client, err := net.Dial("tcp", running.address)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}

	running.stop(t)

	buffer := make([]byte, 1)
	if _, err := client.Read(buffer); err == nil {
		t.Fatal("Read() after shutdown error = nil, want closed connection")
	}
	if connection, err := net.DialTimeout("tcp", running.address, 100*time.Millisecond); err == nil {
		_ = connection.Close()
		t.Fatal("Dial() after shutdown error = nil, want listener closed")
	}
}

func TestServeReturnsUnexpectedAcceptErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("accept failed")
	shiden := New("unused")
	err := shiden.serve(context.Background(), &failingListener{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("serve() error = %v, want wrapped %v", err, want)
	}
}

func TestServeConnectionRoundTripsBinaryBulkStrings(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
	})
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	go serveConnection(server, store.New())

	value := []byte{0x00, 0xff, 'a', '\r', '\n', 'b'}
	var request bytes.Buffer
	fmt.Fprintf(&request, "*3\r\n$3\r\nSET\r\n$6\r\nbinary\r\n$%d\r\n", len(value))
	request.Write(value)
	request.WriteString("\r\n*2\r\n$3\r\nGET\r\n$6\r\nbinary\r\n")

	writeResult := make(chan error, 1)
	go func() {
		_, err := client.Write(request.Bytes())
		writeResult <- err
	}()

	decoder := resp.NewDecoder(bufio.NewReader(client))
	setResponse, err := decoder.Decode()
	if err != nil {
		t.Fatalf("SET response Decode() error = %v", err)
	}
	if want := resp.SimpleString("OK"); !reflect.DeepEqual(setResponse, want) {
		t.Fatalf("SET response = %#v, want %#v", setResponse, want)
	}

	getResponse, err := decoder.Decode()
	if err != nil {
		t.Fatalf("GET response Decode() error = %v", err)
	}
	if getResponse.Type != resp.TypeBulkString || getResponse.Null || !bytes.Equal([]byte(getResponse.Text), value) {
		t.Fatalf("GET response = %#v, want binary bulk string %v", getResponse, value)
	}
	if err := <-writeResult; err != nil {
		t.Fatalf("Write() error = %v", err)
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

type testServer struct {
	address string
	cancel  context.CancelFunc
	done    chan error
	once    sync.Once
}

func startTestServer(t *testing.T) *testServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	running := &testServer{
		address: listener.Addr().String(),
		cancel:  cancel,
		done:    make(chan error, 1),
	}
	shiden := New(running.address)
	go func() {
		running.done <- shiden.serve(ctx, listener)
	}()
	t.Cleanup(func() {
		running.stop(t)
	})
	return running
}

func (s *testServer) stop(t *testing.T) {
	t.Helper()

	s.once.Do(func() {
		s.cancel()
		select {
		case err := <-s.done:
			if err != nil {
				t.Errorf("serve() error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("serve() did not stop within 2 seconds")
		}
	})
}

func executeTCPRequest(t *testing.T, address string, request string) resp.Value {
	t.Helper()

	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	if _, err := connection.Write([]byte(request)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	response, err := resp.NewDecoder(bufio.NewReader(connection)).Decode()
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	return response
}

type failingListener struct {
	err error
}

func (l *failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (l *failingListener) Close() error {
	return nil
}

func (l *failingListener) Addr() net.Addr {
	return staticAddress("failing-listener")
}

type staticAddress string

func (a staticAddress) Network() string {
	return "test"
}

func (a staticAddress) String() string {
	return string(a)
}
