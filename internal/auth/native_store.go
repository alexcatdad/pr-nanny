package auth

import "github.com/zalando/go-keyring"

// NativeStore delegates secret storage to the macOS Keychain, Windows
// Credential Manager, or Linux Secret Service. Errors never trigger fallback.
type NativeStore struct{}

func (NativeStore) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (NativeStore) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (NativeStore) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if err == keyring.ErrNotFound {
		return nil
	}
	return err
}
