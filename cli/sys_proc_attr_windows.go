//go:build windows

package main

import (
	"os"
	"syscall"
)

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func setupWinchSignal(winchChan chan os.Signal) {
	// SIGWINCH is not supported on Windows
}
