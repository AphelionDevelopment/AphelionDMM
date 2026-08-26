package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrSecretNotFound = errors.New("secret not found")
	secretNamePattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type SecretStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

func validateSecretReference(reference string, requiredNamespace string) error {
	parts := strings.Split(reference, "/")
	if len(parts) != 2 || !validNamespace(parts[0]) || !secretNamePattern.MatchString(parts[1]) {
		return fmt.Errorf("invalid secret reference")
	}
	if requiredNamespace != "" && parts[0] != requiredNamespace {
		return fmt.Errorf("invalid secret namespace")
	}
	return nil
}

func validNamespace(namespace string) bool {
	switch namespace {
	case IdentityNamespace, SessionKeyNamespace, GroupKeyNamespace, CapabilityNamespace:
		return true
	default:
		return false
	}
}

func splitSecretReference(reference string) (string, string, error) {
	if err := validateSecretReference(reference, ""); err != nil {
		return "", "", err
	}
	parts := strings.Split(reference, "/")
	return parts[0], parts[1], nil
}
