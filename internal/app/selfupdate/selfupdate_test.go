package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"aead.dev/minisign"
	minioselfupdate "github.com/minio/selfupdate"
)

func TestParseSignedManifestRejectsBadSignature(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := testManifestPayload(t, privateKey, []byte("artifact"))
	_, untrustedPrivateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := json.Marshal(signedManifest{Manifest: payload, Signature: string(minisign.Sign(untrustedPrivateKey, payload))})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSignedManifest(envelope, publicKey.String()); err == nil {
		t.Fatal("manifest with bad signature accepted")
	}
}

func TestFetchRemoteManifestUsesBoundedJSONResponse(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	payload := testManifestPayload(t, privateKey, []byte("artifact"))
	envelope := signedEnvelope(t, privateKey, payload)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(envelope)
	}))
	defer server.Close()
	manifest, err := FetchRemoteManifestWithClient(context.Background(), server.Client(), server.URL, publicKey.String())
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "v-test" {
		t.Fatalf("version = %q, want v-test", manifest.Version)
	}
}

func TestUpdateRejectsBadOrTruncatedArtifactAndPreservesTarget(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldBuild := []byte("installed executable")
	newBuild := []byte("verified replacement executable")
	target := filepath.Join(t.TempDir(), "app.exe")
	if err := os.WriteFile(target, oldBuild, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := signedArtifact(privateKey, "https://example.invalid/app.exe", newBuild)
	badSignature := artifact
	badSignature.Signature = string(minisign.Sign(privateKey, []byte("different")))
	for _, test := range []struct {
		name     string
		build    []byte
		artifact Artifact
	}{
		{name: "bad signature", build: newBuild, artifact: badSignature},
		{name: "truncated", build: newBuild[:8], artifact: artifact},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := updateTarget(test.build, test.artifact, publicKey.String(), target, target+".old", minioselfupdate.Apply); err == nil {
				t.Fatal("invalid artifact accepted")
			}
			assertFileContents(t, target, oldBuild)
		})
	}
}

func TestUpdatePreservesTargetWhenReplacementPreparationFails(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldBuild := []byte("installed executable")
	newBuild := []byte("verified replacement executable")
	directory := t.TempDir()
	target := filepath.Join(directory, "app.exe")
	if err := os.WriteFile(target, oldBuild, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, ".app.exe.new"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := signedArtifact(privateKey, "https://example.invalid/app.exe", newBuild)
	if err := updateTarget(newBuild, artifact, publicKey.String(), target, target+".old", minioselfupdate.Apply); err == nil {
		t.Fatal("replacement preparation unexpectedly succeeded")
	}
	assertFileContents(t, target, oldBuild)
}

func TestUpdateReplacesVerifiedTargetAndRetainsOldExecutable(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldBuild := []byte("installed executable")
	newBuild := []byte("verified replacement executable")
	target := filepath.Join(t.TempDir(), "app.exe")
	oldSavePath := target + ".old"
	if err := os.WriteFile(target, oldBuild, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := signedArtifact(privateKey, "https://example.invalid/app.exe", newBuild)
	if err := updateTarget(newBuild, artifact, publicKey.String(), target, oldSavePath, minioselfupdate.Apply); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, target, newBuild)
	assertFileContents(t, oldSavePath, oldBuild)
}

func testManifestPayload(t *testing.T, privateKey minisign.PrivateKey, build []byte) []byte {
	t.Helper()
	artifact := signedArtifact(privateKey, "https://example.invalid/app-%VERSION%.exe", build)
	payload, err := json.Marshal(Manifest{
		Name: "AphelionDMM", Version: "v-test", Description: "test",
		DownloadLinks: DownloadLinks{Windows: artifact, Linux: artifact, MacOS: artifact},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func signedEnvelope(t *testing.T, privateKey minisign.PrivateKey, payload []byte) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{"manifest": json.RawMessage(payload), "signature": string(minisign.Sign(privateKey, payload))})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func signedArtifact(privateKey minisign.PrivateKey, url string, build []byte) Artifact {
	digest := sha256.Sum256(build)
	return Artifact{URL: url, SHA256: hex.EncodeToString(digest[:]), Signature: string(minisign.Sign(privateKey, build))}
}

func assertFileContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("file contents = %q, want %q", got, want)
	}
}
