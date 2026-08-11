package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/pion/webrtc/v4"
	"golang.org/x/term"
)

// -------------------------------------------------------------------
// CONFIGURATION MANAGER & CROSS-PLATFORM STORAGE
// -------------------------------------------------------------------
var Version = "dev"
const DefaultServerURL = "https://minishare.divamtech.com"

type Config struct {
	CustomConfigPath string    `json:"custom_config_path,omitempty"`
	ServerURL        string    `json:"server_url"`
	PersistentUUID   string    `json:"persistent_uuid,omitempty"`
	UUIDExpiresAt    time.Time `json:"uuid_expires_at,omitempty"`
	BlockedCommands  []string  `json:"blocked_commands,omitempty"`
	BlockedFolders   []string  `json:"blocked_folders,omitempty"`
}

func getDefaultConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		dir := filepath.Join(configDir, "minishare")
		_ = os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "config.json")
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		dir := filepath.Join(homeDir, ".minishare")
		_ = os.MkdirAll(dir, 0755)
		return filepath.Join(dir, "config.json")
	}

	return "minishare_config.json"
}

func getPathPointerFile() string {
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".minishare_path")
	}
	return ".minishare_path"
}

func GetConfigPath() string {
	if envPath := os.Getenv("MINISHARE_CONFIG"); envPath != "" {
		return envPath
	}

	if pointerFile := getPathPointerFile(); pointerFile != "" {
		if data, err := os.ReadFile(pointerFile); err == nil {
			customPath := strings.TrimSpace(string(data))
			if customPath != "" {
				return customPath
			}
		}
	}

	return getDefaultConfigPath()
}

func LoadConfig() *Config {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return &Config{ServerURL: DefaultServerURL}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.ServerURL == "" {
		return &Config{ServerURL: DefaultServerURL}
	}
	return &cfg
}

func SaveConfig(cfg *Config) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func getDaemonPIDPath() string {
	dir := filepath.Dir(GetConfigPath())
	return filepath.Join(dir, "daemon.pid")
}

func getDaemonUUIDPath() string {
	dir := filepath.Dir(GetConfigPath())
	return filepath.Join(dir, "daemon.uuid")
}

func getDaemonLogPath() string {
	dir := filepath.Dir(GetConfigPath())
	return filepath.Join(dir, "daemon.log")
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func parseDurationStr(s string) (time.Time, bool, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "never" || s == "permanent" || s == "0" || s == "infinite" {
		return time.Time{}, true, nil
	}

	now := time.Now()

	if strings.HasSuffix(s, "y") {
		numStr := strings.TrimSuffix(s, "y")
		years, err := strconv.Atoi(numStr)
		if err != nil {
			return time.Time{}, false, err
		}
		return now.AddDate(years, 0, 0), false, nil
	}

	if strings.HasSuffix(s, "mo") {
		numStr := strings.TrimSuffix(s, "mo")
		months, err := strconv.Atoi(numStr)
		if err != nil {
			return time.Time{}, false, err
		}
		return now.AddDate(0, months, 0), false, nil
	}

	if strings.HasSuffix(s, "d") {
		numStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return time.Time{}, false, err
		}
		return now.AddDate(0, 0, days), false, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, false, err
	}
	return now.Add(d), false, nil
}

// -------------------------------------------------------------------
// RESET COMMAND HANDLERS
// -------------------------------------------------------------------
func HandleResetCommand(args []string) {
	target := "all"
	if len(args) > 0 {
		target = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch target {
	case "default", "all":
		_ = os.Remove(getPathPointerFile())
		cfg := &Config{
			ServerURL:       DefaultServerURL,
			PersistentUUID:  "",
			UUIDExpiresAt:   time.Time{},
			BlockedCommands: nil,
			BlockedFolders:  nil,
		}
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to reset configurations: %v\n", err)
			return
		}
		fmt.Println("🔄 [MiniShare] All configurations reset to default values.")
		fmt.Printf("   🌐 Server URL: %s\n", DefaultServerURL)
		fmt.Println("   🔑 Persistent UUID: Cleared (fresh UUID per session)")
		fmt.Println("   🔒 Blocked Commands: Cleared")
		fmt.Println("   📁 Blocked Folders: Cleared")
		fmt.Printf("   📄 Config Path: %s\n", getDefaultConfigPath())

	case "server":
		cfg := LoadConfig()
		cfg.ServerURL = DefaultServerURL
		_ = SaveConfig(cfg)
		fmt.Printf("[MiniShare] Signaling server reset to default: %s\n", DefaultServerURL)

	case "uuid":
		cfg := LoadConfig()
		cfg.PersistentUUID = ""
		cfg.UUIDExpiresAt = time.Time{}
		_ = SaveConfig(cfg)
		fmt.Println("🔑 [MiniShare] Persistent UUID configuration reset to default.")

	case "share":
		cfg := LoadConfig()
		cfg.UUIDExpiresAt = time.Time{}
		_ = SaveConfig(cfg)
		fmt.Println("🔑 [MiniShare] Share duration reset (UUID expiration cleared).")

	case "path", "filepath":
		_ = os.Remove(getPathPointerFile())
		fmt.Printf("📄 Config file path reset to OS default: %s\n", getDefaultConfigPath())

	case "block":
		cfg := LoadConfig()
		cfg.BlockedCommands = nil
		cfg.BlockedFolders = nil
		_ = SaveConfig(cfg)
		fmt.Println("🔓 [MiniShare] All blocked commands and folders cleared.")

	default:
		fmt.Printf("Unknown reset target '%s'. Available: default, all, server, uuid, share, path, block\n", target)
	}
}

// -------------------------------------------------------------------
// BLOCK / UNBLOCK COMMAND HANDLERS
// -------------------------------------------------------------------
func parseBlockArgs(args []string) []string {
	var items []string
	for _, arg := range args {
		for _, part := range strings.Split(arg, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				items = append(items, p)
			}
		}
	}
	return items
}

func HandleBlockCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  minishare block cmd <cmd1> [cmd2] ...         Block commands (comma or space separated)")
		fmt.Println("  minishare block dir|folder <path1> [path2] ... Block folder access (comma or space separated)")
		return
	}

	target := strings.ToLower(args[0])
	valArgs := args[1:]

	if len(valArgs) == 0 {
		cfg := LoadConfig()
		switch target {
		case "cmd", "command":
			if len(cfg.BlockedCommands) == 0 {
				fmt.Println("🔒 [MiniShare] No commands are blocked.")
			} else {
				fmt.Printf("🔒 [MiniShare] Blocked commands: %s\n", strings.Join(cfg.BlockedCommands, ", "))
			}
		case "dir", "folder":
			if len(cfg.BlockedFolders) == 0 {
				fmt.Println("📁 [MiniShare] No folders are blocked.")
			} else {
				fmt.Printf("📁 [MiniShare] Blocked folders: %s\n", strings.Join(cfg.BlockedFolders, ", "))
			}
		default:
			fmt.Printf("Unknown block target '%s'. Use 'cmd' or 'dir/folder'.\n", target)
		}
		return
	}

	newItems := parseBlockArgs(valArgs)
	cfg := LoadConfig()

	switch target {
	case "cmd", "command":
		for _, item := range newItems {
			lowerItem := strings.ToLower(item)
			found := false
			for _, existing := range cfg.BlockedCommands {
				if strings.ToLower(existing) == lowerItem {
					found = true
					break
				}
			}
			if !found {
				cfg.BlockedCommands = append(cfg.BlockedCommands, lowerItem)
			}
		}
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to save blocked commands: %v\n", err)
		} else {
			fmt.Printf("🔒 [MiniShare] Blocked commands updated: %s\n", strings.Join(cfg.BlockedCommands, ", "))
		}

	case "dir", "folder":
		for _, item := range newItems {
			absPath, err := filepath.Abs(item)
			if err != nil {
				absPath = item
			}
			found := false
			for _, existing := range cfg.BlockedFolders {
				if existing == absPath {
					found = true
					break
				}
			}
			if !found {
				cfg.BlockedFolders = append(cfg.BlockedFolders, absPath)
			}
		}
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to save blocked folders: %v\n", err)
		} else {
			fmt.Printf("📁 [MiniShare] Blocked folders updated: %s\n", strings.Join(cfg.BlockedFolders, ", "))
		}

	default:
		fmt.Printf("Unknown block target '%s'. Use 'cmd' or 'dir/folder'.\n", target)
	}
}

func HandleUnblockCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  minishare unblock cmd <cmd1> [cmd2] ...         Unblock specific commands")
		fmt.Println("  minishare unblock dir|folder <path1> [path2] ... Unblock specific folders")
		return
	}

	target := strings.ToLower(args[0])
	valArgs := args[1:]

	if len(valArgs) == 0 {
		fmt.Printf("Please specify items to unblock. Example: minishare unblock %s <item1> [item2]\n", target)
		return
	}

	removeItems := parseBlockArgs(valArgs)
	cfg := LoadConfig()

	switch target {
	case "cmd", "command":
		var remaining []string
		for _, existing := range cfg.BlockedCommands {
			keep := true
			for _, rm := range removeItems {
				if strings.ToLower(existing) == strings.ToLower(rm) {
					keep = false
					break
				}
			}
			if keep {
				remaining = append(remaining, existing)
			}
		}
		cfg.BlockedCommands = remaining
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
		} else {
			if len(remaining) == 0 {
				fmt.Println("🔓 [MiniShare] All commands unblocked.")
			} else {
				fmt.Printf("🔓 [MiniShare] Blocked commands now: %s\n", strings.Join(remaining, ", "))
			}
		}

	case "dir", "folder":
		var remaining []string
		for _, existing := range cfg.BlockedFolders {
			keep := true
			for _, rm := range removeItems {
				absRm, err := filepath.Abs(rm)
				if err != nil {
					absRm = rm
				}
				if existing == absRm {
					keep = false
					break
				}
			}
			if keep {
				remaining = append(remaining, existing)
			}
		}
		cfg.BlockedFolders = remaining
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
		} else {
			if len(remaining) == 0 {
				fmt.Println("🔓 [MiniShare] All folder restrictions removed.")
			} else {
				fmt.Printf("🔓 [MiniShare] Blocked folders now: %s\n", strings.Join(remaining, ", "))
			}
		}

	default:
		fmt.Printf("Unknown unblock target '%s'. Use 'cmd' or 'dir/folder'.\n", target)
	}
}

// -------------------------------------------------------------------
// CONFIG & PATH COMMAND HANDLERS
// -------------------------------------------------------------------
func HandlePathCommand(args []string) {
	if len(args) == 0 {
		fmt.Printf("📄 Active Config Path: %s\n", GetConfigPath())
		return
	}

	targetPath := args[0]
	pointerFile := getPathPointerFile()

	if strings.ToLower(targetPath) == "reset" || strings.ToLower(targetPath) == "default" {
		HandleResetCommand([]string{"path"})
		return
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		absPath = targetPath
	}

	if err := os.WriteFile(pointerFile, []byte(absPath), 0644); err != nil {
		fmt.Printf("❌ Failed to set custom config path: %v\n", err)
		return
	}

	cfg := LoadConfig()
	_ = SaveConfig(cfg)
	fmt.Printf("📄 Custom config file path set to: %s\n", absPath)
}

func HandleConfigCommand(args []string) {
	if len(args) == 0 {
		cfg := LoadConfig()
		path := GetConfigPath()
		fmt.Println("⚡ MiniShare Active Configuration:")
		fmt.Printf("  📄 Config File Path: %s\n", path)
		fmt.Printf("  🌐 Signaling Server: %s\n", cfg.ServerURL)
		if cfg.PersistentUUID == "" {
			fmt.Println("  🔑 Persistent UUID: None (Generates fresh UUID per session)")
		} else if cfg.UUIDExpiresAt.IsZero() {
			fmt.Printf("  🔑 Persistent UUID: %s (Never expires)\n", cfg.PersistentUUID)
		} else if time.Now().After(cfg.UUIDExpiresAt) {
			fmt.Printf("  🔑 Persistent UUID: %s (Expired at %s)\n", cfg.PersistentUUID, cfg.UUIDExpiresAt.Format(time.RFC1123))
		} else {
			fmt.Printf("  🔑 Persistent UUID: %s (Expires: %s)\n", cfg.PersistentUUID, cfg.UUIDExpiresAt.Format(time.RFC1123))
		}
		if len(cfg.BlockedCommands) > 0 {
			fmt.Printf("  🔒 Blocked Commands: %s\n", strings.Join(cfg.BlockedCommands, ", "))
		} else {
			fmt.Println("  🔒 Blocked Commands: None")
		}
		if len(cfg.BlockedFolders) > 0 {
			fmt.Printf("  📁 Blocked Folders: %s\n", strings.Join(cfg.BlockedFolders, ", "))
		} else {
			fmt.Println("  📁 Blocked Folders: None")
		}
		return
	}

	subCmd := strings.ToLower(args[0])
	if subCmd == "reset" {
		HandleResetCommand(args[1:])
		return
	}

	HandlePathCommand(args)
}

func HandleSetCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage:")
		fmt.Println("  minishare set server <url>")
		fmt.Println("  minishare set uuid <uuid>")
		fmt.Println("  minishare set share <1h|2mo|never>")
		fmt.Println("  minishare set path <file-path>")
		return
	}

	setting := strings.ToLower(args[0])
	valArgs := args[1:]

	switch setting {
	case "server":
		HandleServerConfig(valArgs)
	case "uuid":
		HandleUUIDConfig(valArgs)
	case "share":
		HandleShareConfig(valArgs)
	case "path", "filepath":
		HandlePathCommand(valArgs)
	default:
		fmt.Printf("Unknown setting '%s'. Available: server, uuid, share, path\n", setting)
	}
}

func HandleServerConfig(args []string) {
	if len(args) == 0 || args[0] == "" {
		cfg := LoadConfig()
		fmt.Printf("[MiniShare] Current signaling server: %s\n", cfg.ServerURL)
		return
	}

	input := strings.TrimSpace(args[0])
	inputLower := strings.ToLower(input)

	if inputLower == "reset" || inputLower == "default" || inputLower == "null" || inputLower == "empty" {
		HandleResetCommand([]string{"server"})
		return
	}

	url := input
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	cfg := LoadConfig()
	cfg.ServerURL = url
	if err := SaveConfig(cfg); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}
	fmt.Printf("[MiniShare] Signaling server set to: %s\n", url)
}

func HandleUUIDConfig(args []string) {
	if len(args) == 0 {
		cfg := LoadConfig()
		if cfg.PersistentUUID == "" {
			fmt.Println("🔑 [MiniShare] No persistent UUID configured.")
		} else {
			fmt.Printf("🔑 [MiniShare] Active Persistent UUID: %s\n", cfg.PersistentUUID)
		}
		return
	}

	input := strings.TrimSpace(args[0])
	if strings.ToLower(input) == "reset" || strings.ToLower(input) == "clear" {
		HandleResetCommand([]string{"uuid"})
		return
	}

	cfg := LoadConfig()
	cfg.PersistentUUID = input
	cfg.UUIDExpiresAt = time.Time{}
	if err := SaveConfig(cfg); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}
	fmt.Printf("🔑 [MiniShare] Persistent UUID set to: %s (Never expires)\n", input)
}

func HandleShareConfig(args []string) {
	if len(args) == 0 {
		cfg := LoadConfig()
		if cfg.PersistentUUID == "" {
			fmt.Println("[MiniShare] No persistent UUID configured (generates fresh UUID per session).")
		} else if cfg.UUIDExpiresAt.IsZero() {
			fmt.Printf("[MiniShare] Fixed Persistent UUID: %s (Never expires)\n", cfg.PersistentUUID)
		} else if time.Now().After(cfg.UUIDExpiresAt) {
			fmt.Printf("[MiniShare] Persistent UUID %s HAS EXPIRED at %s.\n", cfg.PersistentUUID, cfg.UUIDExpiresAt.Format(time.RFC1123))
		} else {
			fmt.Printf("[MiniShare] Persistent UUID: %s (Expires: %s)\n", cfg.PersistentUUID, cfg.UUIDExpiresAt.Format(time.RFC1123))
		}
		return
	}

	var customUUID string
	var durationStr string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		argLower := strings.ToLower(arg)

		if (argLower == "uuid" || argLower == "set") && i+1 < len(args) {
			customUUID = args[i+1]
			i++
			continue
		}

		if argLower == "reset" || argLower == "clear" {
			HandleResetCommand([]string{"share"})
			return
		}

		if durationStr == "" {
			durationStr = arg
		}
	}

	cfg := LoadConfig()
	if customUUID != "" {
		cfg.PersistentUUID = customUUID
	} else if cfg.PersistentUUID == "" {
		cfg.PersistentUUID = generateUUID()
	}

	if durationStr != "" {
		expiry, neverExpires, err := parseDurationStr(durationStr)
		if err == nil {
			if neverExpires {
				cfg.UUIDExpiresAt = time.Time{}
			} else {
				cfg.UUIDExpiresAt = expiry
			}
		} else if customUUID == "" {
			cfg.PersistentUUID = durationStr
			cfg.UUIDExpiresAt = time.Time{}
		}
	}

	_ = SaveConfig(cfg)

	if cfg.UUIDExpiresAt.IsZero() {
		fmt.Printf("🔑 [MiniShare] Persistent UUID configured: %s (Never expires)\n", cfg.PersistentUUID)
	} else {
		fmt.Printf("🔑 [MiniShare] Persistent UUID configured: %s (Valid until %s)\n", cfg.PersistentUUID, cfg.UUIDExpiresAt.Format(time.RFC1123))
	}
}

// -------------------------------------------------------------------
// DAEMON MANAGEMENT (-d, --daemon, kill -d, daemon status)
// -------------------------------------------------------------------
func launchDaemonProcess() {
	pidPath := getDaemonPIDPath()
	uuidPath := getDaemonUUIDPath()
	logPath := getDaemonLogPath()
	cfg := LoadConfig()

	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			if processExists(pid) {
				daemonUUID := ""
				if uData, err := os.ReadFile(uuidPath); err == nil {
					daemonUUID = strings.TrimSpace(string(uData))
				}
				printDaemonStatusInfo(pid, daemonUUID, cfg.ServerURL, logPath, true)
				return
			}
		}
	}

	sessionUUID := cfg.PersistentUUID
	if sessionUUID == "" || (!cfg.UUIDExpiresAt.IsZero() && time.Now().After(cfg.UUIDExpiresAt)) {
		sessionUUID = generateUUID()
	}

	_ = os.WriteFile(uuidPath, []byte(sessionUUID), 0644)

	// Build args for the child process: strip -d/--daemon flags
	args := []string{}
	for _, arg := range os.Args[1:] {
		if arg != "-d" && arg != "--daemon" {
			args = append(args, arg)
		}
	}

	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "MINISHARE_DAEMON=1")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open daemon log file: %v", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start background daemon: %v", err)
	}

	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

	fmt.Println("⚡ MiniShare Host launched in background daemon mode")
	printDaemonStatusInfo(cmd.Process.Pid, sessionUUID, cfg.ServerURL, logPath, false)
}

func printDaemonStatusInfo(pid int, uuid string, serverURL string, logPath string, alreadyRunning bool) {
	serverURL = strings.TrimSuffix(serverURL, "/")
	if alreadyRunning {
		fmt.Printf("⚠️ MiniShare background daemon is already running (PID: %d)\n", pid)
	}
	fmt.Printf("🆔 Daemon PID:  %d\n", pid)
	if uuid != "" {
		fmt.Printf("🔑 Session UUID: %s\n", uuid)
		fmt.Printf("💻 CLI Connect: minishare connect %s\n", uuid)
		fmt.Printf("🌐 Web Connect: %s/app/%s\n", serverURL, uuid)
	}
	fmt.Printf("📄 Log File:    %s\n", logPath)
	fmt.Println("🛑 Stop Daemon: minishare kill -d")
}

