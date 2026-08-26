//go:build windows

package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceHandlerReportsRunningAndStopsCleanly(t *testing.T) {
	for _, test := range []struct {
		name    string
		command svc.Cmd
	}{
		{name: "stop", command: svc.Stop},
		{name: "shutdown", command: svc.Shutdown},
	} {
		t.Run(test.name, func(t *testing.T) {
			runStarted := make(chan struct{})
			observedArguments := make(chan []string, 1)
			handler := windowsServiceHandler{
				arguments: []string{"-config", "relay.yaml"},
				output:    io.Discard,
				run: func(ctx context.Context, arguments []string, _ io.Writer) error {
					observedArguments <- arguments
					close(runStarted)
					<-ctx.Done()
					return nil
				},
			}
			requests := make(chan svc.ChangeRequest, 1)
			changes := make(chan svc.Status, 4)
			type result struct {
				dynamicStart bool
				exitCode     uint32
			}
			done := make(chan result, 1)
			go func() {
				dynamicStart, exitCode := handler.Execute(nil, requests, changes)
				done <- result{dynamicStart: dynamicStart, exitCode: exitCode}
			}()

			require.Equal(t, svc.StartPending, (<-changes).State)
			running := <-changes
			require.Equal(t, svc.Running, running.State)
			require.Equal(t, svc.AcceptStop|svc.AcceptShutdown, running.Accepts)
			<-runStarted
			require.Equal(t, []string{"-config", "relay.yaml"}, <-observedArguments)
			requests <- svc.ChangeRequest{Cmd: test.command}
			require.Equal(t, svc.StopPending, (<-changes).State)
			select {
			case observed := <-done:
				require.False(t, observed.dynamicStart)
				require.Zero(t, observed.exitCode)
			case <-time.After(time.Second):
				t.Fatal("service handler did not return after stop request")
			}
		})
	}
}
