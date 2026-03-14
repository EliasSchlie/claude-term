package daemon

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/EliasSchlie/claude-term/internal/paths"
	"github.com/EliasSchlie/claude-term/internal/protocol"
	"github.com/EliasSchlie/claude-term/internal/terminal"
)

const (
	idleTimeout        = 30 * time.Minute
	inactivityTimeout  = 24 * time.Hour
	inactivityCheckInt = 5 * time.Minute
	writeQueueSize     = 256
	writeTimeout       = 5 * time.Second
)

// connState tracks per-connection write queue and lifecycle.
type connState struct {
	writeCh chan []byte
	done    chan struct{}
}

func newConnState(conn net.Conn) *connState {
	cs := &connState{
		writeCh: make(chan []byte, writeQueueSize),
		done:    make(chan struct{}),
	}
	go cs.writeLoop(conn)
	return cs
}

func (cs *connState) writeLoop(conn net.Conn) {
	defer close(cs.done)
	for data := range cs.writeCh {
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if _, err := conn.Write(data); err != nil {
			return
		}
	}
}

func (cs *connState) close() {
	close(cs.writeCh)
}

// Daemon is the claude-term server.
type Daemon struct {
	registry *terminal.Registry
	listener net.Listener

	mu      sync.Mutex
	clients map[net.Conn]*connState
	// termID -> set of attached client connections
	attached map[string]map[net.Conn]struct{}

	socketPath string
	pidPath    string
	quit       chan struct{}
	idleTimer  *time.Timer
}

// New creates a new daemon.
func New() *Daemon {
	return &Daemon{
		registry:   terminal.NewRegistry(),
		clients:    make(map[net.Conn]*connState),
		attached:   make(map[string]map[net.Conn]struct{}),
		socketPath: paths.SocketPath(),
		pidPath:    paths.PIDPath(),
		quit:       make(chan struct{}),
	}
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() error {
	if err := paths.EnsureDir(); err != nil {
		return fmt.Errorf("ensure dir: %w", err)
	}

	// Remove stale socket
	_ = os.Remove(d.socketPath)

	ln, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	d.listener = ln

	// Restrict socket permissions
	if err := os.Chmod(d.socketPath, 0o600); err != nil {
		ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	// Write PID file
	if err := os.WriteFile(d.pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644); err != nil {
		ln.Close()
		return fmt.Errorf("write pid: %w", err)
	}

	// Signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		d.Shutdown()
	}()

	// Idle timer
	d.idleTimer = time.AfterFunc(idleTimeout, func() {
		d.mu.Lock()
		clientCount := len(d.clients)
		termCount := d.registry.Count()
		d.mu.Unlock()
		if clientCount == 0 && termCount == 0 {
			log.Println("idle timeout, shutting down")
			d.Shutdown()
		}
	})

	// Inactivity reaper — kills terminals idle for 24h
	go d.inactivityReaper()

	log.Printf("daemon listening on %s (pid %d)", d.socketPath, os.Getpid())

	// Accept loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-d.quit:
				return nil
			default:
				log.Printf("accept error: %v", err)
				continue
			}
		}
		cs := newConnState(conn)
		d.mu.Lock()
		d.clients[conn] = cs
		d.resetIdleTimer()
		d.mu.Unlock()
		go d.handleConn(conn)
	}
}

// Shutdown cleanly stops the daemon.
func (d *Daemon) Shutdown() {
	select {
	case <-d.quit:
		return // already shutting down
	default:
		close(d.quit)
	}

	if d.idleTimer != nil {
		d.idleTimer.Stop()
	}

	// Kill all terminals
	for _, t := range d.registry.List("") {
		_ = t.Kill()
	}

	if d.listener != nil {
		d.listener.Close()
	}
	_ = os.Remove(d.socketPath)
	_ = os.Remove(d.pidPath)
}

