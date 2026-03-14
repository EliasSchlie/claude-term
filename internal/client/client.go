package client

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/EliasSchlie/claude-term/internal/paths"
	"github.com/EliasSchlie/claude-term/internal/protocol"
)

// PushHandler handles asynchronous push events from the daemon.
type PushHandler struct {
	OnData   func(termID string, data []byte)
	OnReplay func(termID string, data []byte)
	OnExit   func(termID string, exitCode int)
}

// Client connects to the claude-term daemon.
type Client struct {
	conn       net.Conn
	scanner    *bufio.Scanner
	socketPath string

	mu       sync.Mutex
	reqID    atomic.Int64
	pending  map[string]chan *protocol.Message
	push     PushHandler
	closed   bool
	closedMu sync.RWMutex
}

// Connect creates a new client connected to the daemon.
func Connect(socketPath string) (*Client, error) {
	if socketPath == "" {
		socketPath = paths.SocketPath()
	}
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon: %w", err)
	}

	c := &Client{
		conn:       conn,
		scanner:    bufio.NewScanner(conn),
		socketPath: socketPath,
		pending:    make(map[string]chan *protocol.Message),
	}
	c.scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	go c.readLoop()
	return c, nil
}

// SetPushHandler sets the handler for push events.
func (c *Client) SetPushHandler(h PushHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.push = h
}

// SocketPath returns the socket path this client is connected to.
func (c *Client) SocketPath() string {
	return c.socketPath
}

// Close disconnects from the daemon.
func (c *Client) Close() error {
	c.closedMu.Lock()
	c.closed = true
	c.closedMu.Unlock()
	return c.conn.Close()
}

func (c *Client) nextID() string {
	return fmt.Sprintf("r%d", c.reqID.Add(1))
}

func (c *Client) readLoop() {
	for c.scanner.Scan() {
		msg, err := protocol.Decode(c.scanner.Bytes())
		if err != nil {
			continue
		}

		// Push events have no ID
		if msg.ID == "" {
			c.handlePush(msg)
			continue
		}

		// Request response
		c.mu.Lock()
		ch, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}
		c.mu.Unlock()

		if ok {
			ch <- msg
		}
	}

	// Connection closed — wake all pending requests
	c.mu.Lock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[string]chan *protocol.Message)
	c.mu.Unlock()
}

func (c *Client) handlePush(msg *protocol.Message) {
	c.mu.Lock()
	push := c.push
	c.mu.Unlock()

	switch msg.Type {
	case protocol.TypeData:
		if push.OnData != nil {
			data, _ := base64.StdEncoding.DecodeString(msg.Data)
			push.OnData(msg.TermID, data)
		}
	case protocol.TypeReplay:
		if push.OnReplay != nil {
			data, _ := base64.StdEncoding.DecodeString(msg.Data)
			push.OnReplay(msg.TermID, data)
		}
	case protocol.TypeExit:
		if push.OnExit != nil {
			code := -1
			if msg.ExitCode != nil {
				code = *msg.ExitCode
			}
			push.OnExit(msg.TermID, code)
		}
	}
}

// writeMsg encodes and writes a message to the connection.
func (c *Client) writeMsg(msg *protocol.Message) error {
	c.closedMu.RLock()
	if c.closed {
		c.closedMu.RUnlock()
		return fmt.Errorf("client closed")
	}
	c.closedMu.RUnlock()

	data, err := protocol.Encode(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = c.conn.Write(data)
	return err
}

func (c *Client) send(msg *protocol.Message) (*protocol.Message, error) {
	ch := make(chan *protocol.Message, 1)
	c.mu.Lock()
	c.pending[msg.ID] = ch
	c.mu.Unlock()

	if err := c.writeMsg(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, msg.ID)
		c.mu.Unlock()
		return nil, err
	}

	resp, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("connection closed")
	}
	if resp.Type == protocol.TypeError {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return resp, nil
}

// sendFireAndForget sends a message without waiting for a response.
func (c *Client) sendFireAndForget(msg *protocol.Message) error {
	return c.writeMsg(msg)
}

// SpawnOpts configures a new terminal spawn.
type SpawnOpts struct {
	Cmd   string
	Args  []string
	Cwd   string
	Cols  int
	Rows  int
	Env   map[string]string
	Owner string
}

// SpawnResult contains spawn response data.
type SpawnResult struct {
	TermID string
	PID    int
}

// Spawn creates a new terminal.
func (c *Client) Spawn(opts SpawnOpts) (*SpawnResult, error) {
	id := c.nextID()
	resp, err := c.send(&protocol.Message{
		Type:  protocol.TypeSpawn,
		ID:    id,
		Cmd:   opts.Cmd,
		Args:  opts.Args,
		Cwd:   opts.Cwd,
		Cols:  opts.Cols,
		Rows:  opts.Rows,
		Env:   opts.Env,
		Owner: opts.Owner,
	})
	if err != nil {
		return nil, err
	}
	return &SpawnResult{TermID: resp.TermID, PID: resp.PID}, nil
}

// Write sends input to a terminal (fire-and-forget).
func (c *Client) Write(termID string, data string) error {
	return c.sendFireAndForget(&protocol.Message{
		Type:   protocol.TypeWrite,
		TermID: termID,
		Data:   data,
	})
}

// Read returns the buffered output of a terminal.
func (c *Client) Read(termID string) ([]byte, error) {
	id := c.nextID()
	resp, err := c.send(&protocol.Message{
		Type:   protocol.TypeRead,
		ID:     id,
		TermID: termID,
	})
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(resp.Data)
}

// Attach subscribes to live terminal output. Push events will be
// delivered via the PushHandler.
func (c *Client) Attach(termID string) error {
	id := c.nextID()
	_, err := c.send(&protocol.Message{
		Type:   protocol.TypeAttach,
		ID:     id,
		TermID: termID,
	})
	return err
}

// Detach unsubscribes from terminal output (fire-and-forget).
func (c *Client) Detach(termID string) error {
	return c.sendFireAndForget(&protocol.Message{
		Type:   protocol.TypeDetach,
		TermID: termID,
	})
}

// Resize changes terminal dimensions.
func (c *Client) Resize(termID string, cols, rows int) error {
	id := c.nextID()
	_, err := c.send(&protocol.Message{
		Type:   protocol.TypeResize,
		ID:     id,
		TermID: termID,
		Cols:   cols,
		Rows:   rows,
	})
	return err
}

// Kill terminates a terminal.
func (c *Client) Kill(termID string) error {
	id := c.nextID()
	_, err := c.send(&protocol.Message{
		Type:   protocol.TypeKill,
		ID:     id,
		TermID: termID,
	})
	return err
}

// List returns terminals, optionally filtered by owner.
func (c *Client) List(owner string) ([]protocol.TerminalInfo, error) {
	id := c.nextID()
	resp, err := c.send(&protocol.Message{
		Type:  protocol.TypeList,
		ID:    id,
		Owner: owner,
	})
	if err != nil {
		return nil, err
	}
	if resp.Terminals == nil {
		return []protocol.TerminalInfo{}, nil
	}
	return resp.Terminals, nil
}

// Ping checks daemon health.
func (c *Client) Ping() error {
	id := c.nextID()
	_, err := c.send(&protocol.Message{
		Type: protocol.TypePing,
		ID:   id,
	})
	return err
}
