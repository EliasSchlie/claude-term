package protocol

import (
	"encoding/json"
	"testing"
)

func TestEncodeDecodeRoundtrip(t *testing.T) {
	exitCode := 42
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "spawn request",
			msg:  Message{Type: TypeSpawn, ID: "r1", Cmd: "/bin/zsh", Args: []string{"-l"}, Cwd: "/tmp", Cols: 120, Rows: 40, Owner: "sess1"},
		},
		{
			name: "write request",
			msg:  Message{Type: TypeWrite, TermID: "t1", Data: "ls\n"},
		},
		{
			name: "spawned response",
			msg:  Message{Type: TypeSpawned, ID: "r1", TermID: "t1", PID: 12345},
		},
		{
			name: "exit event",
			msg:  Message{Type: TypeExit, TermID: "t1", ExitCode: &exitCode},
		},
		{
			name: "list result",
			msg: Message{Type: TypeListResult, ID: "r2", Terminals: []TerminalInfo{
				{TermID: "t1", PID: 123, Cmd: "zsh", Alive: true},
			}},
		},
		{
			name: "error",
			msg:  Message{Type: TypeError, ID: "r3", Error: "not found"},
		},
		{
			name: "ping",
			msg:  Message{Type: TypePing, ID: "r4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Encode(&tt.msg)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}

			got, err := Decode(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}

			if got.Type != tt.msg.Type {
				t.Errorf("type: got %q, want %q", got.Type, tt.msg.Type)
			}
			if got.ID != tt.msg.ID {
				t.Errorf("id: got %q, want %q", got.ID, tt.msg.ID)
			}
		})
	}
}

func TestOmitEmptyFields(t *testing.T) {
	msg := Message{Type: TypePing, ID: "r1"}
	data, _ := Encode(&msg)

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	// These fields should be omitted
	for _, field := range []string{"cmd", "args", "cwd", "env", "owner", "term_id", "data", "error", "terminals"} {
		if _, ok := raw[field]; ok {
			t.Errorf("field %q should be omitted when empty", field)
		}
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode([]byte("not json"))
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}
