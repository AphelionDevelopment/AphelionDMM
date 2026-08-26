package load

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
)

const maxEditorTokenFileBytes = 1 << 20

func LoadEditorTokens(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect editor credential file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("editor credential file must be a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("editor credential file permissions must not grant group or other access")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open editor credential file: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxEditorTokenFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read editor credential file: %w", err)
	}
	if len(data) > maxEditorTokenFileBytes {
		return nil, fmt.Errorf("editor credential file exceeds 1 MiB")
	}
	var document struct {
		Tokens []string `json:"tokens"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode editor credential file: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("decode editor credential file: trailing JSON is forbidden")
	}
	if len(document.Tokens) == 0 || len(document.Tokens) > 100 {
		return nil, fmt.Errorf("editor credential file must contain between 1 and 100 tokens")
	}
	for _, token := range document.Tokens {
		if token == "" {
			return nil, fmt.Errorf("editor credential file contains an empty token")
		}
	}
	return document.Tokens, nil
}
