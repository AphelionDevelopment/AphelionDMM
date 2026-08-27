//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

const windowsServiceName = "AphelionDMMRelay"

type windowsServiceHandler struct {
	arguments []string
	output    io.Writer
	run       func(context.Context, []string, io.Writer) error
}

func runWindowsService(arguments []string) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return true, fmt.Errorf("detect Windows service context: %w", err)
	}
	if !isService {
		return false, nil
	}
	log, err := eventlog.Open(windowsServiceName)
	if err != nil {
		return true, fmt.Errorf("open Windows event source %s: %w", windowsServiceName, err)
	}
	defer func() { _ = log.Close() }()
	output := &eventLogWriter{log: log}
	handler := &windowsServiceHandler{arguments: arguments, output: output, run: run}
	if err := svc.Run(windowsServiceName, handler); err != nil {
		return true, fmt.Errorf("run Windows service %s: %w", windowsServiceName, err)
	}
	return true, nil
}

func (handler windowsServiceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErrors := make(chan error, 1)
	go func() { runErrors <- handler.run(ctx, handler.arguments, handler.output) }()
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	changes <- running
	for {
		select {
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- running
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-runErrors; err != nil {
					_, _ = fmt.Fprintf(handler.output, "service shutdown: %v\n", err)
					return false, 1
				}
				return false, 0
			}
		case err := <-runErrors:
			changes <- svc.Status{State: svc.StopPending}
			if err != nil {
				_, _ = fmt.Fprintf(handler.output, "service runtime: %v\n", err)
				return false, 1
			}
			return false, 0
		}
	}
}

type eventLogWriter struct {
	mu  sync.Mutex
	log *eventlog.Log
}

func (writer *eventLogWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	message := strings.TrimSpace(string(data))
	if message != "" {
		if err := writer.log.Info(1, message); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}
