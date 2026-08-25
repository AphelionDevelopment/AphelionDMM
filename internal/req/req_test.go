package req

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultClientBoundsNetworkPhases(t *testing.T) {
	t.Parallel()

	client := NewClient()
	if client.Timeout <= 0 {
		t.Fatal("client has no total timeout")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.TLSHandshakeTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 {
		t.Fatalf("transport timeouts = TLS %s, header %s", transport.TLSHandshakeTimeout, transport.ResponseHeaderTimeout)
	}
	if transport.DialContext == nil || ConnectTimeout <= 0 {
		t.Fatal("transport has no bounded connect dialer")
	}
}

func TestGetWithClientRejectsTLSFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("untrusted"))
	}))
	defer server.Close()
	_, err := GetWithClient(context.Background(), NewClient(), server.URL, Options{MaxBytes: 32})
	if err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
}

func TestGetWithClientBoundsBlockedConnect(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
		Timeout: 20 * time.Millisecond,
	}
	started := time.Now()
	_, err := GetWithClient(context.Background(), client, "https://example.invalid", Options{MaxBytes: 32})
	if err == nil {
		t.Fatal("blocked connect accepted")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("blocked connect returned after %s", elapsed)
	}
}

func TestGetWithClientBoundsHeadersAndBody(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{name: "header", handler: func(writer http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			writer.WriteHeader(http.StatusOK)
		}},
		{name: "body", handler: func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(100 * time.Millisecond)
			_, _ = writer.Write([]byte("late"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(test.handler)
			defer server.Close()
			client := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 20 * time.Millisecond}, Timeout: 40 * time.Millisecond}
			_, err := GetWithClient(context.Background(), client, server.URL, Options{MaxBytes: 32})
			if err == nil {
				t.Fatal("timed-out response accepted")
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestGetWithClientRejectsStatusTypeAndOversize(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{name: "status", status: http.StatusBadGateway, contentType: "application/json", body: `{}`},
		{name: "content type", status: http.StatusOK, contentType: "text/html", body: `{}`},
		{name: "oversize", status: http.StatusOK, contentType: "application/json", body: strings.Repeat("x", 33)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()
			_, err := GetWithClient(context.Background(), server.Client(), server.URL, Options{MaxBytes: 32, ContentTypes: []string{"application/json"}})
			if err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}
