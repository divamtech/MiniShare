# MiniShare CLI ⚡

The official command-line tool for **MiniShare** — real-time P2P terminal sharing.

## Installation & Setup

1. Navigate to the `cli/` directory:
   ```bash
   cd cli
   ```

2. Download module dependencies:
   ```bash
   go mod tidy
   ```

3. Build the executable binary:
   ```bash
   go build -o minishare main.go
   ```

*(Optional) Move the compiled binary to your PATH:*
```bash
sudo mv minishare /usr/local/bin/
```

---

## Running without Building

You can also run directly using Go:
```bash
go run main.go
```

---

## Commands

### 1. Start Host Session
Share your terminal shell with a remote peer:
```bash
./minishare
# Or: go run main.go
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

### 2. Connect as Viewer
Connect to a remote host using its session UUID:
```bash
./minishare connect 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
# Or: go run main.go connect 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
```
*(Press `Ctrl+]` or type `exit` to detach from the remote shell at any time)*

---

### 3. Server Configuration

Set a custom signaling server URL:
```bash
./minishare server http://localhost:8080
```

Reset back to the default signaling server URL:
```bash
./minishare server reset
```
*(Also accepts `default`, `null`, `empty`, or blank)*

---

### 4. Manual Fallback Mode
Run without a signaling server (manual copy-paste mode):
```bash
./minishare --manual
```
