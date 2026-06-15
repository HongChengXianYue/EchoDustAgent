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
	input     io.Reader
	output    io.Writer
	onToggle  func()
	onFullLog func(input *os.File, output *os.File)
	poll      time.Duration

	mu      sync.Mutex
	running bool
	stop    chan struct{}
	done    chan struct{}
}

func newToggleKeyWatcher(input io.Reader, output io.Writer, onToggle func(), onFullLog func(input *os.File, output *os.File), pollMilliseconds int) *toggleKeyWatcher {
	if pollMilliseconds <= 0 {
		pollMilliseconds = DefaultOptions().TogglePollMilliseconds
	}
	return &toggleKeyWatcher{
		input:     input,
		output:    output,
		onToggle:  onToggle,
		onFullLog: onFullLog,
		poll:      time.Duration(pollMilliseconds) * time.Millisecond,
	}
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
	outputFile, _ := w.output.(*os.File)
	w.stop = make(chan struct{})
	w.done = make(chan struct{})
	w.running = true
	go w.run(file, outputFile, w.stop, w.done)
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

func (w *toggleKeyWatcher) run(file *os.File, outputFile *os.File, stop <-chan struct{}, done chan<- struct{}) {
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
				time.Sleep(w.poll)
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
			case 20:
				if w.onFullLog != nil && outputFile != nil && isTerminal(outputFile) {
					w.onFullLog(file, outputFile)
				}
			case 3:
				if process, err := os.FindProcess(os.Getpid()); err == nil {
					_ = process.Signal(os.Interrupt)
				}
			}
		}
	}
}
