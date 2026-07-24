# MiniShare Cloud Signaling Server & Web SPA 🌐

This package contains the standalone Go **Signaling Server** and embedded **Web Terminal SPA** for MiniShare.

## Features

- **Pure Go HTTP Server**: Zero external dependencies, ultra-fast, <10MB RAM footprint.
- **REST Signaling API**: Handles Session UUID creation and WebRTC SDP offer/answer exchange.
- **Embedded Web Terminal SPA**: Serves `index.html` featuring `xterm.js` for zero-install browser terminal control.
- **Cloud Ready**: Includes `Dockerfile` for instant deployment to Render, Fly.io, Railway, AWS, or GCP.

---

## Installation & Local Execution

1. Navigate to the `server/` directory:
   ```bash
   cd server
   ```

2. Install/verify module dependencies:
   ```bash
   go mod tidy
   ```

3. Run the signaling server:
   ```bash
   go run main.go --port 8080
   ```

Open `http://localhost:8080` in your web browser to test the Web Terminal viewer interface!

---
## Direct binary
```sh
cd server

# macOS (Apple Silicon - M1/M2/M3/M4)
GOOS=darwin GOARCH=arm64 go build -o server
zip server-mac-silicon.zip server index.html app.html

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o server
zip server-mac-intel.zip server index.html app.html

# Linux
GOOS=linux GOARCH=amd64 go build -o server
zip server-linux.zip server index.html app.html

# Windows
GOOS=windows GOARCH=amd64 go build -o server.exe
zip server-windows.zip server.exe index.html app.html

```
## Docker Deployment

Build and run container locally:
```bash
docker build -t minishare-server .
docker run -p 8080:8080 minishare-server
```
