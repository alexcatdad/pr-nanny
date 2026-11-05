//go:build darwin

package helper

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appcore "github.com/alexcatdad/pr-nanny/internal/app"
	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/notify"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

const syntheticToken = "github_pat_synthetic_fixture_value"

type fakeBackend struct {
	mu       sync.Mutex
	calls    []string
	snapshot model.Snapshot
	url      string
	err      error
}

func (f *fakeBackend) record(call string) (model.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.snapshot, f.err
}
func (f *fakeBackend) GetSnapshot() (model.Snapshot, error) { return f.record("snapshot") }
func (f *fakeBackend) SaveSettings(s model.Settings) (model.Snapshot, error) {
	return f.record("saveSettings:" + s.Layout)
}
func (f *fakeBackend) ConnectPAT(token string) (model.Snapshot, error) {
	if token != syntheticToken {
		return model.Snapshot{}, appcore.ErrAuthentication
	}
	return f.record("connectPAT")
}
func (f *fakeBackend) ConnectCLI() (model.Snapshot, error) { return f.record("connectCLI") }
func (f *fakeBackend) SetBell(id string, on bool) (model.Snapshot, error) {
	if on {
		return f.record("setBell:" + id + ":on")
	}
	return f.record("setBell:" + id + ":off")
}
func (f *fakeBackend) SetLayout(layout string) (model.Snapshot, error) {
	return f.record("setLayout:" + layout)
}
func (f *fakeBackend) Refresh() error { _, err := f.record("refresh"); return err }
func (f *fakeBackend) ResolvePR(id string) (string, error) {
	_, err := f.record("resolvePR:" + id)
	return f.url, err
}

type harness struct {
	socket  string
	backend *fakeBackend
	queue   *notify.Queue
	stopped chan struct{}
	done    chan struct{}
}

func shortDir(t *testing.T) string {
	t.Helper()
	// t.TempDir can exceed the 104-byte sun_path limit on macOS.
	dir, err := os.MkdirTemp("/tmp", "pn-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func start(t *testing.T, authorize func(*net.UnixConn) bool) *harness {
	t.Helper()
	dir := shortDir(t)
	lock, err := storage.LockInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	h := &harness{socket: filepath.Join(dir, SocketName), backend: &fakeBackend{}, queue: notify.NewQueue(), stopped: make(chan struct{}), done: make(chan struct{})}
	listener, err := Listen(h.socket)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	server := &Server{Backend: h.backend, Notifications: h.queue, Version: "2026.11.05.1", Commit: "abc123", OnStop: func() { once.Do(func() { close(h.stopped) }) }, authorize: authorize}
	go func() { _ = server.Serve(ctx, listener); close(h.done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(2 * time.Second):
			t.Error("server did not stop")
		}
	})
	return h
}

func (h *harness) raw(t *testing.T, payload []byte) Response {
	t.Helper()
	conn, err := net.Dial("unix", h.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if payload != nil {
		if err := writeFrame(conn, payload); err != nil {
			t.Fatal(err)
		}
	}
	reply, err := readFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := json.Unmarshal(reply, &resp); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reply), syntheticToken) {
		t.Fatalf("reply contains the token: %s", reply)
	}
	return resp
}

func (h *harness) call(t *testing.T, op Op, body any) Response {
	t.Helper()
	req := map[string]any{"version": ProtocolVersion, "id": "req-1", "op": op}
	if body != nil {
		req["body"] = body
	}
	payload, _ := json.Marshal(req)
	resp := h.raw(t, payload)
	if resp.Version != ProtocolVersion || resp.ID != "req-1" {
		t.Fatalf("envelope = %+v", resp)
	}
	return resp
}

func TestSocketIsPrivate(t *testing.T) {
	h := start(t, nil)
	info, err := os.Lstat(h.socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket = %v, %v", info, err)
	}
	dir, err := os.Stat(filepath.Dir(h.socket))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("socket directory = %v, %v", dir, err)
	}
}

