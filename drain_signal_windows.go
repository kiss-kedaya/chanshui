//go:build windows

package main

import (
	"context"
	"github.com/kiss-kedaya/chanshui/internal/api"
)

func installDrainSignal(_ *api.Server, _ context.Context) func() { return func() {} }
