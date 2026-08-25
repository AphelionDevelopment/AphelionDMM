package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestNewOTLPRejectsNonHTTPSEndpoint(t *testing.T) {
	if _, _, err := NewOTLP(context.Background(), "http://collector.example.test"); err == nil {
		t.Fatal("NewOTLP() accepted a non-HTTPS endpoint")
	}
}

func TestNewOTLPConfiguresProvidersWithoutConnectingAtStartup(t *testing.T) {
	observability, shutdown, err := NewOTLP(context.Background(), "https://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if observability == nil || shutdown == nil {
		t.Fatal("NewOTLP() returned an incomplete telemetry lifecycle")
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = shutdown(shutdownContext)
}
