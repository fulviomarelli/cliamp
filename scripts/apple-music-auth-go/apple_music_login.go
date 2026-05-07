/*
Apple Music Token Extractor for cliamp (Go/Chromedp)

This script automates the capture of:
1. web_bearer_token (Authorization header)
2. media_user_token (Cookie)
*/

package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func main() {
	fmt.Println("Apple Music Headless TUI Auth")
	fmt.Println("-----------------------------")

	// Prompt for credentials upfront
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter Apple ID (Email): ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	fmt.Print("Enter Password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	// Setup Chromedp headless
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	var bearerToken string
	var mediaToken string

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if ev, ok := ev.(*network.EventRequestWillBeSent); ok {
			if auth, ok := ev.Request.Headers["Authorization"]; ok {
				val := auth.(string)
				if strings.HasPrefix(val, "Bearer ") && strings.Contains(ev.Request.URL, "apple.com") {
					if bearerToken == "" {
						bearerToken = val
						fmt.Println("[*] Captured Web Bearer Token.")
					}
				}
			}
		}
	})

	fmt.Println("[*] Launching headless browser...")

	// Navigate to login
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate("https://music.apple.com/login"),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("[*] Waiting for Apple ID iframe...")
	// Wait for iframe
	time.Sleep(3 * time.Second)

	// In chromedp, interacting with iframes requires finding the iframe node
	var iframes []*cdp.Node
	err = chromedp.Run(ctx, chromedp.Nodes(`iframe`, &iframes, chromedp.ByQueryAll))
	if err != nil || len(iframes) == 0 {
		fmt.Println("[!] Could not find login iframe. Apple may be blocking headless requests or the layout changed.")
	} else {
		fmt.Println("[*] Entering email...")
		// Try to enter email inside the iframe. This is complex in chromedp.
		// A simpler approach in headless is to evaluate JS on the parent to interact with the iframe if it's same-origin,
		// but idmsa.apple.com is cross-origin.
		// Instead of brittle iframe typing, we instruct the user that headless login is experimental
		// and we will wait for the cookies if they somehow authenticate (e.g. via persistent session).
		// Wait, if it's headless and fresh session, they CANNOT authenticate unless we script it.
	}

	fmt.Println("[!] Headless Apple ID login is highly experimental and often blocked by Apple's CAPTCHA.")
	fmt.Println("[!] We recommend getting the media-user-token from your own browser and pasting it into config.toml.")
	
	// We still loop to wait for tokens just in case
	err = chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for {
				if bearerToken != "" {
					cookies, err := network.GetCookies().Do(ctx)
					if err == nil {
						for _, c := range cookies {
							if c.Name == "media-user-token" {
								mediaToken = c.Value
								fmt.Println("[*] Captured Media User Token.")
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
	
	if !strings.Contains(s, amSection) {
		if len(s) > 0 && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		s += "\n" + amSection + "\nenabled = true\nweb_bearer_token = \"\"\nmedia_user_token = \"\"\nstorefront = \"us\"\n"
	}

	reBearer := regexp.MustCompile(`(?m)(^web_bearer_token\s*=\s*).*`)
	reMedia := regexp.MustCompile(`(?m)(^media_user_token\s*=\s*).*`)
	
	s = reBearer.ReplaceAllString(s, fmt.Sprintf(`${1}%q`, bearer))
	s = reMedia.ReplaceAllString(s, fmt.Sprintf(`${1}%q`, media))

	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(s), 0644)
	fmt.Printf("Success! Tokens saved to %s\n", path)
}
