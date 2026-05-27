package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPorkbunCredentials_FromFile(t *testing.T) {
	withoutKeychain(t)

	path := filepath.Join(t.TempDir(), "porkbun.env")
	writeCredentialsFile(t, path, `
# comment
PORKBUN_API_KEY=file-api
export PORKBUN_SECRET_API_KEY='file-secret'
IGNORED=value
`)

	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, path)

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "file-api" {
		t.Fatalf("APIKey=%q, want file-api", got.APIKey)
	}
	if got.SecretAPIKey != "file-secret" {
		t.Fatalf("SecretAPIKey=%q, want file-secret", got.SecretAPIKey)
	}
}

func TestLoadPorkbunCredentials_EnvOverridesFile(t *testing.T) {
	withoutKeychain(t)

	path := filepath.Join(t.TempDir(), "porkbun.env")
	writeCredentialsFile(t, path, `
PORKBUN_API_KEY=file-api
PORKBUN_SECRET_API_KEY=file-secret
`)

	t.Setenv(porkbunAPIKeyEnv, "env-api")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, path)

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "env-api" {
		t.Fatalf("APIKey=%q, want env-api", got.APIKey)
	}
	if got.SecretAPIKey != "file-secret" {
		t.Fatalf("SecretAPIKey=%q, want file-secret", got.SecretAPIKey)
	}
}

func TestLoadPorkbunCredentials_CompleteEnvSkipsKeychain(t *testing.T) {
	orig := readPorkbunCredentialsFromKeychain
	readPorkbunCredentialsFromKeychain = func() (porkbunCredentials, error) {
		return porkbunCredentials{}, errors.New("should not read keychain")
	}
	t.Cleanup(func() {
		readPorkbunCredentialsFromKeychain = orig
	})

	t.Setenv(porkbunAPIKeyEnv, "env-api")
	t.Setenv(porkbunSecretAPIKeyEnv, "env-secret")
	t.Setenv(porkbunCredentialsFilePathEnv, filepath.Join(t.TempDir(), "missing.env"))

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "env-api" {
		t.Fatalf("APIKey=%q, want env-api", got.APIKey)
	}
	if got.SecretAPIKey != "env-secret" {
		t.Fatalf("SecretAPIKey=%q, want env-secret", got.SecretAPIKey)
	}
}

func TestLoadPorkbunCredentials_ExplicitMissingFileFails(t *testing.T) {
	withoutKeychain(t)

	path := filepath.Join(t.TempDir(), "missing.env")

	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, path)

	_, err := loadPorkbunCredentials()
	if err == nil {
		t.Fatal("loadPorkbunCredentials: expected error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err=%v, want missing-file error", err)
	}
}

func TestLoadPorkbunCredentials_KeychainErrorFallsBackToFile(t *testing.T) {
	withKeychainError(t, errors.New("keychain locked"))

	path := filepath.Join(t.TempDir(), "porkbun.env")
	writeCredentialsFile(t, path, `
PORKBUN_API_KEY=file-api
PORKBUN_SECRET_API_KEY=file-secret
`)

	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, path)

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "file-api" {
		t.Fatalf("APIKey=%q, want file-api", got.APIKey)
	}
	if got.SecretAPIKey != "file-secret" {
		t.Fatalf("SecretAPIKey=%q, want file-secret", got.SecretAPIKey)
	}
}

func TestLoadPorkbunCredentials_KeychainBeforeFile(t *testing.T) {
	withKeychain(t, porkbunCredentials{
		APIKey:       "keychain-api",
		SecretAPIKey: "keychain-secret",
	})

	path := filepath.Join(t.TempDir(), "porkbun.env")
	writeCredentialsFile(t, path, `
PORKBUN_API_KEY=file-api
PORKBUN_SECRET_API_KEY=file-secret
`)

	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, path)

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "keychain-api" {
		t.Fatalf("APIKey=%q, want keychain-api", got.APIKey)
	}
	if got.SecretAPIKey != "keychain-secret" {
		t.Fatalf("SecretAPIKey=%q, want keychain-secret", got.SecretAPIKey)
	}
}

func TestLoadPorkbunCredentials_CompleteKeychainSkipsFile(t *testing.T) {
	withKeychain(t, porkbunCredentials{
		APIKey:       "keychain-api",
		SecretAPIKey: "keychain-secret",
	})

	t.Setenv(porkbunAPIKeyEnv, "")
	t.Setenv(porkbunSecretAPIKeyEnv, "")
	t.Setenv(porkbunCredentialsFilePathEnv, filepath.Join(t.TempDir(), "missing.env"))

	got, err := loadPorkbunCredentials()
	if err != nil {
		t.Fatalf("loadPorkbunCredentials: %v", err)
	}
	if got.APIKey != "keychain-api" {
		t.Fatalf("APIKey=%q, want keychain-api", got.APIKey)
	}
	if got.SecretAPIKey != "keychain-secret" {
		t.Fatalf("SecretAPIKey=%q, want keychain-secret", got.SecretAPIKey)
	}
}

func TestReadPorkbunCredentialsFile_InvalidLine(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "porkbun.env")
	writeCredentialsFile(t, path, "not-valid\n")

	_, err := readPorkbunCredentialsFile(path)
	if err == nil {
		t.Fatal("readPorkbunCredentialsFile: expected error")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("err=%v, want line number", err)
	}
}

func writeCredentialsFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
}

func withoutKeychain(t *testing.T) {
	t.Helper()
	withKeychain(t, porkbunCredentials{})
}

func withKeychain(t *testing.T, creds porkbunCredentials) {
	t.Helper()

	orig := readPorkbunCredentialsFromKeychain
	readPorkbunCredentialsFromKeychain = func() (porkbunCredentials, error) {
		return creds, nil
	}
	t.Cleanup(func() {
		readPorkbunCredentialsFromKeychain = orig
	})
}

func withKeychainError(t *testing.T, err error) {
	t.Helper()

	orig := readPorkbunCredentialsFromKeychain
	readPorkbunCredentialsFromKeychain = func() (porkbunCredentials, error) {
		return porkbunCredentials{}, err
	}
	t.Cleanup(func() {
		readPorkbunCredentialsFromKeychain = orig
	})
}
