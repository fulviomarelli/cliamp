package applemusic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"cliamp/applog"
)

type Browser struct {
	ctx    context.Context
	cancel context.CancelFunc
	extID  string
}

func NewBrowser(extPath string) *Browser {
	home, _ := os.UserHomeDir()
	userDataDir := filepath.Join(home, ".config", "cliamp", "chrome-profile")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", "new"),
		chromedp.Flag("load-extension", extPath),
		chromedp.Flag("user-data-dir", userDataDir),
		chromedp.Flag("autoplay-policy", "no-user-gesture-required"),
		chromedp.Flag("mute-audio", false),
	)

	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	
	// Create context with logging and a longer timeout for initialization
	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(s string, v ...interface{}) {
			applog.Debug("chromedp: "+s, v...)
		}),
		// Silence the noise by providing a custom error logger
		chromedp.WithErrorf(func(s string, v ...interface{}) {
			// Filter out common harmless browser noise
			if strings.Contains(s, "EventAdoptedStyleSheetsModified") {
				return
			}
			applog.Error("chromedp: "+s, v...)
		}),
	)

	return &Browser{ctx: ctx, cancel: cancel}
}

func (b *Browser) Init() error {
	// Increased timeout to 60s for slow connections/cold browser boot
	ctx, cancel := context.WithTimeout(b.ctx, 60*time.Second)
	defer cancel()

	applog.Info("Apple Music: initializing browser...")
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://music.apple.com"),
		chromedp.WaitVisible("body"),
		chromedp.Evaluate(`
			new Promise(resolve => {
				const check = () => {
					if (!window.chrome || !window.chrome.management) {
						setTimeout(check, 100);
						return;
					}
					chrome.management.getAll(extensions => {
						const ext = extensions.find(e => e.name === "Cliamp Apple Music Capture");
						resolve(ext ? ext.id : "");
					});
				};
				check();
			})
		`, &b.extID),
	)
	if err != nil {
		return fmt.Errorf("browser init: %w", err)
	}
	if b.extID == "" {
		return fmt.Errorf("capture extension not found in browser")
	}
	applog.Info("Apple Music: browser ready (ext=%s)", b.extID)
	return nil
}

func (b *Browser) GetTokens() (string, string, error) {
	// Wait for MusicKit to be ready before extracting tokens
	var devToken, userToken string
	applog.Info("Apple Music: extracting tokens...")
	
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()

	err := chromedp.Run(ctx,
		chromedp.Evaluate(`
			new Promise(resolve => {
				const check = () => {
					if (window.MusicKit && window.MusicKit.getInstance()) {
						const mk = window.MusicKit.getInstance();
						if (mk.developerToken && mk.musicUserToken) {
							resolve([mk.developerToken, mk.musicUserToken]);
							return;
						}
					}
					setTimeout(check, 500);
				};
				check();
			})
		`, &[]string{devToken, userToken}),
	)
	// Evaluating into pointers directly for arrays is tricky in chromedp. 
	// Let's use a simpler approach.
	var tokens []string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				if (!window.MusicKit) return [];
				const mk = window.MusicKit.getInstance();
				if (!mk) return [];
				return [mk.developerToken || "", mk.musicUserToken || ""];
			})()
		`, &tokens),
	)
	if err == nil && len(tokens) == 2 {
		devToken, userToken = tokens[0], tokens[1]
	}

	if devToken == "" || userToken == "" {
		return "", "", fmt.Errorf("failed to extract tokens (login may be required)")
	}
	return devToken, userToken, err
}

func (b *Browser) PlayTrack(id string) error {
	script := fmt.Sprintf(`
		const mk = window.MusicKit.getInstance();
		mk.setQueue({ song: '%s' }).then(() => mk.play());
	`, id)
	return chromedp.Run(b.ctx, chromedp.Evaluate(script, nil))
}

func (b *Browser) Pause() error {
	return chromedp.Run(b.ctx, chromedp.Evaluate("window.MusicKit.getInstance().pause()", nil))
}

func (b *Browser) Resume() error {
	return chromedp.Run(b.ctx, chromedp.Evaluate("window.MusicKit.getInstance().play()", nil))
}

func (b *Browser) Seek(seconds int) error {
	script := fmt.Sprintf("window.MusicKit.getInstance().seekToTime(%d)", seconds)
	return chromedp.Run(b.ctx, chromedp.Evaluate(script, nil))
}

func (b *Browser) TriggerCapture() error {
	script := fmt.Sprintf(`chrome.runtime.sendMessage('%s', {action: "startCapture"})`, b.extID)
	return chromedp.Run(b.ctx, chromedp.Evaluate(script, nil))
}

func (b *Browser) Close() {
	b.cancel()
}
