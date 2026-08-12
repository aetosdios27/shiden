package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/aetosdios27/shiden/internal/command"
	"github.com/aetosdios27/shiden/internal/resp"
	"github.com/aetosdios27/shiden/internal/store"
)

const (
	// DefaultAddress is Shiden's wire-protocol listen address.
	DefaultAddress = ":6380"

	expirationCleanupInterval = time.Second
)

// Server owns the TCP listener and connection loops.
type Server struct {
	Address  string
	database *store.Store
}

// New constructs a server with one process-wide datastore shared by every
// client connection.
func New(address string) *Server {
	return &Server{
		Address:  address,
		database: store.New(),
	}
}

// ListenAndServe listens on the configured TCP address and serves clients until
// context cancellation initiates a clean shutdown.
func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.Address, err)
	}
	log.Printf("Shiden listening on %s", listener.Addr())

	return s.serve(ctx, listener)
}

func (s *Server) serve(ctx context.Context, listener net.Listener) error {
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		connectionsMutex sync.Mutex
		connections      = make(map[net.Conn]struct{})
		connectionGroup  sync.WaitGroup
		shutdownOnce     sync.Once
	)

	shutdown := func() {
		shutdownOnce.Do(func() {
			_ = listener.Close()

			connectionsMutex.Lock()
			for connection := range connections {
				_ = connection.Close()
			}
			connectionsMutex.Unlock()
		})
	}

	shutdownWatcherDone := make(chan struct{})
	go func() {
		select {
		case <-serveCtx.Done():
			shutdown()
		case <-shutdownWatcherDone:
		}
	}()

	reaperDone := make(chan struct{})
	go func() {
		defer close(reaperDone)
		s.reapExpired(serveCtx)
	}()

	defer func() {
		close(shutdownWatcherDone)
		cancel()
		shutdown()
		connectionGroup.Wait()
		<-reaperDone
	}()

	for {
		connection, err := listener.Accept()
		if err != nil {
			if serveCtx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept connection: %w", err)
		}

		connectionsMutex.Lock()
		if serveCtx.Err() != nil {
			connectionsMutex.Unlock()
			_ = connection.Close()
			return nil
		}
		connections[connection] = struct{}{}
		connectionGroup.Add(1)
		connectionsMutex.Unlock()

		go func() {
			defer func() {
				connectionsMutex.Lock()
				delete(connections, connection)
				connectionsMutex.Unlock()
				connectionGroup.Done()
			}()
			serveConnection(connection, s.database)
		}()
	}
}

func (s *Server) reapExpired(ctx context.Context) {
	ticker := time.NewTicker(expirationCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.database.DeleteExpired()
		}
	}
}

func serveConnection(connection net.Conn, database *store.Store) {
	defer connection.Close()

	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	decoder := resp.NewDecoder(reader)
	encoder := resp.NewEncoder(writer)

	for {
		frame, err := decoder.Decode()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			if err := encoder.Encode(resp.Error("ERR Protocol error: " + err.Error())); err != nil {
				return
			}
			if err := writer.Flush(); err != nil {
				return
			}
			return
		}

		cmd, err := command.Parse(frame)
		var response resp.Value
		if err != nil {
			response = resp.Error("ERR " + err.Error())
		} else {
			response = command.Execute(cmd, database)
		}

		if err := encoder.Encode(response); err != nil {
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}
