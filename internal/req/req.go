package req

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"slices"
	"time"
)

// APHELION EDIT ADDITION START - SECURE_UPDATER
const (
	ConnectTimeout        = 10 * time.Second
	TLSHandshakeTimeout   = 10 * time.Second
	ResponseHeaderTimeout = 15 * time.Second
	TotalTimeout          = 2 * time.Minute
	DefaultMaxBodyBytes   = 256 << 20
)

type Options struct {
	MaxBytes     int64
	ContentTypes []string
}

func NewClient() *http.Client {
	dialer := &net.Dialer{Timeout: ConnectTimeout, KeepAlive: 30 * time.Second}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   TLSHandshakeTimeout,
			ResponseHeaderTimeout: ResponseHeaderTimeout,
			ExpectContinueTimeout: time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
		Timeout: TotalTimeout,
	}
}

func GetWithClient(ctx context.Context, client *http.Client, url string, options Options) ([]byte, error) {
	if client == nil {
		client = NewClient()
	}
	if options.MaxBytes <= 0 {
		return nil, fmt.Errorf("response byte limit must be positive")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to create a request to [%s]: %w", url, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fail to get a response from [%s]: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("fail to get successful response: code [%d]", response.StatusCode)
	}
	if len(options.ContentTypes) > 0 {
		contentType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if parseErr != nil || !slices.Contains(options.ContentTypes, contentType) {
			return nil, fmt.Errorf("unexpected response content type %q", response.Header.Get("Content-Type"))
		}
	}
	if response.ContentLength > options.MaxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", options.MaxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, options.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("fail to read remote data: %w", err)
	}
	if int64(len(body)) > options.MaxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", options.MaxBytes)
	}
	return body, nil
}

// APHELION EDIT ADDITION END

func Get(url string) (body []byte, err error) {
	// APHELION EDIT ADDITION START - SECURE_UPDATER
	return GetWithClient(context.Background(), NewClient(), url, Options{MaxBytes: DefaultMaxBodyBytes})
	// APHELION EDIT ADDITION END
	/* APHELION EDIT REMOVAL START - SECURE_UPDATER
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to create a request to [%s]: %w", url, err)
	}

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fail to get a response from [%s]: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fail to get successful response: code [%d]", resp.StatusCode)
	}

	if body, err = io.ReadAll(resp.Body); err != nil {
		return nil, fmt.Errorf("fail to read remote data: %w", err)
	}

	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("fail to close response: %w", err)
	}

	return body, nil
	APHELION EDIT REMOVAL END */
}
