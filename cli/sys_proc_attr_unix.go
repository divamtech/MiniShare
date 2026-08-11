//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func setupWinchSignal(winchChan chan os.Signal) {
	signal.Notify(winchChan, syscall.SIGWINCH)
}
