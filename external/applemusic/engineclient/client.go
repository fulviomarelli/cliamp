package engineclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"

	"cliamp/applog"
	"cliamp/internal/amprotocol"
)

type EngineClient struct {
	enginePath     string
	developerToken string
	mediaToken     string
	storefront     string
	debug          bool
	ctrlAddr       string
	audioAddr      string

	mu        sync.Mutex
	cmd       *exec.Cmd
	ctrlConn  net.Conn
	audioConn net.Conn
	streamer  *PCMStreamer
	events    chan amprotocol.Event

	closed bool
}

func New(enginePath, developerToken, mediaToken, storefront string, debug bool) *EngineClient {
	if enginePath == "" {
		enginePath = "cliamp-applemusic-engine"
	}
	// Use temp directory for sockets
	tmpDir := os.TempDir()
	return &EngineClient{
		enginePath:     enginePath,
		developerToken: developerToken,
		mediaToken:     mediaToken,
		storefront:     storefront,
		debug:          debug,
		ctrlAddr:       filepath.Join(tmpDir, "cliamp-am-ctrl.sock"),
		audioAddr:      filepath.Join(tmpDir, "cliamp-am-audio.sock"),
		events:         make(chan amprotocol.Event, 10),
	}
}

func (c *EngineClient) ensureStarted() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}

	// Remove old sockets
	os.Remove(c.ctrlAddr)
	os.Remove(c.audioAddr)

	args := []string{
		"--control-socket", c.ctrlAddr,
		"--audio-socket", c.audioAddr,
		"--developer-token", c.developerToken,
		"--storefront", c.storefront,
		"--media-token", c.mediaToken,
	}
	if c.debug {
		args = append(args, "--debug")
	}

	c.cmd = exec.Command(c.enginePath, args...)
	
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}

	// Connect in background to avoid blocking the TUI
	go c.connectLoop()

	return nil
}

func (c *EngineClient) connectLoop() {
	var err error
	var ctrlConn, audioConn net.Conn

	// Wait for sockets to appear and connect
	for i := 0; i < 100; i++ {
		ctrlConn, err = net.Dial("unix", c.ctrlAddr)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		applog.Error("engineclient: failed to connect to control socket: %v", err)
		return
	}

	for i := 0; i < 100; i++ {
		audioConn, err = net.Dial("unix", c.audioAddr)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		ctrlConn.Close()
		applog.Error("engineclient: failed to connect to audio socket: %v", err)
		return
	}

	c.mu.Lock()
	c.ctrlConn = ctrlConn
	c.audioConn = audioConn
	c.streamer = NewPCMStreamer(c.audioConn, 44100)
	c.mu.Unlock()

	applog.Info("engineclient: connected to engine")
	c.readEvents()
}

func (c *EngineClient) readEvents() {
	c.mu.Lock()
	conn := c.ctrlConn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var ev amprotocol.Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			applog.Error("engineclient: failed to unmarshal event: %v", err)
			continue
		}
		select {
		case c.events <- ev:
		default:
			// Buffer full, drop or handle?
		}
	}
}

func (c *EngineClient) sendCommand(cmd amprotocol.Command) error {
	c.ensureStarted()

	// Wait up to 5s for connection
	for i := 0; i < 50; i++ {
		c.mu.Lock()
		conn := c.ctrlConn
		c.mu.Unlock()
		if conn != nil {
			data, err := json.Marshal(cmd)
			if err != nil {
				return err
			}
			_, err = conn.Write(append(data, '\n'))
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("engine connection timed out")
}

func (c *EngineClient) SetQueue(tracks []string, startIndex int) error {
	return c.sendCommand(amprotocol.Command{
		Type:       amprotocol.TypeSetQueue,
		Tracks:     tracks,
		StartIndex: startIndex,
	})
}

func (c *EngineClient) Play() error {
	return c.sendCommand(amprotocol.Command{Type: amprotocol.TypePlay})
}

func (c *EngineClient) Pause() error {
	return c.sendCommand(amprotocol.Command{Type: amprotocol.TypePause})
}

func (c *EngineClient) Stop() error {
	return c.sendCommand(amprotocol.Command{Type: amprotocol.TypeStop})
}

func (c *EngineClient) Seek(seconds float64) error {
	return c.sendCommand(amprotocol.Command{
		Type:            amprotocol.TypeSeek,
		PositionSeconds: seconds,
	})
}

func (c *EngineClient) Streamer() beep.Streamer {
	return c.streamer
}

func (c *EngineClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.ctrlConn != nil {
		c.ctrlConn.Close()
	}
	if c.audioConn != nil {
		c.audioConn.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
	os.Remove(c.ctrlAddr)
	os.Remove(c.audioAddr)
}

func (c *EngineClient) Events() <-chan amprotocol.Event {
	return c.events
}
