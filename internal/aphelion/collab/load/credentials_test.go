package load

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadEditorTokensReadsSecretFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor-tokens.json")
	if err := os.WriteFile(path, []byte(`{"tokens":["first-secret","second-secret"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tokens, err := LoadEditorTokens(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 2 || tokens[0] != "first-secret" || tokens[1] != "second-secret" {
		t.Fatalf("tokens = %#v", tokens)
	}
}

func TestLoadEditorTokensRejectsUnsafeFilesAndContent(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "unknown field", data: `{"tokens":["secret"],"extra":true}`},
		{name: "empty list", data: `{"tokens":[]}`},
		{name: "empty token", data: `{"tokens":[""]}`},
		{name: "oversized", data: `{"tokens":["` + strings.Repeat("x", (1<<20)+1) + `"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "editor-tokens.json")
			if err := os.WriteFile(path, []byte(test.data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadEditorTokens(path); err == nil {
				t.Fatal("unsafe credential file was accepted")
			}
		})
	}
}

func TestLoadEditorTokensRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.json")
	link := filepath.Join(directory, "link.json")
	if err := os.WriteFile(target, []byte(`{"tokens":["secret"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := LoadEditorTokens(link); err == nil {
		t.Fatal("credential symlink was accepted")
	}
}

func TestLoadEditorTokensRejectsBroadUnixPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions use ACLs")
	}
	path := filepath.Join(t.TempDir(), "editor-tokens.json")
	if err := os.WriteFile(path, []byte(`{"tokens":["secret"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEditorTokens(path); err == nil {
		t.Fatal("broadly readable credential file was accepted")
	}
}
