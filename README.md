# p2p-share

A peer-to-peer file sharing application built with Go and WebRTC.

## Installation

### 🍏 Mac & Linux (via Homebrew)
Anyone in the world can open their terminal and run:

```bash
brew tap bivek7xdd/homebrew-tap
brew install p2p-share
```

### 🐧 Linux (via Snap Store)
Ubuntu, Manjaro, and other Snap-enabled Linux users can install it with one command:

```bash
sudo snap install p2p-share
```

### 📦 Debian / Ubuntu (via .deb)
Users can grab the `.deb` file directly from the GitHub Releases page and install it locally:

1. Go to: https://github.com/bivek7xdd/p2p-share/releases/latest
2. Download the `_linux_amd64.deb` file.
3. Run this in the terminal where it downloaded:

```bash
sudo apt install ./p2p-share_*_linux_amd64.deb
```

### 🪟 Windows (via ZIP)
Windows users don't need a package manager:

1. Go to: https://github.com/bivek7xdd/p2p-share/releases/latest
2. Download the `_windows_amd64.zip` file.
3. Extract it and double-click `p2p-share.exe`.

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
