//go:build windows

package main

import (
	"chanzhushui/internal/api"
	"context"
)

func installDrainSignal(_ *api.Server, _ context.Context) func() { return func() {} }
