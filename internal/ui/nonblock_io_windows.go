//go:build windows

package ui

import (
	"errors"
	"os"
	"syscall"
)

func setFileNonblock(file *os.File, enabled bool) error {
	return syscall.SetNonblock(syscall.Handle(file.Fd()), enabled)
}

func readFileNonblock(file *os.File, buf []byte) (int, error) {
	return syscall.Read(syscall.Handle(file.Fd()), buf)
}

func isNonblockRetry(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
