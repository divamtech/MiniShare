package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultServerURL = "http://localhost:8080"

type Config struct {
	ServerURL string `json:"server_url"`
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(homeDir, ".minishare")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

func LoadConfig() *Config {
	path, err := getConfigPath()
	if err != nil {
		return &Config{ServerURL: DefaultServerURL}
	}

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
	path, err := getConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func HandleServerConfig(args []string) {
	if len(args) == 0 || args[0] == "" {
		cfg := LoadConfig()
		fmt.Printf("[MiniShare] Current signaling server: %s\n", cfg.ServerURL)
		return
	}

	input := strings.TrimSpace(args[0])
	inputLower := strings.ToLower(input)

	// Check if user requested a reset
	if inputLower == "reset" || inputLower == "default" || inputLower == "null" || inputLower == "empty" {
		cfg := &Config{ServerURL: DefaultServerURL}
		if err := SaveConfig(cfg); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			return
		}
		fmt.Printf("[MiniShare] Signaling server reset to default: %s\n", DefaultServerURL)
		return
	}

	// Normalize URL format
	url := input
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}

	cfg := &Config{ServerURL: url}
	if err := SaveConfig(cfg); err != nil {
		fmt.Printf("❌ Failed to save config: %v\n", err)
		return
	}
	fmt.Printf("[MiniShare] Signaling server set to: %s\n", url)
}
