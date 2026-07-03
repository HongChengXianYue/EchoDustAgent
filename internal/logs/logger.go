package logs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Logger struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	logger *log.Logger
}

func New(workdir string) (*Logger, error) {
	dir := filepath.Join(workdir, ".echo-dust-code", "logs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "agent.log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &Logger{
		path:   path,
		file:   file,
		logger: log.New(file, "", log.LstdFlags|log.Lmicroseconds),
	}, nil
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Logger) Errorf(format string, args ...any) {
	l.write("ERROR", format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.write("INFO", format, args...)
}

func (l *Logger) write(level string, format string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("[%s] %s", level, fmt.Sprintf(format, args...))
}

var (
	globalMu sync.RWMutex
	global   *Logger
)

func SetDefault(logger *Logger) {
	globalMu.Lock()
	global = logger
	globalMu.Unlock()
}

func Default() *Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

func Errorf(format string, args ...any) {
	Default().Errorf(format, args...)
}

func Infof(format string, args ...any) {
	Default().Infof(format, args...)
}
