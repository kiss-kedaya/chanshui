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
	stop := make(chan struct{})
	go func() {
		select {
		case <-signals:
			server.BeginDrain()
		case <-stop:
		case <-ctx.Done():
		}
	}()
	return func() {
		signal.Stop(signals)
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
}
