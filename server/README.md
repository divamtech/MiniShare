# MiniShare Cloud Signaling Server & Web SPA 🌐

This package contains the standalone Go **Signaling Server** and embedded **Web Terminal SPA** for MiniShare.

## Features

- **Pure Go HTTP Server**: Zero external dependencies, ultra-fast, <10MB RAM footprint.
- **REST Signaling API**: Handles Session UUID creation and WebRTC SDP offer/answer exchange.
- **Embedded Web Terminal SPA**: Serves `index.html` featuring `xterm.js` for zero-install browser terminal control.
- **Cloud Ready**: Includes `Dockerfile` for instant deployment to Render, Fly.io, Railway, AWS, or GCP.

---

## Local Setup & Execution

```bash
cd server
go run main.go --port 8080
```

Open `http://localhost:8080` in your web browser to test the Web Terminal viewer interface!

---

## Docker Deployment

Build and run container locally:
```bash
docker build -t minishare-server .
docker run -p 8080:8080 minishare-server
```
