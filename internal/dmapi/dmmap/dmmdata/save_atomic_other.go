//go:build !windows

package dmmdata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func replaceFile(source string, target string) error {
	if err := os.Rename(source, target); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("open destination directory: %w", err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}