func TestOperationsReachBackend(t *testing.T) {
	h := start(t, nil)
	h.backend.url = "https://github.com/acme/widgets/pull/7"
	steps := []struct {
		op   Op
		body any
	}{
		{OpSnapshot, nil},
		{OpSaveSettings, SaveSettingsBody{Settings: model.Settings{Layout: "tabs"}}},
		{OpConnectPAT, ConnectPATBody{Token: syntheticToken}},
		{OpConnectCLI, map[string]any{}},
		{OpSetBell, SetBellBody{ID: "PR_1", Enabled: true}},
		{OpSetLayout, SetLayoutBody{Layout: "all"}},
		{OpRefresh, nil},
		{OpResolvePR, ResolvePRBody{ID: "PR_1"}},
	}
	for _, step := range steps {
		if resp := h.call(t, step.op, step.body); resp.Error != nil || len(resp.Body) == 0 {
			t.Fatalf("%s: %+v", step.op, resp)
		}
	}
	want := "snapshot saveSettings:tabs connectPAT connectCLI setBell:PR_1:on setLayout:all refresh resolvePR:PR_1"
	if got := strings.Join(h.backend.calls, " "); got != want {
		t.Fatalf("calls = %s", got)
	}
	var resolved ResolvePRReply
	_ = json.Unmarshal(h.call(t, OpResolvePR, ResolvePRBody{ID: "PR_1"}).Body, &resolved)
	if resolved.URL != "https://github.com/acme/widgets/pull/7" {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolvePRRejectsNonCanonicalURLs(t *testing.T) {
	h := start(t, nil)
	h.backend.url = "https://github.com.evil.invalid/acme/widgets/pull/7"
	if resp := h.call(t, OpResolvePR, ResolvePRBody{ID: "PR_1"}); resp.Error == nil || resp.Error.Code != ErrUnavailable || len(resp.Body) != 0 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestControllerErrorsUseFixedMessages(t *testing.T) {
	h := start(t, nil)
	resp := h.call(t, OpConnectPAT, ConnectPATBody{Token: "wrong-synthetic"})
	if resp.Error == nil || resp.Error.Code != ErrAuthentication || resp.Error.Message != appcore.ErrAuthentication.Error() {
		t.Fatalf("resp = %+v", resp)
	}
	h.backend.err = errors.New("private upstream detail " + syntheticToken)
	resp = h.call(t, OpSnapshot, nil)
	if resp.Error == nil || resp.Error.Code != ErrInternal || strings.Contains(resp.Error.Message, "private") {
		t.Fatalf("resp = %+v", resp)
	}
	h.backend.err = appcore.ErrCredentialStore
	if resp = h.call(t, OpRefresh, nil); resp.Error == nil || resp.Error.Code != ErrCredentialStore {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestInvalidRequestsAreRejected(t *testing.T) {
	h := start(t, nil)
	for name, payload := range map[string]string{
		"not json":      `{`,
		"no id":         `{"version":1,"op":"snapshot"}`,
		"unknown field": `{"version":1,"id":"x","op":"snapshot","extra":true}`,
		"trailing":      `{"version":1,"id":"x","op":"snapshot"} {}`,
	} {
		if resp := h.raw(t, []byte(payload)); resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
			t.Fatalf("%s: %+v", name, resp)
		}
	}
	if resp := h.raw(t, []byte(`{"version":2,"id":"x","op":"snapshot"}`)); resp.Error == nil || resp.Error.Code != ErrProtocolVersion || resp.ID != "x" {
		t.Fatalf("version: %+v", resp)
	}
	if resp := h.call(t, Op("deleteRepository"), nil); resp.Error == nil || resp.Error.Code != ErrUnknownOperation {
		t.Fatalf("unknown op: %+v", resp)
	}
	for op, body := range map[Op]any{
		OpSnapshot:          map[string]any{"unexpected": 1},
		OpConnectPAT:        ConnectPATBody{},
		OpSetBell:           map[string]any{"id": 5},
		OpNextNotifications: NextNotificationsBody{Limit: notify.MaxBatch + 1},
	} {
		if resp := h.call(t, op, body); resp.Error == nil || resp.Error.Code != ErrInvalidRequest {
			t.Fatalf("%s bad body: %+v", op, resp)
		}
	}
	if len(h.backend.calls) != 0 {
		t.Fatalf("invalid requests reached backend: %v", h.backend.calls)
	}
}

func TestOversizedFrameIsRejected(t *testing.T) {
	h := start(t, nil)
	conn, err := net.Dial("unix", h.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxFrameBytes+1)
	_, _ = conn.Write(header[:])
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	reply, err := readFrame(conn)
	if err != nil || !strings.Contains(string(reply), string(ErrInvalidRequest)) {
		t.Fatalf("reply = %s, %v", reply, err)
	}
}

func TestUnauthorizedPeerIsRejectedBeforeReading(t *testing.T) {
	h := start(t, func(*net.UnixConn) bool { return false })
	if resp := h.raw(t, nil); resp.Error == nil || resp.Error.Code != ErrUnauthorizedPeer {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestCurrentUserPeerIsAuthorized(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, "p.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, err := net.Dial("unix", path)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			_ = conn.Close()
		}
	}()
	conn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if !authorizedPeer(conn) {
		t.Fatal("same-user peer rejected")
	}
}

func TestHealthVersionAndStop(t *testing.T) {
	h := start(t, nil)
	var health HealthReply
	_ = json.Unmarshal(h.call(t, OpHealth, nil).Body, &health)
	var version VersionReply
	_ = json.Unmarshal(h.call(t, OpVersion, nil).Body, &version)
	if health.Status != "ok" || version != (VersionReply{Version: "2026.11.05.1", Commit: "abc123", Protocol: ProtocolVersion}) {
		t.Fatalf("health=%+v version=%+v", health, version)
	}
	if resp := h.call(t, OpStop, nil); resp.Error != nil {
		t.Fatalf("stop: %+v", resp)
	}
	select {
	case <-h.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("OnStop not called")
	}
	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server kept serving after stop")
	}
	if _, err := os.Lstat(h.socket); !os.IsNotExist(err) {
		t.Fatalf("socket not removed after stop: %v", err)
	}
}

func TestNextNotificationsUsesQueueCursor(t *testing.T) {
	h := start(t, nil)
	h.queue.SetFilter(func(id string, _ core.Event) bool { return id != "PR_MUTED" })
	h.queue.Notify(model.PullRequest{ID: "PR_1", Repository: "acme/widgets", Number: 1, Title: "One"}, core.EventReviewRequested)
	h.queue.Notify(model.PullRequest{ID: "PR_MUTED", Repository: "acme/widgets", Number: 2, Title: "Two"}, core.EventCI)
	var first notify.Batch
	_ = json.Unmarshal(h.call(t, OpNextNotifications, NextNotificationsBody{}).Body, &first)
	if len(first.Notifications) != 1 || first.Notifications[0].PRID != "PR_1" || first.Cursor == "" {
		t.Fatalf("first = %+v", first)
	}
	var second notify.Batch
	_ = json.Unmarshal(h.call(t, OpNextNotifications, NextNotificationsBody{Cursor: first.Cursor}).Body, &second)
	if len(second.Notifications) != 0 || second.Cursor != first.Cursor {
		t.Fatalf("second = %+v", second)
	}
}

func TestListenRefusesNonSocketAndReplacesStaleSocket(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, SocketName)
	if err := os.WriteFile(path, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path); !errors.Is(err, ErrSocket) {
		t.Fatalf("regular file accepted: %v", err)
	}
	_ = os.Remove(path)
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	_ = stale.Close()
	listener, err := Listen(path)
	if err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	}
	_ = listener.Close()
	if _, err := Listen(filepath.Join(dir, strings.Repeat("x", maxSocketPath))); !errors.Is(err, ErrSocket) {
		t.Fatalf("overlong path accepted: %v", err)
	}
}

func TestWatchProcessClosesWhenProcessExits(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "0.2")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	gone := WatchProcess(cmd.Process.Pid)
	go func() { _ = cmd.Wait() }()
	select {
	case <-gone:
	case <-time.After(3 * time.Second):
		t.Fatal("exit not observed")
	}
	select {
	case <-WatchProcess(cmd.Process.Pid):
	case <-time.After(time.Second):
		t.Fatal("already-exited process not reported")
	}
}
