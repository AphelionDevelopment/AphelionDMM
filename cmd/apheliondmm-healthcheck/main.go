package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

const defaultHealthURL = "http://127.0.0.1:8080/v1/health/ready"

func main() {
	url := os.Getenv("APHELIONDMM_HEALTHCHECK_URL")
	if url == "" {
		url = defaultHealthURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := check(ctx, http.DefaultClient, url); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(ctx context.Context, client *http.Client, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create readiness request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request readiness: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned HTTP %d", response.StatusCode)
	}
	return nil
}
