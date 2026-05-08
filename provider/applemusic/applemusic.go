package applemusic

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/minchao/go-apple-music"

	"cliamp/playlist"
	"cliamp/provider"
)

// Provider implements the cliamp playlist.Provider and extended capabilities.
type Provider struct {
	browser   *Browser
	wsBuffer  *AudioBuffer
	apiClient *applemusic.Client
}

// Ensure Provider implements the necessary interfaces
var _ playlist.Provider = (*Provider)(nil)
var _ provider.Searcher = (*Provider)(nil)
var _ provider.CustomStreamer = (*Provider)(nil)

// NewProvider initializes the Apple Music provider, setting up the Chrome extension,
// WebSocket receiver, browser session, and API client.
func NewProvider() (*Provider, error) {
	log.Println("applemusic: generating and writing extension to disk...")
	extPath, err := WriteExtensionToDisk()
	if err != nil {
		return nil, fmt.Errorf("failed to write extension: %w", err)
	}

	log.Println("applemusic: starting WebSocket receiver...")
	wsBuffer := StartWSServer()

	log.Println("applemusic: starting headless chromedp browser to check auth...")
	b, err := NewBrowser(extPath, true)
	if err != nil {
		return nil, fmt.Errorf("failed to start headless browser (you may need to close other Chrome instances or use a non-snap Chrome): %w", err)
	}

	var devToken, userToken string
	// Poll for up to 10 seconds to allow the page and MusicKit to fully load in the background
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		devToken, userToken, _ = b.GetTokens()
		if userToken != "" {
			break
		}
	}

	if userToken == "" {
		log.Println("applemusic: no active session found. Re-launching in visible mode for authentication...")
		b.Close()

		// Launch in headful mode so the user can log in
		b, err = NewBrowser(extPath, false)
		if err != nil {
			return nil, fmt.Errorf("failed to start headful browser: %w", err)
		}
		
		log.Println("applemusic: Please log in to Apple Music in the opened browser window.")
		// Wait until userToken is populated
		for userToken == "" {
			time.Sleep(2 * time.Second)
			devToken, userToken, _ = b.GetTokens()
		}
		
		log.Println("applemusic: authentication successful! Restarting browser in background...")
		b.Close()
		
		// Re-launch in headless mode for invisible playback
		b, err = NewBrowser(extPath, true)
		if err != nil {
			return nil, fmt.Errorf("failed to restart headless browser after auth: %w", err)
		}
		time.Sleep(2 * time.Second)
	}

	if devToken == "" || userToken == "" {
		return nil, fmt.Errorf("failed to obtain Apple Music tokens after auth flow")
	}

	log.Println("applemusic: initializing REST API client...")
	httpClient := &http.Client{
		Transport: &authTransport{
			base:      http.DefaultTransport,
			devToken:  devToken,
			userToken: userToken,
		},
	}
	apiClient := applemusic.NewClient(httpClient)

	log.Println("applemusic: triggering audio capture via extension...")
	err = b.TriggerCapture()
	if err != nil {
		log.Printf("applemusic: warning: trigger capture failed: %v", err)
	}

	return &Provider{
		browser:   b,
		wsBuffer:  wsBuffer,
		apiClient: apiClient,
	}, nil
}

// Name returns the display name of this provider.
func (p *Provider) Name() string {
	return "Apple Music"
}

// Close shuts down the browser and cleans up resources.
func (p *Provider) Close() error {
	if p.browser != nil {
		p.browser.Close()
	}
	if p.wsBuffer != nil {
		return p.wsBuffer.Close()
	}
	return nil
}

// Playlists returns the available playlists from this provider (e.g. from the user's library).
func (p *Provider) Playlists() ([]playlist.PlaylistInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limit := 50
	libPlaylists, _, err := p.apiClient.Me.GetAllLibraryPlaylists(ctx, &applemusic.PageOptions{Limit: limit})
	if err != nil {
		log.Printf("applemusic: failed to fetch library playlists: %v", err)
		// Return at least the library node even if playlists fail
	}

	out := []playlist.PlaylistInfo{
		{
			ID:      "am:library",
			Name:    "Library",
			Section: "Library",
		},
	}

	if libPlaylists != nil {
		for _, pl := range libPlaylists.Data {
			if pl.Id == "" {
				continue
			}
			name := strings.TrimSpace(pl.Attributes.Name)
			if name == "" {
				name = pl.Id
			}
			out = append(out, playlist.PlaylistInfo{
				ID:      "lib:" + pl.Id,
				Name:    name,
				Section: "Playlists",
			})
		}
	}

	return out, nil
}

// Tracks returns the tracks in the given playlist.
func (p *Provider) Tracks(playlistID string) ([]playlist.Track, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out []playlist.Track

	if playlistID == "am:library" {
		res, _, err := p.apiClient.Me.GetAllLibrarySongs(ctx, &applemusic.PageOptions{Limit: 100})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch library songs: %w", err)
		}
		
		if res != nil {
			for _, song := range res.Data {
				catalogId := song.Attributes.PlayParams.CatalogId
				if catalogId == "" {
					continue
				}
				
				t := playlist.Track{
					Path:         "applemusic:track:" + catalogId,
					Title:        song.Attributes.Name,
					Artist:       song.Attributes.ArtistName,
					Album:        song.Attributes.AlbumName,
					DurationSecs: int(song.Attributes.DurationInMillis / 1000),
					Stream:       true,
				}
				out = append(out, t)
			}
		}
	} else if strings.HasPrefix(playlistID, "lib:") {
		id := strings.TrimPrefix(playlistID, "lib:")
		res, _, err := p.apiClient.Me.GetLibraryPlaylist(ctx, id, &applemusic.Options{
			Include: "tracks",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch playlist tracks: %w", err)
		}

		if res != nil && len(res.Data) > 0 && len(res.Data[0].Relationships.Tracks.Data) > 0 {
			for _, raw := range res.Data[0].Relationships.Tracks.Data {
				parsed, err := raw.Parse()
				if err != nil || parsed == nil {
					continue
				}
				switch v := parsed.(type) {
				case *applemusic.LibrarySong:
					catalogId := v.Attributes.PlayParams.CatalogId
					if catalogId == "" {
						continue
					}
					t := playlist.Track{
						Path:         "applemusic:track:" + catalogId,
						Title:        v.Attributes.Name,
						Artist:       v.Attributes.ArtistName,
						Album:        v.Attributes.AlbumName,
						DurationSecs: int(v.Attributes.DurationInMillis / 1000),
						Stream:       true,
					}
					out = append(out, t)
				}
			}
		}
	}

	return out, nil
}

