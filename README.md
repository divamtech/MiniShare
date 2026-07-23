# MiniShare ⚡

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org)

**MiniShare** is an open-source, real-time P2P terminal sharing tool and cloud signaling server written in Go. It enables secure, encrypted terminal sharing directly between machines using WebRTC DataChannels.

Terminal data flows **100% peer-to-peer (P2P)** with End-to-End Encryption — the cloud signaling server only handles initial session UUID discovery and never touches terminal data.

---

## 📂 Repository Architecture

MiniShare is organized into two completely decoupled Go modules:

```text
MiniShare/
├── cli/                        # 💻 Terminal CLI Application (Host & Viewer)
│   ├── main.go                 # Unified CLI Binary
│   ├── config.go               # Persistent Config (~/.minishare/config.json)
│   ├── go.mod                  # Independent CLI Go module
│   ├── go.sum
│   └── README.md
│
├── server/                     # 🌐 Cloud Signaling Server & Web SPA
│   ├── main.go                 # Pure Go HTTP Signaling Server
│   ├── index.html              # Embedded Web Terminal SPA (xterm.js)
│   ├── go.mod                  # Independent Server Go module
│   ├── Dockerfile              # Container deployment for Cloud (Render/Fly.io/AWS)
│   └── README.md
│
├── LICENSE
├── README.md                   # Main Documentation
└── .gitignore
```

---

## 🚀 Quick Start

### 1. Build the CLI Binary

```bash
cd cli
go build -o minishare main.go
```

---

### 2. Configure Signaling Server (Optional)

By default, MiniShare CLI connects to `http://localhost:8080`.

- **Set a custom signaling server**:
  ```bash
  ./minishare server http://localhost:8080
  # [MiniShare] Signaling server set to: http://localhost:8080
  ```

- **Reset to default signaling server**:
  ```bash
  ./minishare server reset
  # [MiniShare] Signaling server reset to default: http://localhost:8080
  ```

---

### 3. Start Host Session (Computer A)

```bash
./minishare
```

Output:
```text
[MiniShare] Connecting to signaling server: http://localhost:8080

⚡ MiniShare Host Session Live
🔑 Session UUID: 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
💻 Connect via CLI: minishare connect 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
🌐 Connect via Web Browser: http://localhost:8080/#7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
👉 Session UUID copied to clipboard automatically!
```

---

### 4. Connect as Viewer (Computer B)

#### Option A: CLI Viewer (Computer B)
```bash
./minishare connect 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
```
*(Press `Ctrl+]` or type `exit` to detach at any time)*

#### Option B: Web Browser Viewer (Zero Installation Required!)
Open the Web Link in Chrome, Safari, or Firefox:
```text
http://localhost:8080/#7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
```
An interactive terminal renders directly inside your browser window using `xterm.js` connected live to Computer A via WebRTC P2P!

---

## 🌐 Running & Deploying the Signaling Server

```bash
cd server
go run main.go --port 8080
```

To deploy to Docker / Render / Fly.io / AWS / Railway:
```bash
docker build -t minishare-server ./server
docker run -p 8080:8080 minishare-server
```

---

## 🗺️ Future Architecture Roadmap

We are actively expanding MiniShare with advanced networking features:

- **⚡ Ngrok-style Tunneling**: Global HTTP, Database (PostgreSQL, MySQL, Redis), and TCP/UDP port forwarding tunneled directly through WebRTC P2P DataChannels.
- **🔐 Multi-Factor Session Auth**: Optional PIN / passphrase protection for shared sessions.
- **👥 Multi-Viewer Broadcast**: Shared read-only terminal sessions for live pairing, teaching, and demos.

---

## 📄 License

Distributed under the [MIT License](LICENSE).
