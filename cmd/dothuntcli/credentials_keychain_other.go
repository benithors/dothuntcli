//go:build !darwin

package main

func readPorkbunCredentialsFromKeychainOS() (porkbunCredentials, error) {
	return porkbunCredentials{}, nil
}
