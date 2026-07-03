package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const protocolVersion = "2024-11-05"

type stdioClient struct {
	serverName     string
	command        *exec.Cmd
	stdin          io.WriteCloser
	stdout         *bufio.Reader
	stderr         *boundedBuffer
	requestTimeout time.Duration

	mu     sync.Mutex
	nextID int64
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func startStdioClient(ctx context.Context, baseDir string, cfg ServerConfig, requestTimeout time.Duration) (*stdioClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	command := resolveCommand(baseDir, cfg.Command)
	cmd := exec.Command(command, cfg.Args...)
	cmd.Dir = resolveWorkingDir(baseDir, cfg.CWD)
	cmd.Env = append(os.Environ(), envPairs(cfg.Env)...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := &boundedBuffer{limit: 16 * 1024}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &stdioClient{
		serverName:     cfg.Name,
		command:        cmd,
		stdin:          stdin,
		stdout:         bufio.NewReader(stdoutPipe),
		stderr:         stderr,
		requestTimeout: requestTimeout,
	}, nil
}

func (c *stdioClient) Initialize(ctx context.Context) error {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "echo-dust-code",
			"version": "0.1.0",
		},
	}, &result); err != nil {
		return err
	}
	return c.notify(ctx, "notifications/initialized", map[string]any{})
}

func (c *stdioClient) ListTools(ctx context.Context) ([]ToolSpec, error) {
	var result struct {
		Tools []ToolSpec `json:"tools"`
	}
	if err := c.request(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (c *stdioClient) CallTool(ctx context.Context, name string, args json.RawMessage) (CallResult, error) {
	var arguments map[string]any
	if len(bytes.TrimSpace(args)) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return CallResult{}, err
		}
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	var result CallResult
	if err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	}, &result); err != nil {
		return CallResult{}, err
	}
	return result, nil
}

func (c *stdioClient) Close() error {
	if c == nil || c.command == nil {
		return nil
	}
	_ = c.stdin.Close()
	if c.command.Process != nil {
		_ = c.command.Process.Kill()
	}
	return c.command.Wait()
}

func (c *stdioClient) request(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()

	c.nextID++
	id := c.nextID
	if err := c.write(ctx, rpcMessage{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return err
	}
	for {
		message, err := c.read(ctx)
		if err != nil {
			return err
		}
		if !sameID(message.ID, id) {
			continue
		}
		if message.Error != nil {
			return fmt.Errorf("mcp %s %s: %s", c.serverName, method, message.Error.Message)
		}
		if out == nil {
			return nil
		}
		if len(message.Result) == 0 {
			return nil
		}
		if rawAware, ok := out.(*CallResult); ok {
			rawAware.Raw = append(rawAware.Raw[:0], message.Result...)
		}
		return json.Unmarshal(message.Result, out)
	}
}

func (c *stdioClient) notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	return c.write(ctx, rpcMessage{JSONRPC: "2.0", Method: method, Params: params})
}

func (c *stdioClient) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

func (c *stdioClient) write(ctx context.Context, message rpcMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	done := make(chan error, 1)
	go func() {
		_, err := c.stdin.Write(data)
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (c *stdioClient) read(ctx context.Context) (rpcMessage, error) {
	type readResult struct {
		line string
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		line, err := c.stdout.ReadString('\n')
		done <- readResult{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	case result := <-done:
		if result.err != nil {
			return rpcMessage{}, c.decorateReadError(result.err)
		}
		var message rpcMessage
		if err := json.Unmarshal([]byte(strings.TrimSpace(result.line)), &message); err != nil {
			return rpcMessage{}, err
		}
		return message, nil
	}
}

func (c *stdioClient) decorateReadError(err error) error {
	if stderr := strings.TrimSpace(c.stderr.String()); stderr != "" {
		return fmt.Errorf("%w: stderr: %s", err, stderr)
	}
	return err
}

func sameID(raw any, want int64) bool {
	switch value := raw.(type) {
	case float64:
		return int64(value) == want
	case int64:
		return value == want
	case int:
		return int64(value) == want
	case json.Number:
		n, err := value.Int64()
		return err == nil && n == want
	default:
		return fmt.Sprint(value) == fmt.Sprint(want)
	}
}

func resolveCommand(baseDir string, command string) string {
	command = expandHome(strings.TrimSpace(command))
	if filepath.IsAbs(command) {
		return command
	}
	if strings.ContainsRune(command, os.PathSeparator) {
		return filepath.Join(baseDir, command)
	}
	return command
}

func resolveWorkingDir(baseDir string, dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return baseDir
	}
	dir = expandHome(dir)
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Join(baseDir, dir)
}

func envPairs(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

type boundedBuffer struct {
	mu    sync.Mutex
	limit int
	buf   []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if b.limit > 0 && len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
