package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/aetosdios27/shiden/internal/command"
	"github.com/aetosdios27/shiden/internal/resp"
)

// DefaultAddress is Shiden's wire-protocol listen address.
const DefaultAddress = ":6380"

// Server owns the TCP listener and connection loops.
type Server struct {
	Address string
}

// ListenAndServe listens on the configured TCP address and serves clients.
func (s *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.Address, err)
	}
	defer listener.Close()

	for {
		connection, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("accept connection: %w", err)
		}
		go serveConnection(connection)
	}
}

func serveConnection(connection net.Conn) {
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
			response = command.Execute(cmd)
		}

		if err := encoder.Encode(response); err != nil {
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}
