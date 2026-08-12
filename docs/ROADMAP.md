# Shiden Roadmap

Shiden is complete at its deliberately bounded scope. The project stops at a correct, concurrent, Redis-compatible in-memory server rather than expanding into a Redis reimplementation.

## Completed: Wire

- TCP listener and one connection loop per client
- persistent and pipelined requests
- buffered RESP2 stream decoding and encoding
- strict malformed-input handling and framing bounds
- command parsing and Redis-style error responses

## Completed: Shared memory

- `PING`, `ECHO`, `SET`, `GET`, and variadic `DEL`
- one process-wide keyspace shared across connections
- binary-safe values with explicit copy ownership
- missing values distinct from empty values
- one `sync.RWMutex` protecting concurrent access

## Completed: Time

- basic `EXPIRE key seconds`
- lazy expiry during key operations
- periodic expired-key cleanup
- deterministic store tests through an injected clock
- SET clearing prior expiry

## Completed: Lifecycle and evidence

- SIGINT and SIGTERM shutdown
- listener and active-connection closure
- connection-handler draining
- real TCP integration tests
- one-byte fragmentation and pipelining tests
- binary wire round trips
- race-detector and Redis CLI interoperability checks

## Deliberately excluded

- advanced Redis data structures and broad command compatibility
- persistence and crash recovery
- transactions, pub/sub, eviction, replication, and clustering
- authentication and production resource management
- custom allocators, event-loop redesigns, benchmarks, and performance claims

Further work in those areas is a new project milestone, not unfinished Shiden work.