func showDaemonStatus() {
	pidPath := getDaemonPIDPath()
	uuidPath := getDaemonUUIDPath()
	logPath := getDaemonLogPath()
	cfg := LoadConfig()

	if data, err := os.ReadFile(pidPath); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && processExists(pid) {
			daemonUUID := ""
			if uData, err := os.ReadFile(uuidPath); err == nil {
				daemonUUID = strings.TrimSpace(string(uData))
			}
			fmt.Println("⚡ MiniShare Background Daemon: RUNNING")
			printDaemonStatusInfo(pid, daemonUUID, cfg.ServerURL, logPath, false)
			return
		}
	}
	fmt.Println("🔴 MiniShare background daemon is NOT running.")
}

func stopDaemonProcess() {
	pidPath := getDaemonPIDPath()
	uuidPath := getDaemonUUIDPath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Println("⚠️ No active MiniShare background daemon found.")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidPath)
		_ = os.Remove(uuidPath)
		fmt.Println("⚠️ Invalid daemon PID file removed.")
		return
	}

	if !processExists(pid) {
		_ = os.Remove(pidPath)
		_ = os.Remove(uuidPath)
		fmt.Printf("⚠️ Process PID %d is not running. PID file cleaned up.\n", pid)
		return
	}

	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Signal(syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		if processExists(pid) {
			_ = proc.Kill()
		}
	}

	_ = os.Remove(pidPath)
	_ = os.Remove(uuidPath)
	fmt.Printf("🛑 MiniShare background daemon stopped (PID: %d).\n", pid)
}

func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// -------------------------------------------------------------------
// VERSION & SELF-UPDATE COMMANDS
// -------------------------------------------------------------------
func HandleVersionCommand() {
	fmt.Printf("MiniShare CLI version %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
}

func isNewerVersion(current, latest string) bool {
	cleanCurr := strings.TrimPrefix(strings.TrimSpace(current), "v")
	cleanLat := strings.TrimPrefix(strings.TrimSpace(latest), "v")

	if cleanCurr == cleanLat {
		return false
	}

	currParts := strings.Split(cleanCurr, ".")
	latParts := strings.Split(cleanLat, ".")

	for i := 0; i < len(currParts) && i < len(latParts); i++ {
		cVal, err1 := strconv.Atoi(currParts[i])
		lVal, err2 := strconv.Atoi(latParts[i])
		if err1 == nil && err2 == nil {
			if lVal > cVal {
				return true
			}
			if lVal < cVal {
				return false
			}
		} else {
			if latParts[i] > currParts[i] {
				return true
			}
			if latParts[i] < currParts[i] {
				return false
			}
		}
	}
	return len(latParts) > len(currParts)
}

func getReleaseAssetName(tag string) (string, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", fmt.Errorf("empty version tag")
	}

	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return fmt.Sprintf("minishare-mac-silicon-%s.zip", tag), nil
		case "amd64":
			return fmt.Sprintf("minishare-mac-intel-%s.zip", tag), nil
		default:
			return "", fmt.Errorf("unsupported macOS architecture: %s", runtime.GOARCH)
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return fmt.Sprintf("minishare-linux-%s.zip", tag), nil
		}
		return "", fmt.Errorf("unsupported Linux architecture: %s", runtime.GOARCH)
	case "windows":
		if runtime.GOARCH == "amd64" {
			return fmt.Sprintf("minishare-windows-%s.zip", tag), nil
		}
		return "", fmt.Errorf("unsupported Windows architecture: %s", runtime.GOARCH)
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func HandleUpdateCommand() {
	fmt.Println("🔍 Checking for MiniShare CLI updates on GitHub...")

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/divamtech/MiniShare/releases/latest", nil)
	if err != nil {
		fmt.Printf("❌ Failed to create update check request: %v\n", err)
		return
	}
	req.Header.Set("User-Agent", "MiniShare-CLI")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Failed to connect to GitHub API: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ GitHub API returned status code %d\n", resp.StatusCode)
		return
	}

	var releaseInfo struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil || releaseInfo.TagName == "" {
		fmt.Println("❌ Failed to parse release version from GitHub.")
		return
	}

	latestTag := strings.TrimSpace(releaseInfo.TagName)
	if !isNewerVersion(Version, latestTag) {
		fmt.Printf("✨ MiniShare CLI is already up to date (version %s).\n", Version)
		return
	}

	assetName, err := getReleaseAssetName(latestTag)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}

	fmt.Printf("\n⚡ New update available: \033[1;32m%s\033[0m (Current: %s)\n", latestTag, Version)
	fmt.Print("Do you want to install it now? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))

	if answer != "y" && answer != "yes" {
		fmt.Println("Update canceled.")
		return
	}

	downloadURL := fmt.Sprintf("https://github.com/divamtech/MiniShare/releases/download/%s/%s", latestTag, assetName)
	fmt.Printf("📦 Downloading %s...\n", downloadURL)

	dlReq, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to create download request: %v\n", err)
		return
	}
	dlReq.Header.Set("User-Agent", "MiniShare-CLI")

	dlResp, err := client.Do(dlReq)
	if err != nil {
		fmt.Printf("❌ Failed to download release package: %v\n", err)
		return
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Download failed with HTTP status %d\n", dlResp.StatusCode)
		return
	}

	zipBytes, err := io.ReadAll(dlResp.Body)
	if err != nil {
		fmt.Printf("❌ Failed to read download package: %v\n", err)
		return
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		fmt.Printf("❌ Failed to parse ZIP package: %v\n", err)
		return
	}

	execName := "minishare"
	if runtime.GOOS == "windows" {
		execName = "minishare.exe"
	}

	var binaryBytes []byte
	for _, file := range zipReader.File {
		cleanFileName := filepath.Base(file.Name)
		if cleanFileName == execName {
			f, err := file.Open()
			if err != nil {
				fmt.Printf("❌ Failed to open binary inside ZIP: %v\n", err)
				return
			}
			binaryBytes, err = io.ReadAll(f)
			f.Close()
			if err != nil {
				fmt.Printf("❌ Failed to extract binary from ZIP: %v\n", err)
				return
			}
			break
		}
	}

	if len(binaryBytes) == 0 {
		fmt.Printf("❌ Binary '%s' not found inside release asset ZIP.\n", execName)
		return
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Printf("❌ Failed to locate active executable path: %v\n", err)
		return
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		fmt.Printf("❌ Failed to resolve symlinks for executable: %v\n", err)
		return
	}

	dir := filepath.Dir(execPath)

	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(execPath, oldPath); err != nil {
			fmt.Printf("❌ Failed to move old executable on Windows: %v\n", err)
			return
		}
		if err := os.WriteFile(execPath, binaryBytes, 0755); err != nil {
			_ = os.Rename(oldPath, execPath)
			fmt.Printf("❌ Failed to write new binary: %v\n", err)
			return
		}
		_ = os.Remove(oldPath)
	} else {
		tmpFile, err := os.CreateTemp(dir, "minishare-update-*.tmp")
		if err != nil {
			fmt.Printf("❌ Failed to create temporary file in %s: %v\n(You may need to run with sudo: sudo minishare update)\n", dir, err)
			return
		}
		tmpPath := tmpFile.Name()

		if _, err := tmpFile.Write(binaryBytes); err != nil {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			fmt.Printf("❌ Failed to write update binary: %v\n", err)
			return
		}
		tmpFile.Close()

		if err := os.Chmod(tmpPath, 0755); err != nil {
			_ = os.Remove(tmpPath)
			fmt.Printf("❌ Failed to set executable permissions: %v\n", err)
			return
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			_ = os.Remove(tmpPath)
			fmt.Printf("❌ Failed to replace executable at %s: %v\n(Try running: sudo minishare update)\n", execPath, err)
			return
		}
	}

	fmt.Printf("\n✔ \033[1;32mMiniShare CLI successfully updated to %s!\033[0m\n", latestTag)
}

