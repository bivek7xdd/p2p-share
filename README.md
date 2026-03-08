# p2p-share

A peer-to-peer file sharing application built with Go and WebRTC.

## Project Structure

- **server/** - The Matchmaker (Signaling Server) that facilitates peer connections
- **cmd/send/** - CLI tool for sending files
- **cmd/receive/** - CLI tool for receiving files
- **internal/webrtc/** - Shared WebRTC configuration and utilities

## Usage

### Starting the Signaling Server

```bash
go run server/main.go
```

### Sending a File

```bash
go run cmd/send/main.go [options]
```

### Receiving a File

```bash
go run cmd/receive/main.go [options]
```

## Setup

1. Clone the repository
2. Run `go mod download` to install dependencies
3. Build or run using `go run` commands

## Requirements

- Go 1.21 or higher
- WebRTC library (to be added to go.mod)
