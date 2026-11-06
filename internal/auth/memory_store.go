package auth

import "sync"

// MemoryStore keeps tokens in process memory only. The helper uses it for
// PATs: the menu-bar app owns the persistent copy in its keychain and hands
// the token over the helper socket at startup, so nothing here touches disk.
type MemoryStore struct {
	mu     sync.Mutex
	tokens map[string]string
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{tokens: map[string]string{}} }

func memoryKey(service, account string) string { return service + "\x00" + account }

func (s *MemoryStore) Get(service, account string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.tokens[memoryKey(service, account)]
	if !ok {
		return "", ErrNoCredential
	}
	return token, nil
}

func (s *MemoryStore) Set(service, account, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[memoryKey(service, account)] = secret
	return nil
}

func (s *MemoryStore) Delete(service, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, memoryKey(service, account))
	return nil
}
