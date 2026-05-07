package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"cliamp/internal/amprotocol"
)

//go:embed index.html
var indexHTML embed.FS

var (
	ctrlAddr       = flag.String("control-socket", "", "Unix socket for control channel")
	audioAddr      = flag.String("audio-socket", "", "Unix socket for audio channel")
	developerToken = flag.String("developer-token", "", "Apple Music Developer Token")
	mediaToken     = flag.String("media-token", "", "Apple Music Media User Token")
	storefront     = flag.String("storefront", "us", "Apple Music Storefront")
	debug          = flag.Bool("debug", false, "Enable visible window")
)

type Engine struct {
	mu       sync.Mutex
	playing  bool
	audioOut io.Writer

	state amprotocol.Event
	
	ctx    context.Context
	cancel context.CancelFunc
}

func main() {
	flag.Parse()

	if *ctrlAddr == "" || *audioAddr == "" {
		fmt.Fprintln(os.Stderr, "Usage: cliamp-applemusic-engine --control-socket <path> --audio-socket <path> --developer-token <token>")
		os.Exit(1)
	}

	engine := &Engine{
		state: amprotocol.Event{
			Type:  amprotocol.TypeStatus,
			State: "stopped",
		},
	}

	// Start local HTTP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start local server: %v\n", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html, _ := indexHTML.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(html)
	})

	http.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		data, err := base64.StdEncoding.DecodeString(string(body))
		if err == nil && len(data) > 0 {
			engine.mu.Lock()
			out := engine.audioOut
			engine.mu.Unlock()
			if out != nil {
				out.Write(data)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	go http.Serve(listener, nil)

	// Setup chromedp
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !*debug),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	engine.ctx = ctx
	engine.cancel = cancel
	defer cancel()

	// Listen for control
	ctrlListener, err := net.Listen("unix", *ctrlAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on control socket: %v\n", err)
		os.Exit(1)
	}
	defer ctrlListener.Close()

	// Listen for audio
	audioListener, err := net.Listen("unix", *audioAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to listen on audio socket: %v\n", err)
		os.Exit(1)
	}
	defer audioListener.Close()

	go engine.acceptAudio(audioListener)
	go engine.acceptControl(ctrlListener)

	// Navigate to local server
	if err := chromedp.Run(ctx, 
		chromedp.ActionFunc(func(ctx context.Context) error {
			if *mediaToken != "" {
				err := network.SetCookie("media-user-token", *mediaToken).WithDomain(".apple.com").WithPath("/").Do(ctx)
				return err
			}
			return nil
		}),
		chromedp.Navigate(fmt.Sprintf("http://127.0.0.1:%d", port)),
	); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to navigate: %v\n", err)
		os.Exit(1)
	}

	// Inject setup script
	go func() {
		time.Sleep(1 * time.Second)
		chromedp.Run(engine.ctx, chromedp.Evaluate(fmt.Sprintf("window.setupMusicKit(%q, %q)", *developerToken, *storefront), nil))
	}()

	<-ctx.Done()
}

func (e *Engine) acceptControl(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		go e.handleControl(conn)
	}
}

func (e *Engine) handleControl(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var cmd amprotocol.Command
		if err := json.Unmarshal(scanner.Bytes(), &cmd); err != nil {
			continue
		}

		e.mu.Lock()
		switch cmd.Type {
		case amprotocol.TypeSetQueue:
			if len(cmd.Tracks) > 0 {
				trackURL := cmd.Tracks[cmd.StartIndex]
				chromedp.Run(e.ctx, chromedp.Evaluate(fmt.Sprintf("window.amPlay(%q)", trackURL), nil))
				e.state.Track = trackURL
			}
		case amprotocol.TypePlay:
			chromedp.Run(e.ctx, chromedp.Evaluate("window.amResume()", nil))
			e.playing = true
			e.state.State = "playing"
		case amprotocol.TypePause:
			chromedp.Run(e.ctx, chromedp.Evaluate("window.amPause()", nil))
			e.playing = false
			e.state.State = "paused"
		case amprotocol.TypeStop:
			chromedp.Run(e.ctx, chromedp.Evaluate("window.amPause()", nil))
			e.playing = false
			e.state.State = "stopped"
		case amprotocol.TypeSeek:
			chromedp.Run(e.ctx, chromedp.Evaluate(fmt.Sprintf("window.amSeek(%f)", cmd.PositionSeconds), nil))
			e.state.Position = cmd.PositionSeconds
		}

		status, _ := json.Marshal(e.state)
		conn.Write(append(status, '\n'))
		e.mu.Unlock()
	}
}

func (e *Engine) acceptAudio(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}
		e.mu.Lock()
		e.audioOut = conn
		e.mu.Unlock()
	}
}