func (d *Daemon) resetIdleTimer() {
	if d.idleTimer != nil {
		d.idleTimer.Reset(idleTimeout)
	}
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer func() {
		d.mu.Lock()
		if cs, ok := d.clients[conn]; ok {
			cs.close()
		}
		delete(d.clients, conn)
		// Remove from all attached sets
		for termID, clients := range d.attached {
			delete(clients, conn)
			if len(clients) == 0 {
				delete(d.attached, termID)
			}
		}
		d.resetIdleTimer()
		d.mu.Unlock()
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max message
	for scanner.Scan() {
		msg, err := protocol.Decode(scanner.Bytes())
		if err != nil {
			d.sendMsg(conn, &protocol.Message{Type: protocol.TypeError, Error: "invalid json"})
			continue
		}
		d.handleMessage(conn, msg)
	}
}

func (d *Daemon) handleMessage(conn net.Conn, msg *protocol.Message) {
	switch msg.Type {
	case protocol.TypeSpawn:
		d.handleSpawn(conn, msg)
	case protocol.TypeWrite:
		d.handleWrite(msg)
	case protocol.TypeResize:
		d.handleResize(conn, msg)
	case protocol.TypeRead:
		d.handleRead(conn, msg)
	case protocol.TypeAttach:
		d.handleAttach(conn, msg)
	case protocol.TypeDetach:
		d.handleDetach(conn, msg)
	case protocol.TypeSetOwner:
		d.handleSetOwner(conn, msg)
	case protocol.TypeKill:
		d.handleKill(conn, msg)
	case protocol.TypeList:
		d.handleList(conn, msg)
	case protocol.TypePing:
		d.sendMsg(conn, &protocol.Message{Type: protocol.TypePong, ID: msg.ID})
	default:
		d.sendMsg(conn, &protocol.Message{Type: protocol.TypeError, ID: msg.ID, Error: fmt.Sprintf("unknown type: %s", msg.Type)})
	}
}

// requireTerminal looks up a terminal and sends an error if not found. Returns nil on miss.
func (d *Daemon) requireTerminal(conn net.Conn, msg *protocol.Message) *terminal.Terminal {
	t := d.registry.Get(msg.TermID)
	if t == nil {
		d.sendMsg(conn, &protocol.Message{Type: protocol.TypeError, ID: msg.ID, Error: fmt.Sprintf("terminal not found: %s", msg.TermID)})
	}
	return t
}

func (d *Daemon) handleSpawn(conn net.Conn, msg *protocol.Message) {
	id := d.registry.NextID()

	t, err := terminal.Spawn(id, terminal.SpawnOpts{
		Cmd:   msg.Cmd,
		Args:  msg.Args,
		Cwd:   msg.Cwd,
		Cols:  msg.Cols,
		Rows:  msg.Rows,
		Env:   msg.Env,
		Owner: msg.Owner,
		OnData: func(termID string, data []byte) {
			d.broadcast(termID, &protocol.Message{
				Type:   protocol.TypeData,
				TermID: termID,
				Data:   base64.StdEncoding.EncodeToString(data),
			})
		},
		OnExit: func(termID string, code int) {
			exitCode := code
			d.broadcast(termID, &protocol.Message{
				Type:     protocol.TypeExit,
				TermID:   termID,
				ExitCode: &exitCode,
			})
		},
	})
	if err != nil {
		d.sendMsg(conn, &protocol.Message{Type: protocol.TypeError, ID: msg.ID, Error: err.Error()})
		return
	}

	d.registry.Add(t)
	d.resetIdleTimer()
	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeSpawned, ID: msg.ID, TermID: id, PID: t.PID})
}

func (d *Daemon) handleWrite(msg *protocol.Message) {
	t := d.registry.Get(msg.TermID)
	if t == nil {
		return // fire-and-forget, no error response
	}
	_ = t.Write([]byte(msg.Data))
}

func (d *Daemon) handleResize(conn net.Conn, msg *protocol.Message) {
	t := d.requireTerminal(conn, msg)
	if t == nil {
		return
	}
	if err := t.Resize(msg.Cols, msg.Rows); err != nil {
		d.sendMsg(conn, &protocol.Message{Type: protocol.TypeError, ID: msg.ID, Error: err.Error()})
		return
	}
	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeResized, ID: msg.ID, TermID: msg.TermID})
}

func (d *Daemon) handleRead(conn net.Conn, msg *protocol.Message) {
	t := d.requireTerminal(conn, msg)
	if t == nil {
		return
	}
	buf := t.ReadBuffer()
	d.sendMsg(conn, &protocol.Message{
		Type:   protocol.TypeReadResult,
		ID:     msg.ID,
		TermID: msg.TermID,
		Data:   base64.StdEncoding.EncodeToString(buf),
	})
}

