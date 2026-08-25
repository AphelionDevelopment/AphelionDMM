package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckRequiresSuccessfulReadinessResponse(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusOK) }))
	t.Cleanup(ready.Close)
	if err := check(context.Background(), ready.Client(), ready.URL); err != nil {
		t.Fatal(err)
	}
	unavailable := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusServiceUnavailable) }))
	t.Cleanup(unavailable.Close)
	if err := check(context.Background(), unavailable.Client(), unavailable.URL); err == nil {
		t.Fatal("check() error = nil")
	}
}
