// Package storage provides private, versioned JSON persistence for user settings
// and the small amount of state needed to avoid duplicate notifications.
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

const schemaVersion = 1
const maxFileBytes = 8 << 20
const maxTrackedPRs = 10000

var ErrCorrupt = errors.New("stored application data is invalid")

// State contains only non-secret settings and notification deduplication data.
// Bell entries are deliberately separate from GitHub data and default to false.
type State struct {
	Account     string              `json:"account"`
	Bells       map[string]bool     `json:"bells"`
	Baseline    map[string]Observed `json:"baseline"`
	Initialized bool                `json:"initialized"`
}

// Observed is the last successfully observed notification-relevant PR state.
type Observed struct {
	CI              model.CI   `json:"ci"`
	Approved        bool       `json:"approved"`
	ReviewRequested bool       `json:"reviewRequested"`
	Merged          bool       `json:"merged"`
	Closed          bool       `json:"closed"`
	ClosedAt        *time.Time `json:"closedAt,omitempty"`
}

type envelope struct {
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store { return &Store{dir: dir} }

func (s *Store) Settings() (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var settings model.Settings
	if err := s.read("settings.json", &settings); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.DefaultSettings(), nil
		}
		return model.Settings{}, err
	}
	settings = normalizeSettings(settings)
	if err := validateSettings(settings); err != nil {
		return model.Settings{}, ErrCorrupt
	}
	return settings, nil
}

func (s *Store) SaveSettings(settings model.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings = normalizeSettings(settings)
	if err := validateSettings(settings); err != nil {
		return err
	}
	return s.write("settings.json", settings)
}

func (s *Store) LoadState() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state State
	if err := s.read("state.json", &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Bells: map[string]bool{}, Baseline: map[string]Observed{}}, nil
		}
		return State{}, err
	}
	if state.Bells == nil {
		state.Bells = map[string]bool{}
	}
	if state.Baseline == nil {
		state.Baseline = map[string]Observed{}
	}
	if err := validateState(state); err != nil {
		return State{}, ErrCorrupt
	}
	return state, nil
}

func (s *Store) SaveState(state State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Bells == nil {
		state.Bells = map[string]bool{}
	}
	if state.Baseline == nil {
		state.Baseline = map[string]Observed{}
	}
	if err := validateState(state); err != nil {
		return err
	}
	return s.write("state.json", state)
}

func (s *Store) read(name string, dst any) error {
	f, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return ErrCorrupt
	}
	if len(b) > maxFileBytes {
		return ErrCorrupt
	}
	var e envelope
	if err := json.Unmarshal(b, &e); err != nil || e.Version != schemaVersion || len(e.Payload) == 0 || strings.EqualFold(string(e.Payload), "null") {
		return ErrCorrupt
	}
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return ErrCorrupt
	}
	return nil
}

// QuarantineCorrupt preserves an invalid recognized data file under a new name.
// Callers can then report the recovery and load clean defaults without deleting evidence.
func (s *Store) QuarantineCorrupt(name string) error {
	if name != "settings.json" && name != "state.json" {
		return errors.New("unsupported data file")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	source := filepath.Join(s.dir, name)
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Rename(source, filepath.Join(s.dir, name+".corrupt."+stamp))
}

func (s *Store) write(name string, value any) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create private data directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("protect private data directory: %w", err)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode application data: %w", err)
	}
	b, err := json.Marshal(envelope{Version: schemaVersion, Payload: payload})
	if err != nil {
		return fmt.Errorf("encode application data: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".pr-nanny-*.tmp")
	if err != nil {
		return fmt.Errorf("write application data: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write application data: %w", err)
	}
	if err := replaceFile(tmpName, filepath.Join(s.dir, name)); err != nil {
		return fmt.Errorf("replace application data: %w", err)
	}
	return nil
}

func normalizeSettings(s model.Settings) model.Settings {
	d := model.DefaultSettings()
	if s.Version == 0 {
		s.Version = d.Version
	}
	if s.AuthMethod == "" {
		s.AuthMethod = d.AuthMethod
	}
	if s.IntervalSeconds == 0 {
		s.IntervalSeconds = d.IntervalSeconds
	}
	if s.Layout == "" {
		s.Layout = d.Layout
	}
	return s
}

func validateSettings(s model.Settings) error {
	if s.Version != 1 {
		return ErrCorrupt
	}
	if s.AuthMethod != "gh" && s.AuthMethod != "pat" {
		return ErrCorrupt
	}
	if s.IntervalSeconds != 30 && s.IntervalSeconds != 60 && s.IntervalSeconds != 120 && s.IntervalSeconds != 300 {
		return ErrCorrupt
	}
	if s.Layout != "all" && s.Layout != "tabs" {
		return ErrCorrupt
	}
	if len(s.Account) > 256 || strings.ContainsAny(s.Account, "\r\n\x00") {
		return ErrCorrupt
	}
	return nil
}

func validateState(s State) error {
	if len(s.Account) > 256 || strings.ContainsAny(s.Account, "\r\n\x00") || len(s.Bells) > maxTrackedPRs || len(s.Baseline) > maxTrackedPRs {
		return ErrCorrupt
	}
	for id := range s.Bells {
		if id == "" || len(id) > 512 {
			return ErrCorrupt
		}
	}
	for id, o := range s.Baseline {
		if id == "" || len(id) > 512 {
			return ErrCorrupt
		}
		if o.CI != "" && o.CI != model.CIPassed && o.CI != model.CINotPassed && o.CI != model.CIRunning && o.CI != model.CINotAvailable {
			return ErrCorrupt
		}
		if o.ClosedAt != nil && o.ClosedAt.IsZero() {
			return ErrCorrupt
		}
	}
	return nil
}
