# MiniShare CLI ⚡

The official command-line tool for **MiniShare** — real-time P2P terminal sharing.

## Installation & Setup

```bash
cd cli
go build -o minishare main.go
```

Move the compiled binary to your PATH (optional):
```bash
sudo mv minishare /usr/local/bin/
```

---

## Commands

### 1. Start Host Session
Share your terminal shell with a remote peer:
```bash
minishare
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
minishare connect 7f8a91b2-3c4d-4e5f-a6b7-8c9d0e1f2a3b
```
*(Press `Ctrl+]` or type `exit` to detach from the remote shell at any time)*

---

### 3. Server Configuration

Set a custom signaling server:
```bash
minishare server http://localhost:8080
```

Reset back to the default signaling server:
```bash
minishare server reset
```
*(Also supports `default`, `null`, `empty`, or blank)*

---

### 4. Manual Fallback Mode
Run without a signaling server (manual copy-paste mode):
```bash
minishare --manual
```
