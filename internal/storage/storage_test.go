package storage

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

func TestAtomicPrivateRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s := New(dir)
	settings, err := s.Settings()
	if err != nil || settings.IntervalSeconds != 60 {
		t.Fatalf("default settings = %#v, %v", settings, err)
	}
	settings.Account = "synthetic@example.test"
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	state := State{Bells: map[string]bool{"pr-1": true}, Baseline: map[string]Observed{"pr-1": {CI: model.CIPassed, ReviewRequested: true}}, Initialized: true}
	if err := s.SaveState(state); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadState()
	if err != nil || !got.Bells["pr-1"] || got.Baseline["pr-1"].CI != model.CIPassed || !got.Initialized {
		t.Fatalf("state round trip = %#v, %v", got, err)
	}
	for _, name := range []string{"settings.json", "state.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s permissions = %o", name, info.Mode().Perm())
		}
	}
}

func TestCorruptAndUnsupportedVersionAreSafeErrors(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"version":99,"payload":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.LoadState()
	if !errors.Is(err, ErrCorrupt) || err.Error() != ErrCorrupt.Error() {
		t.Fatalf("error = %v", err)
	}
}

func TestMissingStateDefaultsBellsOff(t *testing.T) {
	s := New(t.TempDir())
	state, err := s.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Bells == nil || state.Bells["unknown"] {
		t.Fatalf("bells should default off: %#v", state.Bells)
	}
}

func TestRejectsInvalidSettingsAndOversizedState(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	invalid := model.DefaultSettings()
	invalid.IntervalSeconds = 17
	if err := s.SaveSettings(invalid); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("invalid settings save error=%v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), make([]byte, maxFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadState(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("oversized state error=%v", err)
	}
}

func TestRejectsInvalidReviewRequestIdentitySets(t *testing.T) {
	s := New(t.TempDir())
	for name, ids := range map[string][]string{
		"empty":           {""},
		"oversized":       {strings.Repeat("x", maxReviewRequestIDBytes+1)},
		"too many for PR": makeIDs(maxReviewRequestIDs + 1),
	} {
		err := s.SaveState(State{Bells: map[string]bool{}, Baseline: map[string]Observed{"pr": {ReviewRequestIDs: ids}}})
		if !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s IDs save error=%v", name, err)
		}
	}
	tooManyAcrossState := map[string]Observed{}
	for i := 0; i < maxTrackedPRs; i++ {
		ids := make([]string, 11)
		for j := range ids {
			ids[j] = "request-" + strconv.Itoa(j)
		}
		tooManyAcrossState["pr-"+strconv.Itoa(i)] = Observed{ReviewRequestIDs: ids}
	}
	if err := s.SaveState(State{Bells: map[string]bool{}, Baseline: tooManyAcrossState}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("aggregate ID count save error=%v", err)
	}
	tooManyBytes := map[string]Observed{}
	for i := 0; i < maxTrackedPRs; i++ {
		tooManyBytes["pr-"+strconv.Itoa(i)] = Observed{ReviewRequestIDs: []string{strings.Repeat("x", 220)}}
	}
	if err := s.SaveState(State{Bells: map[string]bool{}, Baseline: tooManyBytes}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("aggregate ID bytes save error=%v", err)
	}
}

func makeIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = "request-id"
	}
	return ids
}

func TestQuarantinePreservesCorruptFile(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadState(); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corruption error=%v", err)
	}
	if err := s.QuarantineCorrupt("state.json"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "state.json.corrupt.") {
		t.Fatalf("quarantine entries=%v", entries)
	}
}

func TestOpenRootRejectsSettingsSymlinkOutsideDataDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges not guaranteed on Windows CI")
	}
	root := filepath.Join(t.TempDir(), "private")
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.Mkdir(root, 0o700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(outside, []byte(`{"version":1,"payload":{"Version":1,"AuthMethod":"gh","IntervalSeconds":60,"Layout":"all"}}`), 0o600); err != nil { t.Fatal(err) }
	if err := os.Symlink(outside, filepath.Join(root, "settings.json")); err != nil { t.Fatal(err) }
	s := New(root)
	if got, err := s.Settings(); err == nil {
		t.Fatalf("outside symlink was read: %#v", got)
	}
	if err := s.SaveSettings(model.DefaultSettings()); err != nil { t.Fatalf("safe replacement failed: %v", err) }
	if contents, err := os.ReadFile(outside); err != nil || !strings.Contains(string(contents), `"AuthMethod":"gh"`) {
		t.Fatalf("outside file was modified: %q, %v", contents, err)
	}
}
