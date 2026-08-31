//go:build !windows

package server

import "testing"

func secureSecretFixturePermissions(t *testing.T, _ string) {
	t.Helper()
}

func TestSecretFilePermissionsAllowContainerManagedSecretOnly(t *testing.T) {
	if !secretFilePermissionsAllowedOther("/run/secrets/database_dsn", 0o777, false) {
		t.Fatal("read-only Docker secret permissions were rejected")
	}
	if secretFilePermissionsAllowedOther("/run/secrets/database_dsn", 0o777, true) {
		t.Fatal("writable Docker secret was accepted")
	}
	if secretFilePermissionsAllowedOther("/tmp/database_dsn", 0o444, false) {
		t.Fatal("broad permissions outside /run/secrets were accepted")
	}
	if !secretFilePermissionsAllowedOther("/tmp/database_dsn", 0o600, true) {
		t.Fatal("owner-only secret permissions were rejected")
	}
}
