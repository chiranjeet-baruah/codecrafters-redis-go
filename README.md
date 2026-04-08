[![progress-banner](https://backend.codecrafters.io/progress/redis/6f776a61-ecda-4e30-9fb3-606eea516078)](https://app.codecrafters.io/users/codecrafters-bot?r=2qF)

# Build Your Own Redis — Go

A Redis-compatible server built from scratch in Go as part of the [CodeCrafters "Build Your Own Redis" challenge](https://codecrafters.io/challenges/redis).

## Overview

This project implements a subset of the Redis protocol, including:

- TCP server on `0.0.0.0:6379`
- [RESP (Redis Serialization Protocol)](https://redis.io/docs/reference/protocol-spec/) parsing and encoding
- Commands: `PING`, `ECHO`, `SET`, `GET`
- In-memory key-value store with optional TTL support
- Concurrent client handling via goroutines

## Requirements

- Go 1.26+

## Getting Started

**Build:**

```bash
go build -o /tmp/codecrafters-build-redis-go app/*.go
```

**Run:**

```bash
./your_program.sh

# With a custom port:
./your_program.sh --port 6380
```

**Format:**

```bash
gofmt -w app/*.go
```

## Project Structure

```
.
├── app/
│   └── main.go          # Entry point and server implementation
├── your_program.sh      # Local compile-and-run wrapper
├── .codecrafters/
│   ├── compile.sh       # Remote build script
│   └── run.sh           # Remote execution script
├── go.mod
└── README.md
```

## Testing

There is no local test suite. Validation is handled remotely by the CodeCrafters platform. Push your changes to run the test harness:

```bash
git push origin master
```

## CodeCrafters Stages

Each stage builds on the previous. After implementing a stage, commit and push:

```bash
git commit -am "implement stage N"
git push origin master
```

Test output streams directly to your terminal.

> **Note:** If you're viewing this on GitHub, head to [codecrafters.io](https://codecrafters.io) to attempt the challenge yourself.

## License

This project is for educational purposes as part of the CodeCrafters challenge platform.