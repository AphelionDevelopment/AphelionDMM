package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"sdmm/internal/aphelion/buildcheck"
)

const toolchainManifestPath = "tools/toolchain/manifest.json"

func main() {
	os.Exit(run(context.Background(), os.Stdout, os.Stderr, toolchainManifestPath, buildcheck.ExecRunner{}))
}

func run(ctx context.Context, stdout, stderr io.Writer, manifestPath string, runner buildcheck.Runner) int {
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		reportError(stderr, "open toolchain manifest: %v", err)
		return 1
	}

	manifest, err := buildcheck.DecodeManifest(manifestFile)
	closeErr := manifestFile.Close()
	if err != nil {
		reportError(stderr, "%v", err)
		return 1
	}
	if closeErr != nil {
		reportError(stderr, "close toolchain manifest: %v", closeErr)
		return 1
	}

	exitCode := 0
	for _, result := range buildcheck.Check(ctx, runner, manifest) {
		status := "ok"
		if !result.OK {
			status = "fail"
			exitCode = 1
		}
		line := fmt.Sprintf("status=%s tool=%s expected=%q actual=%q", status, result.Name, result.Expected, result.Actual)
		if result.Detail != "" {
			line += fmt.Sprintf(" detail=%q", result.Detail)
		}
		if _, err := fmt.Fprintln(stdout, line); err != nil {
			reportError(stderr, "write environment report: %v", err)
			return 1
		}
	}
	return exitCode
}

func reportError(writer io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(writer, format+"\n", args...)
}
