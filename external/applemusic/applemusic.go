package applemusic

import (
	"context"
	"fmt"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/minchao/go-apple-music"

	"cliamp/playlist"
	"cliamp/provider"
)

// Compile-time interface checks.
var (
	_ playlist.Provider      = (*Provider)(nil)
	_ provider.Searcher      = (*Provider)(nil)
	_ provider.CustomStreamer = (*Provider)(nil)
	_ provider.Closer         = (*Provider)(nil)
)

type Provider struct {
	browser   *Browser
	wsBuffer  *AudioBuffer
	apiClient *applemusic.Client
}

func NewProvider() (*Provider, error) {
	extPath, err := WriteExtensionToDisk()
	if err != nil {
		return nil, err
	}

	wsBuffer := StartWSServer()
	browser := NewBrowser(extPath)
	
	if err := browser.Init(); err != nil {
		return nil, err
	}

	devToken, userToken, err := browser.GetTokens()
	if err != nil {
		// Provide guidance if tokens are missing
		return nil, fmt.Errorf("%w (you may need to run with a visible browser first to log in)", err)
	}

	tp := &applemusic.Transport{Token: devToken, MusicUserToken: userToken}
	client := applemusic.NewClient(tp.Client())

	// Initial capture trigger
	browser.TriggerCapture()

	return &Provider{
		browser:   browser,
		wsBuffer:  wsBuffer,
		apiClient: client,
	}, nil
}

func (p *Provider) Name() string {
	return "Apple Music"
}

func (p *Provider) Playlists() ([]playlist.PlaylistInfo, error) {
	// Fetch user's library playlists using the REST API
	results, _, err := p.apiClient.Me.GetAllLibraryPlaylists(context.Background(), nil)
	if err != nil {
		return nil, err
	}

	var infos []playlist.PlaylistInfo
	for _, pl := range results.Data {
		infos = append(infos, playlist.PlaylistInfo{
			ID:         pl.Id,
			Name:       pl.Attributes.Name,
			TrackCount: 0, 
		})
	}
	return infos, nil
}

func (p *Provider) Tracks(playlistID string) ([]playlist.Track, error) {
	// Fetch tracks for the given library playlist
	songs, _, err := p.apiClient.Me.GetLibraryPlaylistTracks(context.Background(), playlistID, nil)
	if err != nil {
		return nil, err
	}

	var tracks []playlist.Track
	for _, t := range songs {
		tracks = append(tracks, playlist.Track{
			Path:         "applemusic:track:" + t.Id,
			Title:        t.Attributes.Name,
			Artist:       t.Attributes.ArtistName,
			Album:        t.Attributes.AlbumName,
			DurationSecs: int(t.Attributes.DurationInMillis / 1000),
			Stream:       true,
		})
	}
	return tracks, nil
}

func (p *Provider) SearchTracks(ctx context.Context, query string, limit int) ([]playlist.Track, error) {
	opts := &applemusic.SearchOptions{Term: query, Limit: limit, Types: "songs"}
	results, _, err := p.apiClient.Catalog.Search(ctx, "us", opts)
	if err != nil {
		return nil, err
	}

	if results.Results.Songs == nil {
		return nil, nil
	}

	var tracks []playlist.Track
	for _, t := range results.Results.Songs.Data {
		tracks = append(tracks, playlist.Track{
			Path:         "applemusic:track:" + t.Id,
			Title:        t.Attributes.Name,
			Artist:       t.Attributes.ArtistName,
			Album:        t.Attributes.AlbumName,
			DurationSecs: int(t.Attributes.DurationInMillis / 1000),
			Stream:       true,
		})
	}
	return tracks, nil
}

func (p *Provider) URISchemes() []string {
	return []string{"applemusic:track:"}
}

func (p *Provider) NewStreamer(uri string) (beep.StreamSeekCloser, beep.Format, time.Duration, error) {
	trackID := uri[len("applemusic:track:"):]
	if err := p.browser.PlayTrack(trackID); err != nil {
		return nil, beep.Format{}, 0, err
	}

	// Apple Music streams are typically 44100Hz stereo.
	format := beep.Format{
		SampleRate:  beep.SampleRate(44100),
		NumChannels: 2,
		Precision:   2,
	}
	
	return &OpusStreamerWrapper{OpusStreamer: &OpusStreamer{input: p.wsBuffer}}, format, 0, nil
}

func (p *Provider) Close() {
	p.browser.Close()
}

// OpusStreamerWrapper implements beep.StreamSeekCloser by wrapping OpusStreamer.
type OpusStreamerWrapper struct {
	*OpusStreamer
}

func (w *OpusStreamerWrapper) Len() int { return 0 }
func (w *OpusStreamerWrapper) Position() int { return w.OpusStreamer.pos }
func (w *OpusStreamerWrapper) Seek(pos int) error { return nil }
