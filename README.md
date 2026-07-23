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
│   ├── main.go                 # Self-contained Unified CLI Binary
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

## 📦 Dependency Installation & Quick Start

### 1. Install CLI Dependencies & Build

```bash
cd cli
go mod tidy
go build -o minishare main.go
```

---

### 2. Start Host Session

- **Standard Host Mode** (fresh UUID by default for safe use):
  ```bash
  ./minishare
  ```

- **Background Daemon Mode (`-d`)**:
  ```bash
  ./minishare -d                        # Run Host in background
  ./minishare uuid team-room -d         # Custom UUID + background daemon
  ./minishare daemon status             # Check daemon status & Session UUID
  ./minishare kill -d                   # Stop background daemon
  ```

- **Persistent / Duration-based UUID (`share` / `uuid`)**:
  ```bash
  ./minishare set share 1h              # Persistent for 1 hour
  ./minishare set share 2mo             # Persistent for 2 months
  ./minishare set share 4y              # Persistent for 4 years
  ./minishare set uuid team-room        # Fixed UUID permanently
  ```

---

### 3. Symmetric Configuration Management (`set` & `reset`)

| Target Property | Set Command | Reset Command |
| :--- | :--- | :--- |
| **Signaling Server** | `./minishare set server <url>` | `./minishare reset server` |
| **Persistent UUID** | `./minishare set uuid <uuid>` | `./minishare reset uuid` |
| **UUID Duration** | `./minishare set share <1h\|2mo>` | `./minishare reset share` |
| **Config File Path** | `./minishare set path <file-path>` | `./minishare reset path` |
| **ALL Settings** | — | `./minishare reset` *(or `reset default` / `reset all`)* |

---

### 4. Connect as Viewer

#### Option A: CLI Viewer
```bash
./minishare connect <session-uuid>
# Aliases: ./minishare -c <session-uuid> or ./minishare c <session-uuid>
```
*(Press `Ctrl+]` or type `exit` to detach at any time)*

#### Option B: Web Browser Viewer (Zero Installation Required!)
Open `http://localhost:8080/app/<session-uuid>` in Chrome, Safari, or Firefox.
An interactive terminal renders directly inside your browser window using `xterm.js` connected live to the Host via WebRTC P2P!

---

## 🌐 Running & Deploying the Signaling Server

```bash
cd server
go mod tidy
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
