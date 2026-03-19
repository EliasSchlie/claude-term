package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/EliasSchlie/claude-term/internal/client"
	"github.com/EliasSchlie/claude-term/internal/daemon"
	"github.com/EliasSchlie/claude-term/internal/owner"
	"github.com/EliasSchlie/claude-term/internal/paths"
	"github.com/EliasSchlie/claude-term/internal/protocol"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "start":
		err = cmdStart()
	case "stop":
		err = cmdStop()
	case "spawn":
		err = cmdSpawn()
	case "list":
		err = cmdList()
	case "write":
		err = cmdWrite()
	case "read":
		err = cmdRead()
	case "attach":
		err = cmdAttach()
	case "resize":
		err = cmdResize()
	case "set-owner":
		err = cmdSetOwner()
	case "kill":
		err = cmdKill()
	case "subscribe":
		err = cmdSubscribe()
	case "ping":
		err = cmdPing()
	case "install":
		err = cmdInstall()
	case "uninstall":
		err = cmdUninstall()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage: claude-term <command> [args]

Commands:
  start              Start daemon (foreground)
  stop               Stop daemon
  spawn [cmd]        Spawn terminal (--owner, --cwd, --cols, --rows)
  list               List your terminals (--all for all, --owner to filter)
  write <id> <input> Write to terminal
  read <id>          Read terminal buffer
  attach <id>        Attach to terminal (interactive)
  resize <id> <c> <r> Resize terminal
  set-owner <id> <o> Change terminal owner
  kill <id>          Kill terminal
  subscribe          Stream lifecycle events (JSON lines)
  ping               Health check
  install            Install hooks and skill into Claude Code
  uninstall          Remove hooks and skill from Claude Code`)
}

func cmdStart() error {
	d := daemon.New()
	return d.Run()
}

func cmdStop() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	return fmt.Errorf("stop not implemented yet — use kill signal")
}

func connect() (*client.Client, error) {
	c, err := client.Connect("")
	if err == nil {
		return c, nil
	}

	// Daemon not running — auto-start it
	if err := ensureDaemon(); err != nil {
		return nil, fmt.Errorf("auto-start daemon: %w", err)
	}
	return client.Connect("")
}

// ensureDaemon starts the daemon in the background if it's not running.
func ensureDaemon() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(self, "start")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()

	// Wait for daemon to accept connections (not just socket file existing)
	sockPath := paths.SocketPath()
	for i := 0; i < 50; i++ {
		time.Sleep(20 * time.Millisecond)
		if c, err := client.Connect(sockPath); err == nil {
			c.Close()
			return nil
		}
	}
	return fmt.Errorf("daemon did not start within 1s")
}

// resolveOwner returns the owner from --owner flag, or auto-discovers from process tree.
// Warns if running inside Claude Code without the hook installed.
func resolveOwner(explicit string) string {
	if explicit != "" {
		return explicit
	}
	discovered := owner.Discover()
	if discovered == "" && os.Getenv("CLAUDECODE") == "1" {
		fmt.Fprintln(os.Stderr, "⚠️  claude-term hooks not installed. Install via plugin or run: claude-term install")
	}
	return discovered
}

func cmdSpawn() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	opts := client.SpawnOpts{}
	var explicitOwner string
	args := os.Args[2:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--owner":
			i++
			if i < len(args) {
				explicitOwner = args[i]
			}
		case "--cwd":
			i++
			if i < len(args) {
				opts.Cwd = args[i]
			}
		case "--cols":
			i++
			if i < len(args) {
				opts.Cols, _ = strconv.Atoi(args[i])
			}
		case "--rows":
			i++
			if i < len(args) {
				opts.Rows, _ = strconv.Atoi(args[i])
			}
		default:
			if opts.Cmd == "" {
				opts.Cmd = args[i]
			} else {
				opts.Args = append(opts.Args, args[i])
			}
		}
	}

	opts.Owner = resolveOwner(explicitOwner)

	result, err := c.Spawn(opts)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", result.TermID)
	return nil
}

func cmdList() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	showAll := false
	var explicitOwner string
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			showAll = true
		case "--owner":
			if i+1 < len(args) {
				i++
				explicitOwner = args[i]
			}
		}
	}

	// Default: filter by auto-discovered owner (show only your terminals)
	// --all: show all terminals
	// --owner <id>: filter by specific owner
	filterOwner := ""
	if !showAll {
		filterOwner = resolveOwner(explicitOwner)
	}

	terms, err := c.List(filterOwner)
	if err != nil {
		return err
	}

	data, _ := json.MarshalIndent(terms, "", "  ")
	fmt.Println(string(data))
	return nil
}

func cmdWrite() error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: claude-term write <id> <input>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	termID := os.Args[2]
	input := unescapeInput(strings.Join(os.Args[3:], " "))
	return c.Write(termID, input)
}

// unescapeInput interprets C-style escape sequences in CLI input so that
// `claude-term write t1 "npm test\n"` sends a real newline.
func unescapeInput(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		switch s[i+1] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case 'x':
			// Hex escape: \x1b etc.
			if i+3 < len(s) {
				if val, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteByte(byte(val))
					i += 3
					continue
				}
			}
			b.WriteByte('\\')
			continue // don't skip next char
		default:
			b.WriteByte('\\')
			continue // don't skip next char — not a recognized escape
		}
		i++ // skip the char after backslash
	}
	return b.String()
}

func cmdRead() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: claude-term read <id>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	data, err := c.Read(os.Args[2])
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

func cmdAttach() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: claude-term attach <id>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	termID := os.Args[2]
	done := make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(done) }) }

	c.SetPushHandler(client.PushHandler{
		OnData: func(_ string, data []byte) {
			os.Stdout.Write(data)
		},
		OnReplay: func(_ string, data []byte) {
			os.Stdout.Write(data)
		},
		OnExit: func(_ string, code int) {
			fmt.Fprintf(os.Stderr, "\n[process exited with code %d]\n", code)
			finish()
		},
	})

	if err := c.Attach(termID); err != nil {
		return err
	}

	// Put terminal in raw mode for interactive use
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err == nil {
		defer term.Restore(fd, oldState)
	}

	// Sync remote terminal size to match local terminal on attach
	syncSize := func() {
		w, h, err := term.GetSize(fd)
		if err == nil {
			c.Resize(termID, w, h)
		}
	}
	syncSize()

	// Forward SIGWINCH (terminal resize) to remote terminal with debounce.
	// Debounce prevents resize loops in nested PTY environments (e.g., Open Cockpit).
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(winchCh)
		var lastW, lastH int
		for {
			select {
			case <-winchCh:
				w, h, err := term.GetSize(fd)
				if err != nil {
					continue
				}
				// Only forward if size actually changed (breaks resize loops)
				if w == lastW && h == lastH {
					continue
				}
				lastW, lastH = w, h
				c.Resize(termID, w, h)
			case <-done:
				return
			}
		}
	}()

	// Detect connection loss
	go func() {
		<-c.Done()
		fmt.Fprintf(os.Stderr, "\n[connection to daemon lost]\n")
		finish()
	}()

	// Forward stdin to terminal
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				// Ctrl+] to detach
				if buf[0] == 0x1d {
					c.Detach(termID)
					finish()
					return
				}
				c.Write(termID, string(buf[:n]))
			}
			if err == io.EOF || err != nil {
				return
			}
		}
	}()

	<-done
	return nil
}

func cmdResize() error {
	if len(os.Args) < 5 {
		return fmt.Errorf("usage: claude-term resize <id> <cols> <rows>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	cols, _ := strconv.Atoi(os.Args[3])
	rows, _ := strconv.Atoi(os.Args[4])
	return c.Resize(os.Args[2], cols, rows)
}

func cmdSetOwner() error {
	if len(os.Args) < 4 {
		return fmt.Errorf("usage: claude-term set-owner <id> <owner>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.SetOwner(os.Args[2], os.Args[3])
}

func cmdKill() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: claude-term kill <id>")
	}
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Kill(os.Args[2])
}

func cmdSubscribe() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()

	writeEvent := func(msg *protocol.Message) {
		data, err := protocol.Encode(msg)
		if err != nil {
			return
		}
		data = append(data, '\n')
		os.Stdout.Write(data)
	}

	c.SetPushHandler(client.PushHandler{
		OnTermSpawned:      writeEvent,
		OnTermKilled:       writeEvent,
		OnTermExited:       writeEvent,
		OnTermOwnerChanged: writeEvent,
	})

	if err := c.Subscribe(); err != nil {
		return err
	}

	// Block until connection closes or signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	select {
	case <-sigs:
		c.Unsubscribe()
	case <-c.Done():
		return fmt.Errorf("connection to daemon lost")
	}
	return nil
}

func cmdPing() error {
	c, err := connect()
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Ping(); err != nil {
		return err
	}
	fmt.Println("pong")
	return nil
}
