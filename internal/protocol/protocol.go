package protocol

import (
	"encoding/json"
	"time"
)

// Request types
const (
	TypeSpawn       = "spawn"
	TypeWrite       = "write"
	TypeResize      = "resize"
	TypeRead        = "read"
	TypeAttach      = "attach"
	TypeDetach      = "detach"
	TypeSetOwner    = "set_owner"
	TypeKill        = "kill"
	TypeList        = "list"
	TypePing        = "ping"
	TypeSubscribe   = "subscribe"
	TypeUnsubscribe = "unsubscribe"
)

// Response types
const (
	TypeSpawned    = "spawned"
	TypeResized    = "resized"
	TypeReadResult = "read_result"
	TypeAttached   = "attached"
	TypeOwnerSet   = "owner_set"
	TypeKilled     = "killed"
	TypeListResult = "list_result"
	TypePong       = "pong"
	TypeError      = "error"
	TypeSubscribed = "subscribed"
)

// Push event types
const (
	TypeData   = "data"
	TypeReplay = "replay"
	TypeExit   = "exit"

	// Lifecycle push events (for subscribers)
	TypeTermSpawned      = "term_spawned"
	TypeTermKilled       = "term_killed"
	TypeTermExited       = "term_exited"
	TypeTermOwnerChanged = "term_owner_changed"
)

// Message is the wire format for all protocol messages.
// Fields are omitted when zero/empty to keep messages compact.
type Message struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`

	// Spawn fields
	Cmd   string            `json:"cmd,omitempty"`
	Args  []string          `json:"args,omitempty"`
	Cwd   string            `json:"cwd,omitempty"`
	Env   map[string]string `json:"env,omitempty"`
	Owner string            `json:"owner,omitempty"`

	// Terminal identifier
	TermID string `json:"term_id,omitempty"`

	// Dimensions
	Cols int `json:"cols,omitempty"`
	Rows int `json:"rows,omitempty"`

	// Data (base64 for data/replay, plain for write)
	Data string `json:"data,omitempty"`

	// Spawn response
	PID int `json:"pid,omitempty"`

	// Exit event
	ExitCode *int `json:"exit_code,omitempty"`

	// Error
	Error string `json:"error,omitempty"`

	// List response
	Terminals []TerminalInfo `json:"terminals,omitempty"`
}

// TerminalInfo describes a terminal in list responses.
type TerminalInfo struct {
	TermID    string    `json:"term_id"`
	PID       int       `json:"pid"`
	Cmd       string    `json:"cmd"`
	Cwd       string    `json:"cwd"`
	Cols      int       `json:"cols"`
	Rows      int       `json:"rows"`
	Owner     string    `json:"owner,omitempty"`
	StartedAt time.Time `json:"started_at"`
	Alive     bool      `json:"alive"`
}

// Encode serializes a message to JSON bytes (no trailing newline).
func Encode(m *Message) ([]byte, error) {
	return json.Marshal(m)
}

// Decode deserializes a message from JSON bytes.
func Decode(data []byte) (*Message, error) {
	var m Message
	err := json.Unmarshal(data, &m)
	return &m, err
}
