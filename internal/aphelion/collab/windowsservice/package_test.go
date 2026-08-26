package windowsservice_test

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServiceScriptSetupCreatesEditableRelayConfig(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows service package is exercised on Windows")
	}
	root := repositoryRoot(t)
	packageRoot := t.TempDir()
	command := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "deploy", "relay", "service.ps1"), "-Action", "Setup", "-PackageRoot", packageRoot)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	config, err := os.ReadFile(filepath.Join(packageRoot, "relay.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(config), "bind_address: 127.0.0.1:8080")
	require.Contains(t, string(output), "mapping.a13.info")
}

func TestServiceScriptBuildsAndValidatesDockerFreePackage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows service package is exercised on Windows")
	}
	root := repositoryRoot(t)
	outputRoot := t.TempDir()
	script := filepath.Join(root, "deploy", "relay", "service.ps1")
	command := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", script, "-Action", "Package", "-PackageRoot", outputRoot)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	stage := filepath.Join(outputRoot, "AphelionDMM-Relay-Windows-x64")
	require.FileExists(t, filepath.Join(stage, "bin", "apheliondmm-relay.exe"))
	require.FileExists(t, filepath.Join(stage, "bin", "apheliondmm-healthcheck.exe"))
	require.FileExists(t, filepath.Join(stage, "service.ps1"))
	require.FileExists(t, filepath.Join(stage, "relay.yaml.example"))
	require.FileExists(t, filepath.Join(stage, "README.md"))
	require.FileExists(t, filepath.Join(stage, "package-manifest.json"))
	archivePath := filepath.Join(outputRoot, "AphelionDMM-Relay-Windows-x64.zip")
	require.FileExists(t, archivePath)
	command = exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", script, "-Action", "Package", "-PackageRoot", outputRoot)
	output, err = command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "already exists")
	command = exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", script, "-Action", "Package", "-PackageRoot", outputRoot, "-Force")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	archive, err := zip.OpenReader(archivePath)
	require.NoError(t, err)
	defer archive.Close()
	for _, file := range archive.File {
		name := filepath.ToSlash(file.Name)
		require.NotContains(t, name, "Dockerfile")
		require.NotContains(t, name, "compose.")
	}

	command = exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", filepath.Join(stage, "service.ps1"), "-Action", "Setup", "-PackageRoot", stage)
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	command = exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", filepath.Join(stage, "service.ps1"), "-Action", "Validate", "-PackageRoot", stage, "-SkipCloudflare")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	badToken := filepath.Join(outputRoot, "bad-token.txt")
	require.NoError(t, os.WriteFile(badToken, []byte("not-a-token"), 0o600))
	command = exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-File", filepath.Join(stage, "service.ps1"), "-Action", "Validate", "-PackageRoot", stage, "-TunnelTokenFile", badToken)
	output, err = command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(output), "tunnel token is invalid")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
}
