//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func readPorkbunCredentialsFromKeychainOS() (porkbunCredentials, error) {
	var creds porkbunCredentials

	apiKey, err := readGenericPasswordFromKeychain(porkbunKeychainService, porkbunAPIKeyKeychainAccount)
	if err != nil {
		return creds, err
	}
	secretAPIKey, err := readGenericPasswordFromKeychain(porkbunKeychainService, porkbunSecretAPIKeyKeychainAccount)
	if err != nil {
		return creds, err
	}

	creds.APIKey = apiKey
	creds.SecretAPIKey = secretAPIKey
	return creds, nil
}

func readGenericPasswordFromKeychain(service, account string) (string, error) {
	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-s", service, "-a", account, "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if isMissingKeychainItem(msg) {
			return "", nil
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("read macOS Keychain item %s/%s: %s", service, account, msg)
	}

	return strings.TrimSpace(string(out)), nil
}

func isMissingKeychainItem(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "could not be found") || strings.Contains(msg, "not found")
}