// -------------------------------------------------------------------
// MAIN CLI ENTRYPOINT
// -------------------------------------------------------------------
func main() {
	if len(os.Args) > 1 {
		cmd1 := strings.ToLower(os.Args[1])

		// Version & Self-Update commands
		if cmd1 == "version" || cmd1 == "-v" || cmd1 == "--version" || cmd1 == "v" {
			HandleVersionCommand()
			return
		}

		if cmd1 == "update" {
			HandleUpdateCommand()
			return
		}

		// Reset commands: minishare reset [default|all|server|uuid|share|path]
		if cmd1 == "reset" {
			HandleResetCommand(os.Args[2:])
			return
		}

		// Stop daemon command: minishare kill -d or minishare daemon stop
		if (cmd1 == "kill" && len(os.Args) > 2 && os.Args[2] == "-d") ||
			(cmd1 == "daemon" && len(os.Args) > 2 && (os.Args[2] == "stop" || os.Args[2] == "kill")) {
			stopDaemonProcess()
			return
		}

		// Daemon status command: minishare daemon status
		if cmd1 == "daemon" && len(os.Args) > 2 && os.Args[2] == "status" {
			showDaemonStatus()
			return
		}

		// Pure configuration setting commands (DO NOT launch host session)
		if cmd1 == "config" {
			HandleConfigCommand(os.Args[2:])
			return
		}

		if cmd1 == "set" {
			HandleSetCommand(os.Args[2:])
			return
		}

		if cmd1 == "server" {
			HandleServerConfig(os.Args[2:])
			return
		}

		if cmd1 == "share" {
			HandleShareConfig(os.Args[2:])
			return
		}

		if cmd1 == "uuid" {
			HandleUUIDConfig(os.Args[2:])
			return
		}

		// Security block/unblock commands
		if cmd1 == "block" {
			HandleBlockCommand(os.Args[2:])
			return
		}

		if cmd1 == "unblock" {
			HandleUnblockCommand(os.Args[2:])
			return
		}

		// Viewer connection command
		if cmd1 == "connect" || cmd1 == "-c" || cmd1 == "c" {
			if len(os.Args) < 3 {
				fmt.Println("Usage: minishare connect|-c|c <session-uuid>")
				os.Exit(1)
			}
			runViewer(os.Args[2])
			return
		}

		if cmd1 == "--help" || cmd1 == "-h" || cmd1 == "help" {
			printHelp()
			return
		}
	}

	// Daemon mode flag for starting host session in background: minishare -d
	for _, arg := range os.Args[1:] {
		if arg == "-d" || arg == "--daemon" {
			launchDaemonProcess()
			return
		}
	}

	// Default action: Start Host session live
	runHost()
}

func printHelp() {
	helpText := `[TITLE]MiniShare CLI [YELLOW]⚡[TITLE] - Real-time P2P Terminal Sharing[RESET]

[HEADER]Usage:[RESET]
  [CMD]minishare[RESET]                            [DESC]Start Host session (fresh UUID by default)[RESET]
  [CMD]minishare -d[RESET]                         [DESC]Start Host session in background daemon mode[RESET]
  [CMD]minishare daemon status[RESET]              [DESC]Check background daemon status and UUID[RESET]
  [CMD]minishare kill -d[RESET]                    [DESC]Stop running background daemon process[RESET]
  [CMD]minishare connect|-c|c[RESET] [PARAM]<session-uuid>[RESET] [DESC]Connect to a remote Host session[RESET]

[HEADER]Management & System:[RESET]
  [CMD]minishare version[RESET]                    [DESC]Print current MiniShare CLI version (alias: -v, --version)[RESET]
  [CMD]minishare update[RESET]                     [DESC]Check GitHub and self-update CLI binary to latest release[RESET]
  [CMD]minishare config[RESET]                     [DESC]View active settings & config file location[RESET]

[HEADER]Set Options:[RESET]
  [CMD]minishare set server[RESET] [PARAM]<url>[RESET]           [DESC]Set signaling server URL[RESET]
  [CMD]minishare set uuid[RESET] [PARAM]<uuid>[RESET]            [DESC]Set fixed persistent UUID[RESET]
  [CMD]minishare set share[RESET] [PARAM]<1h|2mo|never>[RESET]   [DESC]Set UUID duration (1h, 30m, 2d, 2mo, 4y, never)[RESET]
  [CMD]minishare set path[RESET] [PARAM]<file-path>[RESET]       [DESC]Set custom config file path[RESET]

[HEADER]Reset Commands:[RESET]
  [CMD]minishare reset[RESET]                      [DESC]Reset all settings to default (alias: reset default, reset all)[RESET]
  [CMD]minishare reset server[RESET]               [DESC]Reset signaling server URL to default[RESET]
  [CMD]minishare reset uuid[RESET]                 [DESC]Reset persistent UUID to default[RESET]
  [CMD]minishare reset share[RESET]                [DESC]Reset UUID duration / expiration setting[RESET]
  [CMD]minishare reset path[RESET]                 [DESC]Reset config file path to default OS location[RESET]
  [CMD]minishare reset block[RESET]                [DESC]Clear all blocked commands and folders[RESET]

[HEADER]Security (Block / Unblock):[RESET]
  [CMD]minishare block cmd[RESET] [PARAM]<cmds...>[RESET]        [DESC]Block commands (comma or space separated)[RESET]
  [CMD]minishare block dir|folder[RESET] [PARAM]<paths...>[RESET] [DESC]Block folder access (comma or space separated)[RESET]
  [CMD]minishare unblock cmd[RESET] [PARAM]<cmds...>[RESET]      [DESC]Unblock specific commands[RESET]
  [CMD]minishare unblock dir|folder[RESET] [PARAM]<paths...>[RESET] [DESC]Unblock specific folder restrictions[RESET]`

	replacer := strings.NewReplacer(
		"[TITLE]", "\033[1;36m",
		"[YELLOW]", "\033[1;33m",
		"[HEADER]", "\033[1;35m",
		"[CMD]", "\033[1;32m",
		"[PARAM]", "\033[36m",
		"[DESC]", "\033[90m",
		"[RESET]", "\033[0m",
	)
	fmt.Println(replacer.Replace(helpText))
}

