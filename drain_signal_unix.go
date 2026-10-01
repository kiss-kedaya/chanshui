//go:build !windows

package main

import (
	"chanzhushui/internal/api"
	"context"
	"os"
	"os/signal"
	"syscall"
)

func installDrainSignal(server *api.Server, ctx context.Context) func() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	done := make(chan struct{})
	go func() {
		select {
		case <-signals:
			server.BeginDrain()
		case <-ctx.Done():
		}
		close(done)
	}()
	return func() {
		signal.Stop(signals)
		closeIfOpen(done)
	}
}

func closeIfOpen(done chan struct{}) {
	select {
	case <-done:
	default:
	}
}
