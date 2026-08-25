package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sdmm/internal/aphelion/collab/testoidc"
)

const fixtureSecretEnvironment = "APHELIONDMM_OIDC_FIXTURE_CLIENT_SECRET"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer, lookup func(string) (string, bool)) int {
	flags := flag.NewFlagSet("apheliondmm-oidc-fixture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listenAddress := flags.String("listen", "127.0.0.1:9443", "local TLS listen address")
	issuer := flags.String("issuer", "", "exact HTTPS issuer origin")
	redirectURL := flags.String("redirect-url", "", "exact hosted-service OIDC callback URL")
	clientID := flags.String("client-id", "apheliondmm-local-fixture", "test OIDC client ID")
	caPath := flags.String("ca-file", "", "new protected file that receives the disposable root CA")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	clientSecret, found := lookup(fixtureSecretEnvironment)
	if *issuer == "" || *redirectURL == "" || *caPath == "" || *clientID == "" || !found || clientSecret == "" || flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "-issuer, -redirect-url, -ca-file, -client-id, and %s are required; positional arguments are forbidden\n", fixtureSecretEnvironment)
		return 2
	}
	parsedIssuer, err := url.Parse(*issuer)
	if err != nil || parsedIssuer.Hostname() == "" {
		_, _ = fmt.Fprintln(stderr, "issuer URL is invalid")
		return 2
	}
	fixture, err := testoidc.New(testoidc.Config{
		Issuer: *issuer, ClientID: *clientID, ClientSecret: clientSecret, RedirectURL: *redirectURL,
		Subject: "aphelion-local-fixture-subject", DisplayName: "Aphelion Local Fixture",
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "initialize OIDC fixture: %v\n", err)
		return 1
	}
	certificate, caPEM, err := testoidc.GenerateLocalTLSCertificate([]string{parsedIssuer.Hostname(), "localhost", "127.0.0.1"}, time.Now())
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "generate fixture TLS certificate: %v\n", err)
		return 1
	}
	if err := writeCAFile(*caPath, caPEM); err != nil {
		_, _ = fmt.Fprintf(stderr, "write fixture CA: %v\n", err)
		return 1
	}
	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		_ = os.Remove(*caPath)
		_, _ = fmt.Fprintf(stderr, "listen for OIDC fixture: %v\n", err)
		return 1
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{
		Handler: fixture.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second,
	}
	tlsListener := tls.NewListener(listener, fixtureTLSConfig(certificate))
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- server.Serve(tlsListener) }()
	_, _ = fmt.Fprintf(stdout, "issuer=%q ca_file=%q\n", *issuer, *caPath)
	select {
	case <-ctx.Done():
	case err := <-errorsChannel:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_, _ = fmt.Fprintf(stderr, "serve OIDC fixture: %v\n", err)
			return 1
		}
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_, _ = fmt.Fprintf(stderr, "shutdown OIDC fixture: %v\n", err)
		return 1
	}
	return 0
}

func fixtureTLSConfig(certificate tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
}

func writeCAFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	writeErr := func() error {
		if _, err := file.Write(data); err != nil {
			return err
		}
		return file.Sync()
	}()
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	return closeErr
}