// -------------------------------------------------------------------
// HOST MODE
// -------------------------------------------------------------------
func runHost() {
	cfg := LoadConfig()
	isDaemon := os.Getenv("MINISHARE_DAEMON") == "1"

	if !isDaemon {
		fmt.Printf("\033[90m[MiniShare] Connecting to signaling server: %s\033[0m\n", cfg.ServerURL)
	}

	manualFlag := flag.Bool("manual", false, "Run in manual copy-paste mode")
	flag.CommandLine.Parse(os.Args[1:])

	var sessionUUID string
	if cfg.PersistentUUID != "" {
		if cfg.UUIDExpiresAt.IsZero() || time.Now().Before(cfg.UUIDExpiresAt) {
			sessionUUID = cfg.PersistentUUID
		}
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Fatalf("failed to start pty: %v", err)
	}
	defer ptmx.Close()

	// Single PTY output reader goroutine for the lifetime of ptmx
	ptyOutputChan := make(chan []byte, 200)
	shellExited := make(chan struct{})

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				tmp := make([]byte, n)
				copy(tmp, buf[:n])
				ptyOutputChan <- tmp
			}
			if err != nil {
				close(shellExited)
				break
			}
		}
	}()

	// Sync initial pty size from the host terminal (non-daemon only)
	if !isDaemon {
		if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
			_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
		}
	}

	// Put host terminal into raw mode so local I/O works correctly (non-daemon)
	if !isDaemon {
		if oldState, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
			defer term.Restore(int(os.Stdin.Fd()), oldState)
		}

		// Pipe host's local stdin to pty (non-daemon only)
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					_, _ = ptmx.Write(buf[:n])
				}
				if err != nil {
					break
				}
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	processTerminated := false

	go func() {
		<-sigChan
		processTerminated = true
		if !isDaemon {
			fmt.Print("\r\n[MiniShare] Host shutting down...\r\n")
		} else {
			log.Println("[MiniShare] Host shutting down...")
		}
	}()

	// Buffer to keep recent terminal output while waiting for viewer
	var scrollbackMu sync.Mutex
	scrollback := make([]byte, 0, 16384)

	// Background worker to collect PTY output into scrollback buffer and current data channel
	var currentDC atomic.Value // holds *webrtc.DataChannel

	go func() {
		for {
			select {
			case <-shellExited:
				return
			case data, ok := <-ptyOutputChan:
				if !ok {
					return
				}
				if !isDaemon {
					_, _ = os.Stdout.Write(data)
				}
				scrollbackMu.Lock()
				if len(scrollback)+len(data) > 16384 {
					overflow := (len(scrollback) + len(data)) - 16384
					if overflow < len(scrollback) {
						scrollback = append(scrollback[overflow:], data...)
					} else {
						scrollback = append([]byte(nil), data...)
					}
				} else {
					scrollback = append(scrollback, data...)
				}
				scrollbackMu.Unlock()

				if dcVal := currentDC.Load(); dcVal != nil {
					if dc, ok := dcVal.(*webrtc.DataChannel); ok && dc != nil {
						_ = dc.Send(data)
					}
				}
			}
		}
	}()

	for {
		if processTerminated {
			break
		}

		select {
		case <-shellExited:
			if !isDaemon {
				fmt.Print("\r\n[MiniShare] Shell process exited.\r\n")
			} else {
				log.Println("[MiniShare] Shell process exited.")
			}
			return
		default:
		}

		pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
		})
		if err != nil {
			log.Fatalf("failed to create peer connection: %v", err)
		}

		done := make(chan struct{})
		var doneOnce sync.Once
		closeDone := func() { doneOnce.Do(func() { close(done) }) }

		dc, err := pc.CreateDataChannel("terminal", nil)
		if err != nil {
			pc.Close()
			log.Fatalf("failed to create data channel: %v", err)
		}

		// Handle SIGWINCH for terminal resize (non-daemon only)
		var winchChan chan os.Signal
		if !isDaemon {
			winchChan = make(chan os.Signal, 1)
			setupWinchSignal(winchChan)
			go func() {
				for {
					select {
					case <-winchChan:
						if cols, rows, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
							_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
						}
					case <-done:
						return
					}
				}
			}()
		}

		dc.OnOpen(func() {
			if !isDaemon {
				fmt.Print("\r\n\033[1;32m✓ Peer connected successfully (Web Browser or CLI client)\033[0m\r\n")
				fmt.Print("\033[1;32mSession active — terminal streaming peer-to-peer...\033[0m\r\n\r\n")
			}
			log.Println("Data channel open — P2P session live")

			scrollbackMu.Lock()
			if len(scrollback) > 0 {
				_ = dc.Send(scrollback)
			}
			scrollbackMu.Unlock()

			hostname, _ := os.Hostname()
			banner := fmt.Sprintf("\r\n\033[1;32m┌─────────────────────────────────────────────────────────────┐\033[0m\r\n"+
				"\033[1;32m│  ⚡ CONNECTED TO REMOTE HOST: %-29s │\033[0m\r\n"+
				"\033[1;32m│  OS: %-9s | Shell: %-25s │\033[0m\r\n"+
				"\033[1;32m│  Exit: Type 'exit' or press 'Ctrl+]' to detach             │\033[0m\r\n"+
				"\033[1;32m└─────────────────────────────────────────────────────────────┘\033[0m\r\n\r\n",
				hostname, runtime.GOOS, shell)
			_ = dc.Send([]byte(banner))

			currentDC.Store(dc)
		})

		var cmdBuffer bytes.Buffer
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			// Check for resize control message (prefixed with SOH byte \x01)
			if len(msg.Data) > 1 && msg.Data[0] == 0x01 {
				var resizeMsg struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if err := json.Unmarshal(msg.Data[1:], &resizeMsg); err == nil && resizeMsg.Type == "resize" {
					if resizeMsg.Cols > 0 && resizeMsg.Rows > 0 {
						_ = pty.Setsize(ptmx, &pty.Winsize{
							Rows: uint16(resizeMsg.Rows),
							Cols: uint16(resizeMsg.Cols),
						})
						log.Printf("[Resize] Terminal resized to %dx%d", resizeMsg.Cols, resizeMsg.Rows)
					}
				}
				return
			}

			// Check if we need to enforce security rules
			hasBlockedCmds := len(cfg.BlockedCommands) > 0
			hasBlockedDirs := len(cfg.BlockedFolders) > 0

			// If no security rules, fast path — forward everything
			if !hasBlockedCmds && !hasBlockedDirs {
				for _, b := range msg.Data {
					if b == '\r' || b == '\n' {
						if typedCmd := strings.TrimSpace(cmdBuffer.String()); typedCmd != "" {
							log.Printf("⌨️  [Viewer Command Executed]: %s", typedCmd)
						}
						cmdBuffer.Reset()
					} else if b == 127 || b == 8 {
						if cmdBuffer.Len() > 0 {
							cmdBuffer.Truncate(cmdBuffer.Len() - 1)
						}
					} else if b >= 32 && b <= 126 {
						cmdBuffer.WriteByte(b)
					}
				}
				_, _ = ptmx.Write(msg.Data)
				return
			}

			// Security-enforced path: process byte-by-byte
			for _, b := range msg.Data {
				if b == '\r' || b == '\n' {
					typedCmd := strings.TrimSpace(cmdBuffer.String())
					if typedCmd != "" {
						log.Printf("⌨️  [Viewer Command]: %s", typedCmd)

						// --- Check blocked commands ---
						if hasBlockedCmds {
							cmdParts := strings.Fields(typedCmd)
							blocked := false
							for _, part := range cmdParts {
								cleanPart := strings.ToLower(strings.TrimSpace(part))
								if cleanPart == "|" || cleanPart == "&&" || cleanPart == "||" || cleanPart == ";" {
									continue
								}
								for _, blockedCmd := range cfg.BlockedCommands {
									if cleanPart == blockedCmd {
										blocked = true
										break
									}
								}
								if blocked {
									break
								}
							}

							if blocked {
								log.Printf("🚫 [Security] BLOCKED command: %s", typedCmd)
								rejectMsg := fmt.Sprintf("\r\n\033[1;31m🚫 [MiniShare Security] Command '%s' is BLOCKED by host.\033[0m\r\n", typedCmd)
								_ = dc.Send([]byte(rejectMsg))
								cmdBuffer.Reset()
								_ = dc.Send([]byte("\033[2K\r"))
								_, _ = ptmx.Write([]byte("\n"))
								continue
							}
						}

						// --- Check blocked folders (cd command) ---
						if hasBlockedDirs {
							cmdParts := strings.Fields(typedCmd)
							if len(cmdParts) >= 2 && cmdParts[0] == "cd" {
								targetDir := cmdParts[1]
								if !filepath.IsAbs(targetDir) {
									if home, err := os.UserHomeDir(); err == nil {
										targetDir = filepath.Join(home, targetDir)
									}
								}
								targetDir = filepath.Clean(targetDir)

								dirBlocked := false
								for _, blockedDir := range cfg.BlockedFolders {
									blockedDir = filepath.Clean(blockedDir)
									if targetDir == blockedDir || strings.HasPrefix(targetDir, blockedDir+string(filepath.Separator)) {
										dirBlocked = true
										break
									}
								}

								if dirBlocked {
									log.Printf("🚫 [Security] BLOCKED folder access: %s", cmdParts[1])
									rejectMsg := fmt.Sprintf("\r\n\033[1;31m🚫 [MiniShare Security] Access to '%s' is BLOCKED by host.\033[0m\r\n", cmdParts[1])
									_ = dc.Send([]byte(rejectMsg))
									cmdBuffer.Reset()
									_ = dc.Send([]byte("\033[2K\r"))
									_, _ = ptmx.Write([]byte("\n"))
									continue
								}
							}
						}
					}
					cmdBuffer.Reset()
					_, _ = ptmx.Write([]byte{b})
				} else if b == 127 || b == 8 {
					if cmdBuffer.Len() > 0 {
						cmdBuffer.Truncate(cmdBuffer.Len() - 1)
					}
					_, _ = ptmx.Write([]byte{b})
				} else {
					if b >= 32 && b <= 126 {
						cmdBuffer.WriteByte(b)
					}
					_, _ = ptmx.Write([]byte{b})
				}
			}
		})

		dc.OnClose(func() {
			log.Println("Data channel closed")
			closeDone()
		})

		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
				closeDone()
			}
		})

		offer, err := pc.CreateOffer(nil)
		if err != nil {
			log.Fatalf("failed to create offer: %v", err)
		}
		gatherComplete := webrtc.GatheringCompletePromise(pc)
		if err := pc.SetLocalDescription(offer); err != nil {
			log.Fatalf("failed to set local description: %v", err)
		}
		<-gatherComplete

		offerCode := encodePayload(pc.LocalDescription())

		if *manualFlag {
			runManualHost(pc, offerCode, done)
			return
		}

		serverURL := strings.TrimSuffix(cfg.ServerURL, "/")
		payload := map[string]string{
			"offer": offerCode,
		}
		if sessionUUID != "" {
			payload["uuid"] = sessionUUID
		}

		resp, err := postJSON(serverURL+"/api/session", payload)
		if err != nil {
			fmt.Printf("⚠️ Signaling server unavailable (%v). Falling back to manual mode...\n", err)
			runManualHost(pc, offerCode, done)
			return
		}

		var sessResp struct {
			UUID string `json:"uuid"`
		}
		_ = json.Unmarshal(resp, &sessResp)

		if sessResp.UUID != "" {
			sessionUUID = sessResp.UUID
		} else {
			fmt.Println("⚠️ Failed to obtain UUID from signaling server. Falling back to manual mode...")
			runManualHost(pc, offerCode, done)
			return
		}

		webLink := fmt.Sprintf("%s/app/%s", serverURL, sessResp.UUID)
		if !isDaemon {
			fmt.Print("\r\n\033[1;32m⚡ MiniShare Host Session Live\033[0m\r\n")
			fmt.Printf("🔑 \033[1;37mSession UUID:\033[0m \033[1;36m%s\033[0m\r\n", sessResp.UUID)
			fmt.Printf("💻 \033[1;37mConnect via CLI:\033[0m \033[1;33mminishare connect %s\033[0m\r\n", sessResp.UUID)
			fmt.Printf("🌐 \033[1;37mConnect via Web Browser:\033[0m \033[4;36m%s\033[0m\r\n", webLink)
			copyToClipboard(sessResp.UUID)
			fmt.Print("\033[1;32m👉 Session UUID copied to clipboard automatically!\033[0m\r\n")
			fmt.Print("\r\n\033[90mWaiting for peer to connect...\033[0m\r\n")
		} else {
			log.Printf("⚡ MiniShare Host Session Live — UUID: %s", sessResp.UUID)
			log.Printf("🌐 Web: %s", webLink)
		}

		go func() {
			for {
				time.Sleep(1 * time.Second)
				select {
				case <-done:
					return
				default:
				}

				ansResp, err := getJSON(fmt.Sprintf("%s/api/session/%s/answer", serverURL, sessResp.UUID))
				if err == nil {
					var ansData struct {
						Answer string `json:"answer"`
					}
					_ = json.Unmarshal(ansResp, &ansData)
					if ansData.Answer != "" {
						var answer webrtc.SessionDescription
						decodePayload(ansData.Answer, &answer)
						if setErr := pc.SetRemoteDescription(answer); setErr != nil {
							log.Printf("error setting remote answer: %v", setErr)
						} else {
							log.Println("Remote answer received — establishing P2P link...")
						}
						break
					}
				}
			}
		}()

		select {
		case <-done:
		case <-shellExited:
			closeDone()
		}

		currentDC.Store((*webrtc.DataChannel)(nil))
		_ = dc.Close()
		_ = pc.Close()

		if !isDaemon || processTerminated {
			if !isDaemon {
				fmt.Print("\r\n[MiniShare] Session ended\r\n")
			} else {
				log.Println("Session ended")
			}
			break
		}

		select {
		case <-shellExited:
			log.Println("Shell exited — stopping host daemon")
			return
		default:
			log.Println("[MiniShare] Viewer disconnected. Host daemon active — waiting for reconnection...")
			time.Sleep(500 * time.Millisecond)
		}
	}
	time.Sleep(100 * time.Millisecond)
}

