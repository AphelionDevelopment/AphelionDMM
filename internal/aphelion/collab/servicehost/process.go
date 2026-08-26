package servicehost

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"time"
)

// CloudflaredArguments returns the fixed remotely managed tunnel invocation.
func CloudflaredArguments(tokenFile string) []string {
	return []string{"--no-autoupdate", "tunnel", "run", "--token-file", tokenFile}
}

// Cloudflared creates a supervised connector component.
func Cloudflared(executable, tokenFile string, output io.Writer) Component {
	return func(ctx context.Context) error {
		command := exec.CommandContext(ctx, executable, CloudflaredArguments(tokenFile)...)
		command.Stdout = output
		command.Stderr = output
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("run cloudflared: %w", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("cloudflared exited without an error")
	}
}

// WaitHTTP waits until a loopback readiness endpoint returns HTTP 200.
func WaitHTTP(target string) Component {
	return func(ctx context.Context) error {
		client := &http.Client{Timeout: 2 * time.Second}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				return err
			}
			response, err := client.Do(request)
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
}

// LoopbackReadyURL returns a health URL for an HTTP bind address.
func LoopbackReadyURL(bindAddress string) string {
	host, port, err := net.SplitHostPort(bindAddress)
	if err != nil {
		return "http://" + bindAddress + "/v1/health/ready"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/v1/health/ready"
}
