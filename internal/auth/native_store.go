package auth

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// NativeStore reads and removes the login-keychain item that PR Nanny
// 0.1.x wrote through go-keyring (service "pr-nanny-github"). That item's
// ACL trusts /usr/bin/security, so any process running as the user can read
// it. It is used only to migrate the token into the app's own keychain item
// and then delete it; new tokens are never written here.
type NativeStore struct{}

func (NativeStore) Get(service, account string) (string, error) {
	token, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoCredential
	}
	return token, err
}

func (NativeStore) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (NativeStore) Delete(service, account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
