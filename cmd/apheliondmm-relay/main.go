package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"sdmm/internal/aphelion/collab/relay"
	"sdmm/internal/aphelion/collab/relayruntime"
	"sdmm/internal/aphelion/collab/servicehost"
)

var buildRevision = "development"

func main() {
	if handled, err := runWindowsService(os.Args[1:]); handled {
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("apheliondmm-relay", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "strict relay YAML configuration")
	checkConfig := flags.Bool("check-config", false, "validate configuration and exit")
	cloudflaredPath := flags.String("cloudflared", "", "optional cloudflared executable to supervise")
	tunnelTokenFile := flags.String("tunnel-token-file", "", "Cloudflare tunnel token file used with -cloudflared")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse relay arguments: %w", err)
	}
	if *configPath == "" {
		return fmt.Errorf("-config is required")
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("positional arguments are not accepted")
	}
	if (*cloudflaredPath == "") != (*tunnelTokenFile == "") {
		return fmt.Errorf("-cloudflared and -tunnel-token-file must be supplied together")
	}
	config, err := relay.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if *checkConfig {
		_, _ = fmt.Fprintln(output, "apheliondmm-relay configuration valid")
		return nil
	}
	host := servicehost.Host{
		Relay:     func(ctx context.Context) error { return relayruntime.Run(ctx, config, buildRevision, output) },
		WaitReady: servicehost.WaitHTTP(servicehost.LoopbackReadyURL(config.BindAddress)),
	}
	if *cloudflaredPath != "" {
		host.Tunnel = servicehost.Cloudflared(*cloudflaredPath, *tunnelTokenFile, output)
	}
	return host.Run(ctx)
}
