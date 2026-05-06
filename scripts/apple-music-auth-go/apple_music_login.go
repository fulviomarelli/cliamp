/*
Apple Music Token Extractor for cliamp (Go/Chromedp)

This script automates the capture of:
1. web_bearer_token (Authorization header)
2. media_user_token (Cookie)

Usage:
    go mod init apple-music-auth
    go get github.com/chromedp/chromedp
    go run apple_music_login.go
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func main() {
	// 1. Setup Chromedp with a visible browser
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var bearerToken string
	var mediaToken string

	// 2. Listen for the Authorization header in network requests
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if ev, ok := ev.(*network.EventRequestWillBeSent); ok {
			if auth, ok := ev.Request.Headers["Authorization"]; ok {
				val := auth.(string)
				if strings.HasPrefix(val, "Bearer ") && strings.Contains(ev.Request.URL, "apple.com") {
					if bearerToken == "" {
						bearerToken = val
						fmt.Println("Captured Web Bearer Token.")
					}
				}
			}
		}
	})

	fmt.Println("Launching browser... Please sign in to Apple Music.")

	// 3. Navigate to login and poll for the cookie
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate("https://music.apple.com/login"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Poll until we have both tokens
			for {
				if bearerToken != "" {
					// Use network.GetAllCookies() from the cdproto/network package
					cookies, err := network.GetAllCookies().Do(ctx)
					if err == nil {
						for _, c := range cookies {
							if c.Name == "media-user-token" {
								mediaToken = c.Value
								fmt.Println("Captured Media User Token.")
								return nil
							}
						}
					}
				}
				select {
				case <-time.After(1 * time.Second):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 4. Save tokens to cliamp config
	saveToConfig(bearerToken, mediaToken)
}

func saveToConfig(bearer, media string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(home, ".config", "cliamp", "config.toml")

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	s := string(content)
	amSection := `[apple_music]`
	
	// Create section if missing
	if !strings.Contains(s, amSection) {
		fmt.Printf("Config at %s doesn't have [apple_music] section. Appending it.\n", path)
		if len(s) > 0 && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + amSection + "\nenabled = true\nweb_bearer_token = \"\"\nmedia_user_token = \"\"\nstorefront = \"us\"\n"
	}

	// Surgically replace within the file. 
	// We use regex to find the keys. To be safe, we could narrow this to the section,
	// but in cliamp config these keys are unique to Apple Music.
	reBearer := regexp.MustCompile(`(?m)(^web_bearer_token\s*=\s*).*`)
	reMedia := regexp.MustCompile(`(?m)(^media_user_token\s*=\s*).*`)
	
	s = reBearer.ReplaceAllString(s, fmt.Sprintf(`${1}%q`, bearer))
	s = reMedia.ReplaceAllString(s, fmt.Sprintf(`${1}%q`, media))

	// Ensure parent dir exists
	err = os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(path, []byte(s), 0644)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Success! Tokens saved to %s\n", path)
}
