package selfupdate

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"aead.dev/minisign"
	minioselfupdate "github.com/minio/selfupdate"
)

type applyUpdate func(io.Reader, minioselfupdate.Options) error

// APHELION EDIT ADDITION START - SECURE_UPDATER
func Update(build []byte, artifact Artifact) error {
	target, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	return updateTarget(build, artifact, TrustedPublicKey, target, target+".old", minioselfupdate.Apply)
}

func updateTarget(build []byte, artifact Artifact, publicKeyText, targetPath, oldSavePath string, apply applyUpdate) error {
	checksum, err := verifyArtifact(build, artifact, publicKeyText)
	if err != nil {
		return err
	}
	if apply == nil {
		return fmt.Errorf("update replacement mechanism is not configured")
	}
	options := minioselfupdate.Options{
		TargetPath:  targetPath,
		OldSavePath: oldSavePath,
		Hash:        crypto.SHA256,
		Checksum:    checksum,
	}
	if err := apply(bytes.NewReader(build), options); err != nil {
		if rollbackErr := minioselfupdate.RollbackError(err); rollbackErr != nil {
			return fmt.Errorf("unable to self-update and rollback failed: %v: %w", err, rollbackErr)
		}
		return fmt.Errorf("unable to self-update: %w", err)
	}
	return nil
}

func verifyArtifact(build []byte, artifact Artifact, publicKeyText string) ([]byte, error) {
	if err := artifact.validate(); err != nil {
		return nil, err
	}
	publicKey, err := parsePublicKey(publicKeyText)
	if err != nil {
		return nil, err
	}
	expected, err := hex.DecodeString(artifact.SHA256)
	if err != nil {
		return nil, fmt.Errorf("decode artifact SHA-256: %w", err)
	}
	digest := sha256.Sum256(build)
	if !hmac.Equal(expected, digest[:]) {
		return nil, fmt.Errorf("update artifact SHA-256 verification failed")
	}
	if !minisign.Verify(publicKey, build, []byte(artifact.Signature)) {
		return nil, fmt.Errorf("update artifact signature verification failed")
	}
	return expected, nil
}

// APHELION EDIT ADDITION END

/* APHELION EDIT REMOVAL START - SECURE_UPDATER
func Update(build []byte) error {
	if err := selfupdate.Apply(bytes.NewReader(build), selfupdate.Options{}); err != nil {
		return fmt.Errorf("unable to self-update: %w", err)
	}
	return nil
}
APHELION EDIT REMOVAL END */
