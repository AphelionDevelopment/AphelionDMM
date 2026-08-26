package ui

import "testing"

func TestDefaultHostedOriginIsPublicAphelionService(t *testing.T) {
	if DefaultRelayOrigin != "https://mapping.a13.info" {
		t.Fatalf("DefaultRelayOrigin = %q", DefaultRelayOrigin)
	}
	if DefaultHostedOrigin != "https://mapping.a13.info" {
		t.Fatalf("DefaultHostedOrigin = %q", DefaultHostedOrigin)
	}
	if err := validateHostedEndpoint(DefaultHostedOrigin); err != nil {
		t.Fatalf("default hosted origin is invalid: %v", err)
	}
}
