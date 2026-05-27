package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	porkbunAPIKeyEnv              = "PORKBUN_API_KEY"
	porkbunSecretAPIKeyEnv        = "PORKBUN_SECRET_API_KEY"
	porkbunCredentialsFilePathEnv = "DOTHUNTCLI_PORKBUN_CREDENTIALS_FILE"

	porkbunKeychainService             = "dothuntcli/porkbun"
	porkbunAPIKeyKeychainAccount       = "api-key"
	porkbunSecretAPIKeyKeychainAccount = "secret-api-key"
)

type porkbunCredentials struct {
	APIKey       string
	SecretAPIKey string
}

var readPorkbunCredentialsFromKeychain = readPorkbunCredentialsFromKeychainOS

func (creds porkbunCredentials) complete() bool {
	return creds.APIKey != "" && creds.SecretAPIKey != ""
}

func loadPorkbunCredentials() (porkbunCredentials, error) {
	creds := porkbunCredentials{
		APIKey:       strings.TrimSpace(os.Getenv(porkbunAPIKeyEnv)),
		SecretAPIKey: strings.TrimSpace(os.Getenv(porkbunSecretAPIKeyEnv)),
	}
	if creds.complete() {
		return creds, nil
	}

	keychainCreds, keychainErr := readPorkbunCredentialsFromKeychain()
	if keychainErr == nil {
		if creds.APIKey == "" {
			creds.APIKey = keychainCreds.APIKey
		}
		if creds.SecretAPIKey == "" {
			creds.SecretAPIKey = keychainCreds.SecretAPIKey
		}
		if creds.complete() {
			return creds, nil
		}
	}

	path, explicit := porkbunCredentialsFilePath()
	if path == "" {
		if keychainErr != nil {
			return creds, keychainErr
		}
		return creds, nil
	}

	fileCreds, err := readPorkbunCredentialsFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			if keychainErr != nil {
				return creds, keychainErr
			}
			return creds, nil
		}
		if os.IsNotExist(err) {
			return creds, fmt.Errorf("Porkbun credentials file does not exist: %s", path)
		}
		return creds, err
	}

	if creds.APIKey == "" {
		creds.APIKey = fileCreds.APIKey
	}
	if creds.SecretAPIKey == "" {
		creds.SecretAPIKey = fileCreds.SecretAPIKey
	}
	if keychainErr != nil && !creds.complete() {
		return creds, keychainErr
	}
	return creds, nil
}

func porkbunCredentialsFilePath() (path string, explicit bool) {
	if path := strings.TrimSpace(os.Getenv(porkbunCredentialsFilePathEnv)); path != "" {
		return path, true
	}

	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		return "", false
	}
	return filepath.Join(configDir, "dothuntcli", "porkbun.env"), false
}

func porkbunCredentialsFileHint() string {
	path, _ := porkbunCredentialsFilePath()
	if path == "" {
		return "the dothuntcli Porkbun credentials file"
	}
	return path
}

func porkbunCredentialsHint() string {
	return fmt.Sprintf("set %s and %s, add them to macOS Keychain service %q, or add them to %s", porkbunAPIKeyEnv, porkbunSecretAPIKeyEnv, porkbunKeychainService, porkbunCredentialsFileHint())
}

func readPorkbunCredentialsFile(path string) (porkbunCredentials, error) {
	var creds porkbunCredentials

	f, err := os.Open(path)
	if err != nil {
		return creds, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return creds, fmt.Errorf("invalid Porkbun credentials line %d in %s", lineNo, path)
		}

		key = strings.TrimSpace(key)
		val = cleanCredentialValue(strings.TrimSpace(val))
		switch key {
		case porkbunAPIKeyEnv:
			creds.APIKey = val
		case porkbunSecretAPIKeyEnv:
			creds.SecretAPIKey = val
		}
	}
	if err := sc.Err(); err != nil {
		return creds, fmt.Errorf("read Porkbun credentials file %s: %w", path, err)
	}
	return creds, nil
}

func cleanCredentialValue(val string) string {
	if len(val) < 2 {
		return val
	}
	first, last := val[0], val[len(val)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return val[1 : len(val)-1]
	}
	return val
}
