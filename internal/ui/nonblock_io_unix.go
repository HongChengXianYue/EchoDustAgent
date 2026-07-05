//go:build !windows

package ui

import (
	"errors"
	"os"
	"syscall"
)

func setFileNonblock(file *os.File, enabled bool) error {
	return syscall.SetNonblock(int(file.Fd()), enabled)
}

func readFileNonblock(file *os.File, buf []byte) (int, error) {
	return syscall.Read(int(file.Fd()), buf)
}

func isNonblockRetry(err error) bool {
	return errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK)
}
