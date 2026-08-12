# Shiden

Shiden is a deliberately small Redis-compatible in-memory database server written from first principles in Go. It implements the complete path from a TCP byte stream through RESP2 framing and command execution to shared, synchronized process state.

Shiden is a learning system, not a production Redis replacement. It uses only the Go standard library.

## Status

Complete at its bounded scope:

- TCP listener on port `6380`
- one goroutine per client connection
- persistent connections and pipelined commands
- streaming RESP2 decoding and encoding
- one process-wide binary-safe store
- lazy expiration plus periodic expired-key cleanup
- signal-aware shutdown for SIGINT and SIGTERM

## Run

```bash
go run ./cmd/shiden
```

The server logs its bound address after the listener is ready. Press Ctrl-C to stop accepting clients, close active connections, and wait for connection handlers to exit.

## Commands

| Command | Behavior |
|---|---|
| `PING` | Returns `PONG` |
| `PING message` | Returns `message` |
| `ECHO message` | Returns `message` |
| `SET key value` | Stores a binary-safe value and clears any previous expiry |
| `GET key` | Returns the value, or a null bulk string when missing or expired |
| `DEL key [key ...]` | Deletes live keys and returns the number removed |
| `EXPIRE key seconds` | Sets a lifetime and returns `1`, or `0` when the key is missing or expired |

`EXPIRE` accepts signed integer seconds. Zero or negative seconds delete a live key immediately. Options such as NX, XX, GT, and LT are intentionally unsupported.

## Redis CLI

Shiden speaks RESP2 and works with a real Redis-compatible CLI:

```bash
redis-cli -p 6380 PING
redis-cli -p 6380 SET name shiden
redis-cli -p 6380 GET name
redis-cli -p 6380 EXPIRE name 10
redis-cli -p 6380 DEL name
```

Separate CLI invocations use separate TCP connections while observing the same server-owned state.

## Verify

```bash
go test ./...
go test -race ./...
go vet ./...
```

The tests cover RESP2 values and malformed frames, one-byte fragmentation, pipelining, command validation, missing versus empty values, arbitrary binary payloads, byte ownership, deterministic expiration, shared state across real TCP clients, concurrent store access, and controlled server shutdown.

## Architecture

- `cmd/shiden` owns process signals and starts the server.
- `internal/server` owns the listener, active connections, buffered connection loop, cleanup loop, and shutdown.
- `internal/resp` owns RESP2 framing and encoding.
- `internal/command` owns command parsing, validation, and execution.
- `internal/store` owns key/value state, expiration, byte ownership, and synchronization.

## Explicit limitations

Shiden does not implement persistence, advanced Redis data structures, transactions, pub/sub, eviction, replication, clustering, authentication, broad Redis compatibility, production resource controls, or performance claims. Those are outside this project's finish line.
