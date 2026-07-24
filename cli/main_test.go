package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// generateUUID
// ---------------------------------------------------------------------------
func TestGenerateUUID_Format(t *testing.T) {
	uuid := generateUUID()
	// UUIDs from this generator look like "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
	matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, uuid)
	if !matched {
		t.Errorf("generateUUID() = %q, does not match expected UUID v4 format", uuid)
	}
}

func TestGenerateUUID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		u := generateUUID()
		if seen[u] {
			t.Fatalf("generateUUID() produced duplicate: %s", u)
		}
		seen[u] = true
	}
}

// ---------------------------------------------------------------------------
// parseDurationStr
// ---------------------------------------------------------------------------
func TestParseDurationStr(t *testing.T) {
	tests := []struct {
		input       string
		wantNever   bool
		wantErr     bool
		minDuration time.Duration // approximate minimum from now
	}{
		{"never", true, false, 0},
		{"permanent", true, false, 0},
		{"0", true, false, 0},
		{"infinite", true, false, 0},
		{"1h", false, false, 59 * time.Minute},
		{"30m", false, false, 29 * time.Minute},
		{"2d", false, false, 47 * time.Hour},
		{"1y", false, false, 364 * 24 * time.Hour},
		{"2mo", false, false, 58 * 24 * time.Hour},
		{"garbage", false, true, 0},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			now := time.Now()
			expiry, neverExpires, err := parseDurationStr(tc.input)

			if tc.wantErr {
				if err == nil {
					t.Errorf("parseDurationStr(%q) expected error, got nil", tc.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseDurationStr(%q) unexpected error: %v", tc.input, err)
			}

			if neverExpires != tc.wantNever {
				t.Errorf("parseDurationStr(%q) neverExpires = %v, want %v", tc.input, neverExpires, tc.wantNever)
			}

			if !tc.wantNever {
				diff := expiry.Sub(now)
				if diff < tc.minDuration {
					t.Errorf("parseDurationStr(%q) expiry too soon: %v < %v from now", tc.input, diff, tc.minDuration)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cleanInput
// ---------------------------------------------------------------------------
func TestCleanInput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  hello  ", "hello"},
		{"hello\nworld", "helloworld"},
		{"hello\r\nworld", "helloworld"},
		{"  hello world  ", "helloworld"},
		{`"quoted"`, "quoted"},
		{"'single'", "single"},
		{"no-change", "no-change"},
		{"", ""},
	}

	for _, tc := range tests {
		got := cleanInput(tc.input)
		if got != tc.want {
			t.Errorf("cleanInput(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Config round-trip (LoadConfig / SaveConfig)
// ---------------------------------------------------------------------------
func TestConfigRoundTrip(t *testing.T) {
	// Setup temp config path
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	cfg := &Config{
		ServerURL:       "http://localhost:9090",
		PersistentUUID:  "test-uuid-1234",
		UUIDExpiresAt:   time.Now().Add(1 * time.Hour).Truncate(time.Second),
		BlockedCommands: []string{"rm", "sudo"},
		BlockedFolders:  []string{"/etc", "/var/log"},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	loaded := LoadConfig()

	if loaded.ServerURL != cfg.ServerURL {
		t.Errorf("ServerURL = %q, want %q", loaded.ServerURL, cfg.ServerURL)
	}
	if loaded.PersistentUUID != cfg.PersistentUUID {
		t.Errorf("PersistentUUID = %q, want %q", loaded.PersistentUUID, cfg.PersistentUUID)
	}
	if !loaded.UUIDExpiresAt.Equal(cfg.UUIDExpiresAt) {
		t.Errorf("UUIDExpiresAt = %v, want %v", loaded.UUIDExpiresAt, cfg.UUIDExpiresAt)
	}
	if len(loaded.BlockedCommands) != len(cfg.BlockedCommands) {
		t.Errorf("BlockedCommands len = %d, want %d", len(loaded.BlockedCommands), len(cfg.BlockedCommands))
	}
	if len(loaded.BlockedFolders) != len(cfg.BlockedFolders) {
		t.Errorf("BlockedFolders len = %d, want %d", len(loaded.BlockedFolders), len(cfg.BlockedFolders))
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	cfg := LoadConfig()
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("LoadConfig() with missing file: ServerURL = %q, want %q", cfg.ServerURL, DefaultServerURL)
	}
}

func TestLoadConfig_CorruptFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "corrupt.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	_ = os.WriteFile(configPath, []byte("{invalid json"), 0644)

	cfg := LoadConfig()
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("LoadConfig() with corrupt file: ServerURL = %q, want %q", cfg.ServerURL, DefaultServerURL)
	}
}

// ---------------------------------------------------------------------------
// encodePayload / decodePayload round-trip
// ---------------------------------------------------------------------------
func TestEncodeDecodePayload(t *testing.T) {
	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := TestPayload{Name: "test", Value: 42}
	encoded := encodePayload(original)

	if encoded == "" {
		t.Fatal("encodePayload() returned empty string")
	}

	var decoded TestPayload
	decodePayload(encoded, &decoded)

	if decoded.Name != original.Name || decoded.Value != original.Value {
		t.Errorf("decodePayload() = %+v, want %+v", decoded, original)
	}
}

// ---------------------------------------------------------------------------
// parseBlockArgs
// ---------------------------------------------------------------------------
func TestParseBlockArgs(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{[]string{"rm,sudo,shutdown"}, []string{"rm", "sudo", "shutdown"}},
		{[]string{"rm", "sudo"}, []string{"rm", "sudo"}},
		{[]string{"rm, sudo , shutdown"}, []string{"rm", "sudo", "shutdown"}},
		{[]string{}, nil},
		{[]string{","}, nil},
	}

	for _, tc := range tests {
		got := parseBlockArgs(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseBlockArgs(%v) len = %d, want %d", tc.input, len(got), len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseBlockArgs(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// HandleBlockCommand / HandleUnblockCommand integration
// ---------------------------------------------------------------------------
func TestBlockUnblockIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	// Save initial config
	_ = SaveConfig(&Config{ServerURL: DefaultServerURL})

	// Block some commands
	HandleBlockCommand([]string{"cmd", "rm,sudo"})

	cfg := LoadConfig()
	if len(cfg.BlockedCommands) != 2 {
		t.Fatalf("After blocking, expected 2 blocked commands, got %d", len(cfg.BlockedCommands))
	}
	if cfg.BlockedCommands[0] != "rm" || cfg.BlockedCommands[1] != "sudo" {
		t.Errorf("Blocked commands = %v, want [rm sudo]", cfg.BlockedCommands)
	}

	// Block duplicates — should not grow
	HandleBlockCommand([]string{"cmd", "rm"})
	cfg = LoadConfig()
	if len(cfg.BlockedCommands) != 2 {
		t.Errorf("After duplicate block, expected 2 blocked commands, got %d", len(cfg.BlockedCommands))
	}

	// Unblock one
	HandleUnblockCommand([]string{"cmd", "rm"})
	cfg = LoadConfig()
	if len(cfg.BlockedCommands) != 1 {
		t.Fatalf("After unblock, expected 1 blocked command, got %d", len(cfg.BlockedCommands))
	}
	if cfg.BlockedCommands[0] != "sudo" {
		t.Errorf("Remaining blocked command = %q, want 'sudo'", cfg.BlockedCommands[0])
	}
}

// ---------------------------------------------------------------------------
// HandleResetCommand
// ---------------------------------------------------------------------------
func TestHandleResetCommand_All(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	// Save a modified config
	_ = SaveConfig(&Config{
		ServerURL:       "http://custom-server.example.com",
		PersistentUUID:  "custom-uuid",
		BlockedCommands: []string{"rm"},
		BlockedFolders:  []string{"/secret"},
	})

	HandleResetCommand([]string{"all"})

	cfg := LoadConfig()
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("After reset all, ServerURL = %q, want %q", cfg.ServerURL, DefaultServerURL)
	}
	if cfg.PersistentUUID != "" {
		t.Errorf("After reset all, PersistentUUID = %q, want empty", cfg.PersistentUUID)
	}
	if len(cfg.BlockedCommands) != 0 {
		t.Errorf("After reset all, BlockedCommands = %v, want empty", cfg.BlockedCommands)
	}
	if len(cfg.BlockedFolders) != 0 {
		t.Errorf("After reset all, BlockedFolders = %v, want empty", cfg.BlockedFolders)
	}
}

func TestHandleResetCommand_Server(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	_ = SaveConfig(&Config{
		ServerURL:      "http://custom.example.com",
		PersistentUUID: "keep-me",
	})

	HandleResetCommand([]string{"server"})

	cfg := LoadConfig()
	if cfg.ServerURL != DefaultServerURL {
		t.Errorf("After reset server, ServerURL = %q, want %q", cfg.ServerURL, DefaultServerURL)
	}
	if cfg.PersistentUUID != "keep-me" {
		t.Errorf("After reset server, PersistentUUID should be preserved, got %q", cfg.PersistentUUID)
	}
}

// ---------------------------------------------------------------------------
// GetConfigPath (via MINISHARE_CONFIG env)
// ---------------------------------------------------------------------------
func TestGetConfigPath_FromEnv(t *testing.T) {
	expected := "/tmp/test-minishare/config.json"
	t.Setenv("MINISHARE_CONFIG", expected)

	got := GetConfigPath()
	if got != expected {
		t.Errorf("GetConfigPath() = %q, want %q", got, expected)
	}
}

// ---------------------------------------------------------------------------
// postJSON / getJSON with real HTTP (would need a test server)
// These are tested indirectly via the server tests below.
// We test the JSON marshalling here as a sanity check.
// ---------------------------------------------------------------------------
func TestPostJSONMarshal(t *testing.T) {
	payload := map[string]string{"key": "value"}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"key":"value"`) {
		t.Errorf("Unexpected JSON: %s", string(data))
	}
}

// ---------------------------------------------------------------------------
// getDaemonPIDPath / getDaemonUUIDPath / getDaemonLogPath
// ---------------------------------------------------------------------------
func TestDaemonPaths(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("MINISHARE_CONFIG", configPath)

	pidPath := getDaemonPIDPath()
	if !strings.HasSuffix(pidPath, "daemon.pid") {
		t.Errorf("getDaemonPIDPath() = %q, want suffix daemon.pid", pidPath)
	}

	uuidPath := getDaemonUUIDPath()
	if !strings.HasSuffix(uuidPath, "daemon.uuid") {
		t.Errorf("getDaemonUUIDPath() = %q, want suffix daemon.uuid", uuidPath)
	}

	logPath := getDaemonLogPath()
	if !strings.HasSuffix(logPath, "daemon.log") {
		t.Errorf("getDaemonLogPath() = %q, want suffix daemon.log", logPath)
	}

	// They should all be in the same directory
	if filepath.Dir(pidPath) != filepath.Dir(uuidPath) || filepath.Dir(uuidPath) != filepath.Dir(logPath) {
		t.Error("Daemon paths should all be in the same directory")
	}
}

// ---------------------------------------------------------------------------
// processExists (basic sanity — our own PID should exist)
// ---------------------------------------------------------------------------
func TestProcessExists(t *testing.T) {
	pid := os.Getpid()
	if !processExists(pid) {
		t.Errorf("processExists(%d) = false, want true for own PID", pid)
	}

	// Very high PID that almost certainly doesn't exist
	if processExists(9999999) {
		t.Error("processExists(9999999) = true, expected false")
	}
}
