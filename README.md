# MiniShare ⚡

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.23%2B-blue.svg)](https://golang.org)

**MiniShare** is a lightweight, zero-infrastructure terminal-sharing CLI application written in Go. It enables secure, real-time P2P terminal sharing directly between two machines using WebRTC DataChannels and pseudo-terminals (`pty`).

No signaling servers, cloud services, databases, or user accounts are required.

---

## 🚀 Features

- **⚡ DEFLATE Compression**: Offer & Answer SDP payloads are compressed using DEFLATE, reducing code lengths by **75%+** (~280 characters).
- **📋 Auto-Clipboard Integration**: Automatically copies connection codes to your system clipboard (`pbcopy` on macOS / `xclip` on Linux / `clip` on Windows).
- **⏎ One-Key Clipboard Input**: When prompted for a code, simply press **Enter** to read directly from your system clipboard.
- **⌨️ Live Command Logging**: The host terminal logs viewer commands in real-time as they execute.
- **🔌 Instant Hotkey Detach**: Press `Ctrl+]` in the viewer terminal to detach from the remote host at any time.
- **🛡 Input Sanitization**: Automatically strips line-breaks, spaces, and quotes introduced by terminal wrapping.

---

## 📦 Setup Instructions

1. Clone the repository:
   ```bash
   git clone git@github.com:divamtech/MiniShare.git
   cd MiniShare
   ```

2. Download module dependencies:
   ```bash
   go mod tidy
   ```

---

## 💻 How to Run

### Step 1: Start the Host Session
On the machine whose terminal you want to share:

```bash
go run main.go
```

1. The host generates a compressed code and automatically copies it to your system clipboard:
   ```text
   === Share this code with the viewer ===
   vJRLb9pOFMW_ysjLv...
   === end code ===
   👉 Code copied to system clipboard automatically!
   ```
2. Keep this host terminal open.

---

### Step 2: Start the Viewer Session
On the machine that wants to view/control the remote terminal:

```bash
go run ./viewer/main.go
```

1. Press **Enter** (it automatically reads the host code from your system clipboard) or paste it manually.
2. The viewer will generate an answer code and copy it to your system clipboard automatically:
   ```text
   === Send this code back to the host ===
   tJJda_s2GMW_ivDl...
   === end code ===
   👉 Code copied to system clipboard automatically!
   ```

---

### Step 3: Complete Connection
In the **Host** terminal, simply press **Enter** (to read the viewer's answer code from your clipboard).

The WebRTC P2P DataChannel will open instantly, connecting the shell session!

---

## 💡 How Signaling Works

WebRTC requires an exchange of Session Description Protocol (SDP) payloads (containing encryption fingerprints and NAT ICE candidate IP addresses) to establish a direct P2P link.

MiniShare is built to be **100% serverless, private, and zero-infrastructure**. Without a central cloud server, you (the user) act as the human signaling mechanism by exchanging the compressed SDP payloads directly via clipboard or copy-paste.

---

## 📄 License

Distributed under the [MIT License](LICENSE).
