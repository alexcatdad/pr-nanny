package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestNativeStoreRoundTrip is opt-in because a real OS vault may prompt for
// access. CI enables it only after provisioning an isolated unlocked vault.
func TestNativeStoreRoundTrip(t *testing.T) {
	if os.Getenv("PR_NANNY_TEST_NATIVE_VAULT") != "1" {
		t.Skip("native credential vault test disabled; set PR_NANNY_TEST_NATIVE_VAULT=1 with an isolated unlocked test vault")
	}
	service := "pr-nanny-test-" + randomTestID(t)
	account := "synthetic-" + randomTestID(t)
	first := "synthetic-secret-" + randomTestID(t)
	updated := "synthetic-secret-" + randomTestID(t)
	store := NativeStore{}
	cleanup := func() {
		if err := store.Delete(service, account); err != nil {
			t.Error("could not remove synthetic test credential")
			return
		}
		if _, err := store.Get(service, account); !errors.Is(err, keyring.ErrNotFound) {
			t.Error("synthetic test credential remained after cleanup")
		}
	}
	t.Cleanup(cleanup)
	if err := store.Set(service, account, first); err != nil {
		t.Fatal("native credential store write failed")
	}
	got, err := store.Get(service, account)
	if err != nil || got != first {
		t.Fatal("native credential store readback failed")
	}
	if err := store.Set(service, account, updated); err != nil {
		t.Fatal("native credential store update failed")
	}
	got, err = store.Get(service, account)
	if err != nil || got != updated {
		t.Fatal("native credential store updated readback failed")
	}
	if err := store.Delete(service, account); err != nil {
		t.Fatal("native credential store delete failed")
	}
	if _, err := store.Get(service, account); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatal("deleted synthetic credential was still available")
	}
}

func randomTestID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatal("could not generate synthetic test identifier")
	}
	return hex.EncodeToString(value[:])
}
