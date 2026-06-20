package mcp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"local-agent/internal/logs"
	"local-agent/internal/tools"
)

type Options struct {
	Dir            string
	StartTimeout   time.Duration
	RequestTimeout time.Duration
}

type Manager struct {
	config  Config
	clients []*stdioClient
	tools   []tools.Tool
}

func Load(ctx context.Context, options Options) (*Manager, error) {
	if options.StartTimeout <= 0 {
		options.StartTimeout = 10 * time.Second
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 60 * time.Second
	}
	config, err := LoadConfig(options.Dir)
	if err != nil {
		return nil, err
	}
	manager := &Manager{config: config}
	for _, server := range config.Servers {
		if !serverEnabled(server) {
			continue
		}
		client, specs, err := startAndList(ctx, config.Dir, server, options)
		if err != nil {
			logs.Errorf("mcp server %q failed: %v", server.Name, err)
			continue
		}
		manager.clients = append(manager.clients, client)
		for _, spec := range specs {
			if strings.TrimSpace(spec.Name) == "" {
				continue
			}
			manager.tools = append(manager.tools, NewTool(server.Name, spec, client))
		}
	}
	return manager, nil
}

func startAndList(ctx context.Context, baseDir string, server ServerConfig, options Options) (*stdioClient, []ToolSpec, error) {
	startCtx, cancel := context.WithTimeout(ctx, options.StartTimeout)
	defer cancel()

	client, err := startStdioClient(startCtx, baseDir, server, options.RequestTimeout)
	if err != nil {
		return nil, nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = client.Close()
		}
	}()
	if err := client.Initialize(startCtx); err != nil {
		return nil, nil, err
	}
	specs, err := client.ListTools(startCtx)
	if err != nil {
		return nil, nil, err
	}
	ok = true
	return client, specs, nil
}

func (m *Manager) Register(registry *tools.Registry) {
	if m == nil || registry == nil {
		return
	}
	for _, tool := range m.tools {
		registry.Register(tool)
	}
}

func (m *Manager) Tools() []tools.Tool {
	if m == nil {
		return nil
	}
	out := make([]tools.Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var errs []error
	for _, client := range m.clients {
		if err := client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) ConfigPath() string {
	if m == nil || m.config.Dir == "" {
		return ""
	}
	return filepath.Join(m.config.Dir, serverConfigFile)
}
