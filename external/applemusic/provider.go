package applemusic

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"cliamp/config"
	"cliamp/playlist"
	"cliamp/provider"
)

var (
	_ playlist.Provider = (*Provider)(nil)
	_ provider.Searcher = (*Provider)(nil)
)

const providerRequestTimeout = 20 * time.Second

// Provider implements playlist.Provider and provider.Searcher for Apple Music.
// It exposes catalog playlists/charts and opens tracks externally in Apple Music.
type Provider struct {
	client *apiClient

	mu            sync.Mutex
	playlistCache []playlist.PlaylistInfo
	trackCache    map[string][]playlist.Track
}

func NewFromConfig(cfg config.AppleMusicConfig) *Provider {
	if !cfg.IsSet() {
		return nil
	}

	return &Provider{
		client:     newAPIClient(cfg.WebBearerToken, cfg.MediaUserToken, cfg.Storefront),
		trackCache: make(map[string][]playlist.Track),
	}
}

func (p *Provider) Name() string { return "Apple Music" }

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
