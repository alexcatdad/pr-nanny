package auth

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryStoreRoundTrip(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Get(service, "octocat"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("empty get = %v", err)
	}
	if err := s.Set(service, "octocat", "synthetic"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(service, "octocat"); err != nil || got != "synthetic" {
		t.Fatalf("get = %q %v", got, err)
	}
	if _, err := s.Get(service, "hubot"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("other account = %v", err)
	}
	if err := s.Delete(service, "octocat"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(service, "octocat"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("after delete = %v", err)
	}
}

func TestProvideLoadsTokenForLoad(t *testing.T) {
	token := "synthetic-token-123456789"
	m := NewManager(NewMemoryStore(), validatorFunc(func(context.Context, string) (string, error) { return "octocat", nil }), nil)
	if err := m.Provide("octocat", "short"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("short = %v", err)
	}
	if err := m.Provide("-bad", token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("bad login = %v", err)
	}
	if err := m.Provide("octocat", token); err != nil {
		t.Fatal(err)
	}
	got, login, err := m.Load(context.Background(), MethodPAT, "octocat")
	if err != nil || got != token || login != "octocat" {
		t.Fatalf("load = %q %q %v", got, login, err)
	}
	if err := NewManager(&memoryStore{setErr: errors.New("locked")}, nil, nil).Provide("octocat", token); !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("store failure = %v", err)
	}
	if err := (*Manager)(nil).Provide("octocat", token); !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("nil manager = %v", err)
	}
}

func TestLegacyTokenMovesIntoMemory(t *testing.T) {
	want := "not-a-real-legacy-value"
	legacy := &memoryStore{value: want, account: "octocat"}
	memory := NewMemoryStore()
	m := NewManager(memory, nil, nil).WithLegacyStore(legacy)
	got, err := m.LegacyToken("octocat")
	if err != nil || got != want {
		t.Fatalf("legacy = %q %v", got, err)
	}
	if loaded, err := memory.Get(service, "octocat"); err != nil || loaded != want {
		t.Fatalf("memory = %q %v", loaded, err)
	}
	if _, err := m.LegacyToken("hubot"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("missing = %v", err)
	}
	if _, err := NewManager(memory, nil, nil).LegacyToken("octocat"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("no legacy store = %v", err)
	}
	broken := NewManager(memory, nil, nil).WithLegacyStore(failingStore{})
	if _, err := broken.LegacyToken("octocat"); !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("locked = %v", err)
	}
	if err := broken.DeleteLegacy("octocat"); !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("delete locked = %v", err)
	}
	if err := m.DeleteLegacy("octocat"); err != nil || legacy.value != "" {
		t.Fatalf("delete = %v, %q", err, legacy.value)
	}
	if err := m.DeleteLegacy("-bad"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("bad login = %v", err)
	}
	if err := NewManager(memory, nil, nil).DeleteLegacy("octocat"); !errors.Is(err, ErrCredentialStore) {
		t.Fatalf("no legacy store delete = %v", err)
	}
	short := NewManager(memory, nil, nil).WithLegacyStore(&memoryStore{value: "short", account: "octocat"})
	if _, err := short.LegacyToken("octocat"); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("malformed = %v", err)
	}
}

type failingStore struct{}

func (failingStore) Get(string, string) (string, error) { return "", errors.New("locked") }
func (failingStore) Set(string, string, string) error   { return errors.New("locked") }
func (failingStore) Delete(string, string) error        { return errors.New("locked") }
