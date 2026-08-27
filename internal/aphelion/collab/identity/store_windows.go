//go:build windows

package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
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
	protected, err := protectData(value, []byte(name))
	if err != nil {
		return fmt.Errorf("protect secret: %w", err)
	}
	defer zero(protected)
	temporary, err := os.CreateTemp(filepath.Dir(path), ".secret-*")
	if err != nil {
		return fmt.Errorf("create protected secret: %w", err)
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
		return fmt.Errorf("restrict protected secret: %w", err)
	}
	if _, err := temporary.Write(protected); err != nil {
		return fmt.Errorf("write protected secret: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync protected secret: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close protected secret: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace protected secret: %w", err)
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
	protected, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read protected secret: %w", err)
	}
	defer zero(protected)
	value, err := unprotectData(protected, []byte(name))
	if err != nil {
		return nil, fmt.Errorf("unprotect secret: %w", err)
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
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return ErrSecretNotFound
	} else if err != nil {
		return fmt.Errorf("delete protected secret: %w", err)
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
	}
	return filepath.Join(directory, identifier+".secret"), nil
}

func protectData(plaintext []byte, entropy []byte) ([]byte, error) {
	input := dataBlob(plaintext)
	optionalEntropy := dataBlob(entropy)
	var output windows.DataBlob
	if err := windows.CryptProtectData(&input, nil, &optionalEntropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, err
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data))) }()
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func unprotectData(ciphertext []byte, entropy []byte) ([]byte, error) {
	input := dataBlob(ciphertext)
	optionalEntropy := dataBlob(entropy)
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, &optionalEntropy, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, err
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(output.Data))) }()
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func dataBlob(value []byte) windows.DataBlob {
	if len(value) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
}
