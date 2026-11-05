package helper

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	appcore "github.com/alexcatdad/pr-nanny/internal/app"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/notify"
)

const (
	requestTimeout = 5 * time.Second
	// Connecting runs gh or validates a token against GitHub (30 s in app).
	connectTimeout = 35 * time.Second
	maxConnections = 16
	maxSocketPath  = 103 // sizeof(sockaddr_un.sun_path) - 1 on Darwin
)

// Backend is the controller surface exposed over IPC. *app.Controller
// implements it.
type Backend interface {
	GetSnapshot() (model.Snapshot, error)
	SaveSettings(model.Settings) (model.Snapshot, error)
	ConnectPAT(string) (model.Snapshot, error)
	ConnectCLI() (model.Snapshot, error)
	SetBell(string, bool) (model.Snapshot, error)
	SetLayout(string) (model.Snapshot, error)
	Refresh() error
	ResolvePR(string) (string, error)
}

// Notifications is the pull side of notify.Queue.
type Notifications interface {
	Next(cursor string, limit int) notify.Batch
}

// Server answers helper requests on one listener.
type Server struct {
	Backend       Backend
	Notifications Notifications
	Version       string
	Commit        string
	// OnStop runs once after the stop reply is written.
	OnStop func()

	listener  *net.UnixListener
	stopOnce  sync.Once
	authorize func(*net.UnixConn) bool
}

// ErrSocket means the socket could not be created safely.
var ErrSocket = errors.New("the PR Nanny helper socket is unavailable")

// Listen binds a 0600 socket at path. The caller must hold the instance lock
// (storage.LockInstance) for the directory, which makes any existing socket
// stale; a stale socket is removed only if it is a socket owned by this user.
func Listen(path string) (*net.UnixListener, error) {
	if len(path) > maxSocketPath {
		return nil, ErrSocket
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 || !ownedByCurrentUser(info) {
			return nil, ErrSocket
		}
		if os.Remove(path) != nil {
			return nil, ErrSocket
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, ErrSocket
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, ErrSocket
	}
	if os.Chmod(path, 0o600) != nil {
		_ = listener.Close()
		return nil, ErrSocket
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		_ = listener.Close()
		return nil, ErrSocket
	}
	listener.SetUnlinkOnClose(true)
	return listener, nil
}

// Serve accepts connections until ctx ends or the listener closes, then
// waits for in-flight requests and closes the listener.
func (s *Server) Serve(ctx context.Context, listener *net.UnixListener) error {
	if s.Backend == nil || s.Notifications == nil || listener == nil {
		return ErrSocket
	}
	s.listener = listener
	if s.authorize == nil {
		s.authorize = authorizedPeer
	}
	go func() { <-ctx.Done(); _ = listener.Close() }()
	slots := make(chan struct{}, maxConnections)
	var wg sync.WaitGroup
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			break
		}
		select {
		case slots <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			s.serveOne(conn)
		}()
	}
	wg.Wait()
	_ = listener.Close()
	return ctx.Err()
}

func (s *Server) serveOne(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(requestTimeout))
	if !s.authorize(conn) {
		writeReply(conn, Response{Version: ProtocolVersion, Error: wireError(ErrUnauthorizedPeer)})
		return
	}
	frame, err := readFrame(conn)
	if err != nil {
		writeReply(conn, Response{Version: ProtocolVersion, Error: wireError(ErrInvalidRequest)})
		return
	}
	var req Request
	decodeErr := decodeStrict(frame, &req)
	// The frame may carry a token; do not keep it around longer than needed.
	clear(frame)
	if decodeErr != nil || req.ID == "" || len(req.ID) > 128 || req.Op == "" {
		writeReply(conn, Response{Version: ProtocolVersion, Error: wireError(ErrInvalidRequest)})
		return
	}
	if req.Version != ProtocolVersion {
		writeReply(conn, Response{Version: ProtocolVersion, ID: req.ID, Error: wireError(ErrProtocolVersion)})
		return
	}
	if req.Op == OpConnectPAT || req.Op == OpConnectCLI {
		_ = conn.SetDeadline(time.Now().Add(connectTimeout))
	}
	result, err := s.dispatch(req)
	clear(req.Body)
	if err != nil {
		writeReply(conn, Response{Version: ProtocolVersion, ID: req.ID, Error: wireError(codeFor(err))})
		return
	}
	body, err := json.Marshal(result)
	if err != nil {
		writeReply(conn, Response{Version: ProtocolVersion, ID: req.ID, Error: wireError(ErrInternal)})
		return
	}
	writeReply(conn, Response{Version: ProtocolVersion, ID: req.ID, Body: body})
	if req.Op == OpStop {
		s.stopOnce.Do(func() {
			if s.OnStop != nil {
				s.OnStop()
			}
			if s.listener != nil {
				_ = s.listener.Close()
			}
		})
	}
}

