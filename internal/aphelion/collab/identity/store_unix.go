//go:build !windows

package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type platformStore struct {
	directory string
}

func NewPlatformStore(directory string) (SecretStore, error) {
	if directory == "" {
		return nil, fmt.Errorf("secret directory is empty")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, fmt.Errorf("resolve secret directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create secret directory: %w", err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("restrict secret directory: %w", err)
	}
	return &platformStore{directory: absolute}, nil
}

func (store *platformStore) Put(ctx context.Context, name string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := store.recordPath(name, true)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("secret record is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect secret record: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return fmt.Errorf("create secret record: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("restrict secret record: %w", err)
	}
	if _, err := temporary.Write(value); err != nil {
		return fmt.Errorf("write secret record: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync secret record: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close secret record: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace secret record: %w", err)
	}
	committed = true
	return nil
}

func (store *platformStore) Get(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := store.recordPath(name, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("inspect secret record: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("secret record is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("secret record permissions are too broad")
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret record: %w", err)
	}
	return value, nil
}

func (store *platformStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := store.recordPath(name, false)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrSecretNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect secret record: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("secret record is not a regular file")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete secret record: %w", err)
	}
	return nil
}

func (store *platformStore) recordPath(name string, createNamespace bool) (string, error) {
	namespace, identifier, err := splitSecretReference(name)
	if err != nil {
		return "", err
	}
	directory := filepath.Join(store.directory, namespace)
	if createNamespace {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", fmt.Errorf("create secret namespace: %w", err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return "", fmt.Errorf("restrict secret namespace: %w", err)
		}
	}
	return filepath.Join(directory, identifier+".secret"), nil
}