func runManualHost(pc *webrtc.PeerConnection, offerCode string, done chan struct{}) {
	fmt.Println("\n\033[1;35m=== Share this code with the viewer ===\033[0m")
	fmt.Println("\033[1;36m" + offerCode + "\033[0m")
	fmt.Println("\033[1;35m=== end code ===\033[0m")
	copyToClipboard(offerCode)
	fmt.Println("\033[1;32m👉 Code copied to system clipboard automatically!\033[0m")

	fmt.Print("\n\033[1;37mPaste the code from the viewer (or press Enter to use clipboard):\033[0m\n> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = cleanInput(line)
	if line == "" {
		line = cleanInput(readFromClipboard())
	}

	var answer webrtc.SessionDescription
	decodePayload(line, &answer)
	_ = pc.SetRemoteDescription(answer)

	log.Println("waiting for viewer to connect...")
	<-done
	log.Println("session ended")
}

// -------------------------------------------------------------------
// VIEWER MODE
// -------------------------------------------------------------------
func runViewer(uuid string) {
	cfg := LoadConfig()

	fmt.Printf("\033[90m[MiniShare] Connecting to signaling server: %s\033[0m\n", cfg.ServerURL)

	serverURL := strings.TrimSuffix(cfg.ServerURL, "/")
	resp, err := getJSON(fmt.Sprintf("%s/api/session/%s/offer", serverURL, uuid))
	if err != nil {
		log.Fatalf("failed to fetch session %s from signaling server: %v", uuid, err)
	}

	var offerData struct {
		Offer string `json:"offer"`
	}
	if err := json.Unmarshal(resp, &offerData); err != nil || offerData.Offer == "" {
		log.Fatalf("invalid session offer data from server")
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		log.Fatalf("failed to create peer connection: %v", err)
	}
	defer pc.Close()

	var dataChan *webrtc.DataChannel
	connected := make(chan struct{})
	done := make(chan struct{})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\n[MiniShare] Viewer shutting down...")
		if dataChan != nil {
			_ = dataChan.Close()
		}
		_ = pc.Close()
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dataChan = dc
		dc.OnOpen(func() {
			select {
			case <-connected:
			default:
				close(connected)
			}
		})
		dc.OnClose(func() {
			select {
			case <-done:
			default:
				close(done)
			}
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			os.Stdout.Write(msg.Data)
		})
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateDisconnected || state == webrtc.ICEConnectionStateFailed || state == webrtc.ICEConnectionStateClosed {
			select {
			case <-done:
			default:
				close(done)
			}
		}
	})

	var offer webrtc.SessionDescription
	decodePayload(offerData.Offer, &offer)
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Fatalf("failed to set remote description: %v", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Fatalf("failed to create answer: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Fatalf("failed to set local description: %v", err)
	}
	<-gatherComplete

	encodedAnswer := encodePayload(pc.LocalDescription())
	_, err = postJSON(fmt.Sprintf("%s/api/session/%s/answer", serverURL, uuid), map[string]string{"answer": encodedAnswer})
	if err != nil {
		log.Fatalf("failed to post answer to signaling server: %v", err)
	}

	fmt.Println("\033[90mConnecting to host P2P...\033[0m")
	select {
	case <-connected:
	case <-done:
		log.Fatal("failed to connect to host")
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err == nil {
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 && dataChan != nil {
				for i := 0; i < n; i++ {
					if buf[i] == 0x1d { // Ctrl+]
						fmt.Print("\r\n\033[1;33m[MiniShare] Detached via Ctrl+].\033[0m\r\n")
						select {
						case <-done:
						default:
							close(done)
						}
						return
					}
				}
				_ = dataChan.Send(buf[:n])
			}
			if err != nil {
				break
			}
		}
		select {
		case <-done:
		default:
			close(done)
		}
	}()

	<-done
	fmt.Println("\r\n\033[1;31m[MiniShare] Disconnected from remote session. Returned to local terminal.\033[0m")
}

// -------------------------------------------------------------------
// UTILITY & ENCODING FUNCTIONS
// -------------------------------------------------------------------
func encodePayload(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Fatalf("marshal error: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func decodePayload(s string, v interface{}) {
	s = cleanInput(s)
	if s == "" {
		log.Fatalf("no input code provided")
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			log.Fatalf("invalid code encoding: %v", err)
		}
	}

	r := flate.NewReader(bytes.NewReader(b))
	decompressed, err := io.ReadAll(r)
	r.Close()
	if err == nil && len(decompressed) > 0 {
		b = decompressed
	}

	if err := json.Unmarshal(b, v); err != nil {
		log.Fatalf("invalid code contents: %v", err)
	}
}

func cleanInput(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "'", "")
	return s
}

func postJSON(url string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func getJSON(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func copyToClipboard(text string) bool {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("pbcopy")
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("xclip", "-selection", "clipboard")
	} else if runtime.GOOS == "windows" {
		cmd = exec.Command("clip")
	} else {
		return false
	}
	cmd.Stdin = strings.NewReader(strings.TrimSpace(text))
	return cmd.Run() == nil
}

func readFromClipboard() string {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("pbpaste")
	} else if runtime.GOOS == "linux" {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
	} else {
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