var (
	errBadBody   = errors.New("invalid body")
	errUnknownOp = errors.New("unknown operation")
)

func (s *Server) dispatch(req Request) (any, error) {
	switch req.Op {
	case OpSnapshot:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return s.Backend.GetSnapshot()
	case OpSaveSettings:
		var b SaveSettingsBody
		if body(req.Body, &b) != nil {
			return nil, errBadBody
		}
		return s.Backend.SaveSettings(b.Settings)
	case OpConnectPAT:
		var b ConnectPATBody
		if body(req.Body, &b) != nil || b.Token == "" {
			return nil, errBadBody
		}
		return s.Backend.ConnectPAT(b.Token)
	case OpConnectCLI:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return s.Backend.ConnectCLI()
	case OpSetBell:
		var b SetBellBody
		if body(req.Body, &b) != nil {
			return nil, errBadBody
		}
		return s.Backend.SetBell(b.ID, b.Enabled)
	case OpSetLayout:
		var b SetLayoutBody
		if body(req.Body, &b) != nil {
			return nil, errBadBody
		}
		return s.Backend.SetLayout(b.Layout)
	case OpRefresh:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return struct{}{}, s.Backend.Refresh()
	case OpResolvePR:
		var b ResolvePRBody
		if body(req.Body, &b) != nil {
			return nil, errBadBody
		}
		url, err := s.Backend.ResolvePR(b.ID)
		if err != nil {
			return nil, err
		}
		// Defense in depth: never hand the app a non-canonical URL.
		if !appcore.ValidPRURL(url) {
			return nil, appcore.ErrUnavailable
		}
		return ResolvePRReply{URL: url}, nil
	case OpNextNotifications:
		var b NextNotificationsBody
		if body(req.Body, &b) != nil || len(b.Cursor) > 64 || b.Limit < 0 || b.Limit > notify.MaxBatch {
			return nil, errBadBody
		}
		return s.Notifications.Next(b.Cursor, b.Limit), nil
	case OpHealth:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return HealthReply{Status: "ok"}, nil
	case OpVersion:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return VersionReply{Version: s.Version, Commit: s.Commit, Protocol: ProtocolVersion}, nil
	case OpStop:
		if body(req.Body, &struct{}{}) != nil {
			return nil, errBadBody
		}
		return struct{}{}, nil
	}
	return nil, errUnknownOp
}

// codeFor maps errors to stable codes. Only the controller's fixed,
// user-safe errors carry their own message; anything else is INTERNAL.
func codeFor(err error) ErrorCode {
	switch {
	case errors.Is(err, errBadBody):
		return ErrInvalidRequest
	case errors.Is(err, errUnknownOp):
		return ErrUnknownOperation
	case errors.Is(err, appcore.ErrSettings):
		return ErrSettings
	case errors.Is(err, appcore.ErrAuthentication):
		return ErrAuthentication
	case errors.Is(err, appcore.ErrCredentialStore):
		return ErrCredentialStore
	case errors.Is(err, appcore.ErrUnavailable):
		return ErrUnavailable
	}
	return ErrInternal
}

var messages = map[ErrorCode]string{
	ErrInvalidRequest:   "The request is invalid.",
	ErrProtocolVersion:  "The PR Nanny helper protocol version is unsupported.",
	ErrUnknownOperation: "The PR Nanny helper does not support that operation.",
	ErrUnauthorizedPeer: "The PR Nanny helper only accepts requests from the current user.",
	ErrSettings:         appcore.ErrSettings.Error(),
	ErrAuthentication:   appcore.ErrAuthentication.Error(),
	ErrCredentialStore:  appcore.ErrCredentialStore.Error(),
	ErrUnavailable:      appcore.ErrUnavailable.Error(),
	ErrInternal:         "The PR Nanny helper could not complete the request.",
}

func wireError(code ErrorCode) *WireError {
	return &WireError{Code: code, Message: messages[code]}
}

func body(data json.RawMessage, out any) error {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		data = json.RawMessage(`{}`)
	}
	return decodeStrict(data, out)
}

func decodeStrict(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
}

func writeReply(w io.Writer, r Response) {
	data, err := json.Marshal(r)
	if err == nil {
		_ = writeFrame(w, data)
	}
}

func writeFrame(w io.Writer, data []byte) error {
	if len(data) == 0 || len(data) > MaxFrameBytes {
		return errBadBody
	}
	frame := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(frame, uint32(len(data))) // #nosec G115 -- bounded by MaxFrameBytes.
	copy(frame[4:], data)
	_, err := w.Write(frame)
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > MaxFrameBytes {
		return nil, errBadBody
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}
