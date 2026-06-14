package ui

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
)

type toggleKeyWatcher struct {
	input    io.Reader
	onToggle func()

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

func newToggleKeyWatcher(input io.Reader, onToggle func()) *toggleKeyWatcher {
	return &toggleKeyWatcher{input: input, onToggle: onToggle}
}

func (w *toggleKeyWatcher) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return
	}
	file, ok := w.input.(*os.File)
	if !ok || !isTerminal(file) {
		return
	}
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	w.running = true
	go w.run(file, w.stop, w.done)
}

func (w *toggleKeyWatcher) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	stop := w.stop
	done := w.done
	w.running = false
	w.stop = nil
	w.done = nil
	close(stop)
	w.mu.Unlock()

	<-done
}

func (w *toggleKeyWatcher) run(file *os.File, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	raw := enterRawMode(file)
	defer raw.restore()

	fd := int(file.Fd())
	_ = syscall.SetNonblock(fd, true)
	defer syscall.SetNonblock(fd, false)

	var buf [16]byte
	for {
		select {
		case <-stop:
			return
		default:
		}

		n, err := syscall.Read(fd, buf[:])
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(40 * time.Millisecond)
				continue
			}
			return
		}
		for _, b := range buf[:n] {
			switch b {
			case 5:
				if w.onToggle != nil {
					go w.onToggle()
				}
			case 3:
				if process, err := os.FindProcess(os.Getpid()); err == nil {
					_ = process.Signal(os.Interrupt)
				}
			}
		}
	}
}
