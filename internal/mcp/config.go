package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const serverConfigFile = "servers.json"

type Config struct {
	Dir     string
	Servers []ServerConfig
}

type ServerConfig struct {
	Name    string            `json:"name,omitempty"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
}

type serverConfigDocument struct {
	Servers json.RawMessage `json:"servers"`
}

func LoadConfig(dir string) (Config, error) {
	dir = absPath(expandHome(strings.TrimSpace(dir)))
	if dir == "" {
		return Config{}, fmt.Errorf("mcp dir is required")
	}
	path := filepath.Join(dir, serverConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{Dir: dir}, nil
		}
		return Config{}, err
	}
	var doc serverConfigDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	servers, err := parseServers(doc.Servers)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	for i := range servers {
		if err := normalizeServerConfig(&servers[i]); err != nil {
			return Config{}, fmt.Errorf("%s: server %d: %w", path, i+1, err)
		}
	}
	return Config{Dir: dir, Servers: servers}, nil
}

func parseServers(raw json.RawMessage) ([]ServerConfig, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var byName map[string]ServerConfig
	if err := json.Unmarshal(raw, &byName); err == nil {
		names := make([]string, 0, len(byName))
		for name := range byName {
			names = append(names, name)
		}
		sort.Strings(names)
		out := make([]ServerConfig, 0, len(names))
		for _, name := range names {
			cfg := byName[name]
			if strings.TrimSpace(cfg.Name) == "" {
				cfg.Name = name
			}
			out = append(out, cfg)
		}
		return out, nil
	}
	var list []ServerConfig
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("servers must be an object or array")
	}
	return list, nil
}

func normalizeServerConfig(cfg *ServerConfig) error {
	cfg.Name = sanitizeSegment(cfg.Name)
	cfg.Command = strings.TrimSpace(cfg.Command)
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.Command == "" {
		return fmt.Errorf("command is required")
	}
	return nil
}

func serverEnabled(cfg ServerConfig) bool {
	return cfg.Enabled == nil || *cfg.Enabled
}

func expandHome(path string) string {
	switch {
	case path == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	case strings.HasPrefix(path, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	default:
		return path
	}
}

func absPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