// SearchTracks queries the Apple Music catalog for tracks.
func (p *Provider) SearchTracks(ctx context.Context, query string, limit int) ([]playlist.Track, error) {
	// Fast API search completely bypassing the browser.
	// The Apple Music API expects storefront in the search. We use a default here.
	// In a real implementation, we would fetch the user's storefront.
	storefront := "us" 
	
	opts := &applemusic.SearchOptions{
		Term:  query,
		Types: "songs",
		Limit: limit,
	}

	res, _, err := p.apiClient.Catalog.Search(ctx, storefront, opts)
	if err != nil {
		return nil, fmt.Errorf("apple music API search failed: %w", err)
	}

	var tracks []playlist.Track
	if res.Results.Songs == nil {
		return tracks, nil
	}

	for _, song := range res.Results.Songs.Data {
		durationSecs := song.Attributes.DurationInMillis / 1000
		
		t := playlist.Track{
			Path:         "applemusic:track:" + song.Id,
			Title:        song.Attributes.Name,
			Artist:       song.Attributes.ArtistName,
			Album:        song.Attributes.AlbumName,
			DurationSecs: int(durationSecs),
			Stream:       true,
		}
		tracks = append(tracks, t)
	}

	return tracks, nil
}

// URISchemes returns the URI prefixes this provider handles.
func (p *Provider) URISchemes() []string {
	return []string{"applemusic:"}
}

// NewStreamer intercepts playback for Apple Music tracks.
func (p *Provider) NewStreamer(uri string) (beep.StreamSeekCloser, beep.Format, time.Duration, error) {
	// uri example: "applemusic:track:123456789"
	id := strings.TrimPrefix(uri, "applemusic:track:")

	// Tell the browser to play this track in the background
	err := p.browser.PlayTrack(id)
	if err != nil {
		return nil, beep.Format{}, 0, fmt.Errorf("failed to play track in browser: %w", err)
	}

	// We assume a standard format from the browser (e.g. 48kHz stereo, or 44.1kHz).
	// WebRTC / getUserMedia typically defaults to 48000 Hz for Opus.
	format := beep.Format{
		SampleRate:  beep.SampleRate(48000),
		NumChannels: 2,
		Precision:   2,
	}

	// Return our Opus streamer that reads from the WebSocket buffer.
	streamer := NewOpusStreamer(p.wsBuffer)

	// Since it's a live streaming buffer, seeking isn't natively supported on the Streamer interface directly,
	// but we implement a dummy StreamSeekCloser here to satisfy beep's requirements.
	duration := time.Duration(0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Storefront is required for Catalog search. We default to "us", but ideally we'd get the user's storefront.
	res, _, errApi := p.apiClient.Catalog.GetSong(ctx, "us", id, nil)
	if errApi == nil && res != nil && len(res.Data) > 0 {
		duration = time.Duration(res.Data[0].Attributes.DurationInMillis) * time.Millisecond
	} else if errApi != nil {
		log.Printf("applemusic: failed to fetch track duration for %s: %v", id, errApi)
	}

	wrapped := &noopSeeker{
		Streamer: streamer,
		b:        p.browser,
		duration: duration,
	}

	// Provide the actual duration so the UI progress bar works!
	return wrapped, format, duration, nil
}

// noopSeeker wraps the OpusStreamer to satisfy beep.StreamSeekCloser and delegates seeking to Chrome.
type noopSeeker struct {
	beep.Streamer
	b        *Browser
	duration time.Duration
	pos      int
}

func (n *noopSeeker) Seek(p int) error {
	// p is the sample position. We convert it back to seconds to seek in Apple Music JS.
	secs := p / 48000
	err := n.b.Seek(secs)
	if err == nil {
		n.pos = p
	}
	return err
}

func (n *noopSeeker) Position() int {
	return n.pos
}

func (n *noopSeeker) Len() int {
	// Dummy huge length for live stream, or actual length if known
	return int(n.duration.Seconds()) * 48000
}

func (n *noopSeeker) Close() error {
	// We pause playback in the browser when the stream is closed
	return n.b.Pause()
}

type authTransport struct {
	base      http.RoundTripper
	devToken  string
	userToken string
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	
	bearer := t.devToken
	if !strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
		bearer = "Bearer " + bearer
	}
	req2.Header.Set("Authorization", bearer)
	req2.Header.Set("Music-User-Token", t.userToken)
	
	if req2.Header.Get("Origin") == "" {
		req2.Header.Set("Origin", "https://music.apple.com")
	}
	if req2.Header.Get("Referer") == "" {
		req2.Header.Set("Referer", "https://music.apple.com/")
	}
	
	return t.base.RoundTrip(req2)
}
