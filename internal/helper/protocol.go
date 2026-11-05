// Package helper serves the PR Nanny backend to the menu-bar app over a
// private unix socket. One request per connection; each frame is a 4-byte
// big-endian length followed by a JSON envelope.
package helper

import (
	"encoding/json"

	"github.com/alexcatdad/pr-nanny/internal/model"
)

const (
	// ProtocolVersion is bumped on any incompatible envelope or body change.
	ProtocolVersion = 1
	// MaxFrameBytes bounds requests and replies.
	MaxFrameBytes = 1 << 20
	// SocketName is created inside the private data directory.
	SocketName = "pr-nanny.sock"
)

// Op names one helper operation.
type Op string

const (
	OpSnapshot          Op = "snapshot"
	OpSaveSettings      Op = "saveSettings"
	OpConnectPAT        Op = "connectPAT"
	OpConnectCLI        Op = "connectCLI"
	OpSetBell           Op = "setBell"
	OpSetLayout         Op = "setLayout"
	OpRefresh           Op = "refresh"
	OpResolvePR         Op = "resolvePR"
	OpNextNotifications Op = "nextNotifications"
	OpHealth            Op = "health"
	OpVersion           Op = "version"
	OpStop              Op = "stop"
)

// Request is the client envelope. Body is an operation-specific JSON object;
// it may be omitted for operations without arguments.
type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Op      Op              `json:"op"`
	Body    json.RawMessage `json:"body,omitempty"`
}

// Response echoes Version and ID and carries exactly one of Body or Error.
type Response struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Body    json.RawMessage `json:"body,omitempty"`
	Error   *WireError      `json:"error,omitempty"`
}

// ErrorCode is stable for clients; Message is fixed, user-safe text.
type ErrorCode string

const (
	ErrInvalidRequest   ErrorCode = "INVALID_REQUEST"
	ErrProtocolVersion  ErrorCode = "PROTOCOL_VERSION"
	ErrUnknownOperation ErrorCode = "UNKNOWN_OPERATION"
	ErrUnauthorizedPeer ErrorCode = "UNAUTHORIZED_PEER"
	ErrSettings         ErrorCode = "SETTINGS"
	ErrAuthentication   ErrorCode = "AUTHENTICATION"
	ErrCredentialStore  ErrorCode = "CREDENTIAL_STORE" // #nosec G101 -- an error code name, not a credential.
	ErrUnavailable      ErrorCode = "UNAVAILABLE"
	ErrInternal         ErrorCode = "INTERNAL"
)

type WireError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Request bodies.
type SaveSettingsBody struct {
	Settings model.Settings `json:"settings"`
}
type ConnectPATBody struct {
	Token string `json:"token"`
}
type SetBellBody struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}
type SetLayoutBody struct {
	Layout string `json:"layout"`
}
type ResolvePRBody struct {
	ID string `json:"id"`
}
type NextNotificationsBody struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit,omitempty"`
}

// Reply bodies. snapshot, saveSettings, connectPAT, connectCLI, setBell and
// setLayout reply with model.Snapshot; nextNotifications with notify.Batch.
type ResolvePRReply struct {
	URL string `json:"url"`
}
type HealthReply struct {
	Status string `json:"status"`
}
type VersionReply struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Protocol int    `json:"protocol"`
}
