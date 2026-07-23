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

---

## Commands & Clean Syntax

### 1. Host Commands (Foreground or Background Daemon `-d`)

- **Start Host Session** (fresh UUID per session):
  ```bash
  minishare
  ```

- **Start Host Session in Background Daemon Mode**:
  ```bash
  minishare -d
  ```

- **Start Host Session with Custom UUID in Background**:
  ```bash
  minishare uuid team-room -d
  ```

- **Check Daemon Status & Active UUID**:
  ```bash
  minishare daemon status
  ```

- **Stop Running Background Daemon**:
  ```bash
  minishare kill -d
  ```

---

### 2. Viewer Command

- **Connect to Remote Host Session**:
  ```bash
  minishare connect <session-uuid>
  # Aliases: minishare -c <session-uuid> or minishare c <session-uuid>
  ```
  *(Press `Ctrl+]` or type `exit` to detach at any time)*

---

### 3. Symmetric `set` & `reset` Configuration Commands

`set <property> <value>` matches 1-to-1 with `reset <property>`:

| Target Property | Set Command | Reset Command |
| :--- | :--- | :--- |
| **Signaling Server** | `minishare set server <url>` | `minishare reset server` |
| **Persistent UUID** | `minishare set uuid <uuid>` | `minishare reset uuid` |
| **UUID Duration** | `minishare set share <1h\|2mo>` | `minishare reset share` |
| **Config File Path** | `minishare set path <file-path>` | `minishare reset path` |
| **ALL Settings** | — | `minishare reset` *(or `reset default` / `reset all`)* |

---

### 4. Configuration Overview (`config`)

- **View Active Settings & Config File Location**:
  ```bash
  minishare config
  ```
