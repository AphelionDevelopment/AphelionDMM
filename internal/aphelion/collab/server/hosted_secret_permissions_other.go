//go:build !windows

package server

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

func secretFilePermissionsAllowed(filePath string, mode os.FileMode) (bool, error) {
	if containerSecretPath(filePath) {
		fileWritable, err := probeSecretFileWritable(filePath)
		if err != nil {
			return false, err
		}
		return secretFilePermissionsAllowedOther(filePath, mode, fileWritable), nil
	}
	return secretFilePermissionsAllowedOther(filePath, mode, false), nil
}

func secretFilePermissionsAllowedOther(filePath string, mode os.FileMode, fileWritable bool) bool {
	if containerSecretPath(filePath) {
		return !fileWritable
	}
	return mode.Perm()&0o077 == 0
}

func containerSecretPath(filePath string) bool {
	return strings.HasPrefix(path.Clean(filepath.ToSlash(filePath)), "/run/secrets/")
}

func probeSecretFileWritable(filePath string) (bool, error) {
	file, err := os.OpenFile(filePath, os.O_WRONLY, 0)
	if err == nil {
		return true, file.Close()
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return false, nil
	}
	return false, err
}