func (d *Daemon) handleAttach(conn net.Conn, msg *protocol.Message) {
	t := d.requireTerminal(conn, msg)
	if t == nil {
		return
	}

	d.mu.Lock()
	if d.attached[msg.TermID] == nil {
		d.attached[msg.TermID] = make(map[net.Conn]struct{})
	}
	d.attached[msg.TermID][conn] = struct{}{}
	d.mu.Unlock()

	// Send replay of buffered output
	buf := t.ReadBuffer()
	if len(buf) > 0 {
		d.sendMsg(conn, &protocol.Message{
			Type:   protocol.TypeReplay,
			TermID: msg.TermID,
			Data:   base64.StdEncoding.EncodeToString(buf),
		})
	}

	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeAttached, ID: msg.ID, TermID: msg.TermID})

	// If process already exited, send exit event
	if !t.IsAlive() {
		// We don't have the exit code stored on the terminal, send -1
		exitCode := -1
		d.sendMsg(conn, &protocol.Message{
			Type:     protocol.TypeExit,
			TermID:   msg.TermID,
			ExitCode: &exitCode,
		})
	}
}

func (d *Daemon) handleDetach(conn net.Conn, msg *protocol.Message) {
	d.mu.Lock()
	if clients, ok := d.attached[msg.TermID]; ok {
		delete(clients, conn)
		if len(clients) == 0 {
			delete(d.attached, msg.TermID)
		}
	}
	d.mu.Unlock()
}

func (d *Daemon) handleSetOwner(conn net.Conn, msg *protocol.Message) {
	t := d.requireTerminal(conn, msg)
	if t == nil {
		return
	}
	t.SetOwner(msg.Owner)
	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeOwnerSet, ID: msg.ID, TermID: msg.TermID})
}

func (d *Daemon) handleKill(conn net.Conn, msg *protocol.Message) {
	t := d.requireTerminal(conn, msg)
	if t == nil {
		return
	}
	_ = t.Kill()
	d.registry.Remove(msg.TermID)

	// Clean up attached clients for this terminal
	d.mu.Lock()
	delete(d.attached, msg.TermID)
	d.mu.Unlock()

	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeKilled, ID: msg.ID, TermID: msg.TermID})
}

func (d *Daemon) handleList(conn net.Conn, msg *protocol.Message) {
	terms := d.registry.List(msg.Owner)
	infos := make([]protocol.TerminalInfo, 0, len(terms))
	for _, t := range terms {
		infos = append(infos, protocol.TerminalInfo{
			TermID:    t.ID,
			PID:       t.PID,
			Cmd:       t.Cmd,
			Cwd:       t.Cwd,
			Cols:      t.Cols,
			Rows:      t.Rows,
			Owner:     t.Owner,
			StartedAt: t.StartedAt,
			Alive:     t.IsAlive(),
		})
	}
	d.sendMsg(conn, &protocol.Message{Type: protocol.TypeListResult, ID: msg.ID, Terminals: infos})
}

// broadcast sends a pre-encoded message to all clients attached to a terminal.
// Marshals once, enqueues to each client's write goroutine (non-blocking).
func (d *Daemon) broadcast(termID string, msg *protocol.Message) {
	data, err := protocol.Encode(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')

	d.mu.Lock()
	for conn := range d.attached[termID] {
		if cs, ok := d.clients[conn]; ok {
			select {
			case cs.writeCh <- data:
			default:
				// Client can't keep up — drop message
			}
		}
	}
	d.mu.Unlock()
}

// sendMsg encodes and enqueues a message to a single connection.
func (d *Daemon) sendMsg(conn net.Conn, msg *protocol.Message) {
	data, err := protocol.Encode(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')

	d.mu.Lock()
	cs, ok := d.clients[conn]
	d.mu.Unlock()

	if ok {
		select {
		case cs.writeCh <- data:
		default:
		}
	}
}

// inactivityReaper periodically checks for terminals with no activity
// and kills them after the inactivity timeout (24h).
func (d *Daemon) inactivityReaper() {
	ticker := time.NewTicker(inactivityCheckInt)
	defer ticker.Stop()
	for {
		select {
		case <-d.quit:
			return
		case <-ticker.C:
			now := time.Now()
			for _, t := range d.registry.List("") {
				if now.Sub(t.GetLastActivity()) > inactivityTimeout {
					log.Printf("terminal %s inactive for >24h, cleaning up", t.ID)
					_ = t.Kill()
					d.registry.Remove(t.ID)
					d.mu.Lock()
					delete(d.attached, t.ID)
					d.mu.Unlock()
				}
			}
		}
	}
}
