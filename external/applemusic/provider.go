package applemusic

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"

	"cliamp/applog"
	"cliamp/config"
	"cliamp/external/applemusic/engineclient"
	"cliamp/internal/amprotocol"
	"cliamp/playlist"
	"cliamp/provider"
)

var (
	_ playlist.Provider       = (*Provider)(nil)
	_ provider.Searcher       = (*Provider)(nil)
	_ provider.CustomStreamer = (*Provider)(nil)
	_ provider.Closer         = (*Provider)(nil)
)

const providerRequestTimeout = 20 * time.Second

// Provider implements playlist.Provider and provider.Searcher for Apple Music.
// It exposes catalog playlists/charts and opens tracks externally in Apple Music.
type Provider struct {
	client *apiClient
	engine *engineclient.EngineClient
	cfg    config.AppleMusicConfig

	mu            sync.Mutex
	playlistCache []playlist.PlaylistInfo
	trackCache    map[string][]playlist.Track
}

func NewFromConfig(cfg config.AppleMusicConfig) *Provider {
	if !cfg.IsSet() {
		return nil
	}

	p := &Provider{
		client:     newAPIClient(cfg.WebBearerToken, cfg.MediaUserToken, cfg.Storefront),
		engine:     engineclient.New(cfg.EnginePath, cfg.WebBearerToken, cfg.MediaUserToken, cfg.Storefront, cfg.Debug),
		cfg:        cfg,
		trackCache: make(map[string][]playlist.Track),
	}
	go p.handleEngineEvents()
	return p
}

func (p *Provider) handleEngineEvents() {
	for ev := range p.engine.Events() {
		switch ev.Type {
		case amprotocol.TypeError:
			applog.Error("Apple Music Engine error: %s", ev.Message)
		case amprotocol.TypeTrackChanged:
			applog.Info("Apple Music: track changed to %s", ev.Track)
		}
	}
}

func (p *Provider) Name() string { return "Apple Music" }

func (p *Provider) URISchemes() []string { return []string{"applemusic:"} }

func (p *Provider) NewStreamer(uri string) (beep.StreamSeekCloser, beep.Format, time.Duration, error) {
	// uri is "applemusic:https://music.apple.com/..."
	// We need the track ID or the full URL for the engine.
	// For now, pass the whole URI (stripped of prefix).
	trackURL := strings.TrimPrefix(uri, "applemusic:")

	if err := p.engine.SetQueue([]string{trackURL}, 0); err != nil {
		return nil, beep.Format{}, 0, fmt.Errorf("apple music: set queue: %w", err)
	}

	if err := p.engine.Play(); err != nil {
		return nil, beep.Format{}, 0, fmt.Errorf("apple music: play: %w", err)
	}

	s := p.engine.Streamer().(*engineclient.PCMStreamer)
	return s, s.Format(), 0, nil
}

func (p *Provider) Close() {
	if p.engine != nil {
		p.engine.Close()
	}
}

func (p *Provider) Playlists() ([]playlist.PlaylistInfo, error) {
	p.mu.Lock()
	if p.playlistCache != nil {
		cached := p.playlistCache
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), providerRequestTimeout)
	defer cancel()

	playlists, err := p.client.ChartPlaylists(ctx, 50)
	if err != nil {
		return nil, fmt.Errorf("apple music: playlists: %w", err)
	}

	p.mu.Lock()
	p.playlistCache = playlists
	p.mu.Unlock()
	return playlists, nil
}

func (p *Provider) Tracks(playlistID string) ([]playlist.Track, error) {
	playlistID = strings.TrimSpace(playlistID)
	if playlistID == "" {
		return nil, fmt.Errorf("apple music: empty playlist id")
	}

	p.mu.Lock()
	if cached, ok := p.trackCache[playlistID]; ok {
		p.mu.Unlock()
		return cached, nil
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), providerRequestTimeout)
	defer cancel()

	tracks, err := p.client.PlaylistTracks(ctx, playlistID)
	if err != nil {
		return nil, fmt.Errorf("apple music: tracks: %w", err)
	}

	p.mu.Lock()
	p.trackCache[playlistID] = tracks
	p.mu.Unlock()
	return tracks, nil
}

func (p *Provider) SearchTracks(ctx context.Context, query string, limit int) ([]playlist.Track, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}

	tracks, err := p.client.SearchSongs(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("apple music: search: %w", err)
	}
	return tracks, nil
}
