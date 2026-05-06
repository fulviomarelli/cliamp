package applemusic

import (
	"context"
	"strconv"
	"strings"

	appleapi "github.com/minchao/go-apple-music"

	"cliamp/playlist"
)

type apiClient struct {
	client     *appleapi.Client
	storefront string
}

func newAPIClient(webBearerToken, mediaUserToken, storefront string) *apiClient {
	storefront = strings.ToLower(strings.TrimSpace(storefront))
	if storefront == "" {
		storefront = "us"
	}
	return &apiClient{
		client:     appleapi.NewClient(newHTTPClient(webBearerToken, mediaUserToken)),
		storefront: storefront,
	}
}

func (c *apiClient) SearchSongs(ctx context.Context, query string, limit int) ([]playlist.Track, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 25 {
		limit = 25
	}

	results, _, err := c.client.Catalog.Search(ctx, c.storefront, &appleapi.SearchOptions{
		Term:  query,
		Types: "songs",
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	if results == nil || results.Results.Songs == nil {
		return nil, nil
	}

	out := make([]playlist.Track, 0, len(results.Results.Songs.Data))
	for _, song := range results.Results.Songs.Data {
		if track, ok := songToTrack(song); ok {
			out = append(out, track)
		}
	}
	return out, nil
}

func (c *apiClient) ChartPlaylists(ctx context.Context, limit int) ([]playlist.PlaylistInfo, error) {
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}

	charts, _, err := c.client.Catalog.GetAllCharts(ctx, c.storefront, &appleapi.ChartsOptions{
		Types: "playlists",
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	if charts == nil || charts.Results.Playlists == nil {
		return nil, nil
	}

	seen := make(map[string]struct{})
	var out []playlist.PlaylistInfo
	for _, chart := range *charts.Results.Playlists {
		section := strings.TrimSpace(chart.Name)
		if section == "" {
			section = "Charts"
		}
		for _, pl := range chart.Playlists.Data {
			if pl.Id == "" {
				continue
			}
			if _, ok := seen[pl.Id]; ok {
				continue
			}
			seen[pl.Id] = struct{}{}
			name := strings.TrimSpace(pl.Attributes.Name)
			if name == "" {
				name = pl.Id
			}
			out = append(out, playlist.PlaylistInfo{
				ID:      pl.Id,
				Name:    name,
				Section: section,
			})
		}
	}

	// Also fetch User Library playlists if we have a user token
	libPlaylists, _, err := c.client.Me.GetAllLibraryPlaylists(ctx, &appleapi.PageOptions{Limit: limit})
	if err == nil && libPlaylists != nil {
		for _, pl := range libPlaylists.Data {
			if pl.Id == "" {
				continue
			}
			if _, ok := seen[pl.Id]; ok {
				continue
			}
			seen[pl.Id] = struct{}{}
			name := strings.TrimSpace(pl.Attributes.Name)
			if name == "" {
				name = pl.Id
			}
			out = append(out, playlist.PlaylistInfo{
				ID:      "lib:" + pl.Id,
				Name:    name,
				Section: "Library",
			})
		}
	}

	return out, nil
}

func (c *apiClient) PlaylistTracks(ctx context.Context, playlistID string) ([]playlist.Track, error) {
	if strings.HasPrefix(playlistID, "lib:") {
		return c.LibraryPlaylistTracks(ctx, strings.TrimPrefix(playlistID, "lib:"))
	}

	res, _, err := c.client.Catalog.GetPlaylist(ctx, c.storefront, playlistID, &appleapi.Options{
		Include: "tracks",
	})
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.Data) == 0 {
		return nil, nil
	}

	var out []playlist.Track
	for _, raw := range res.Data[0].Relationships.Tracks.Data {
		parsed, err := raw.Parse()
		if err != nil || parsed == nil {
			continue
		}
		switch v := parsed.(type) {
		case *appleapi.Song:
			if track, ok := songToTrack(*v); ok {
				out = append(out, track)
			}
		case *appleapi.MusicVideo:
			if track, ok := musicVideoToTrack(*v); ok {
				out = append(out, track)
			}
		}
	}

	return out, nil
}

func (c *apiClient) LibraryPlaylistTracks(ctx context.Context, playlistID string) ([]playlist.Track, error) {
	res, _, err := c.client.Me.GetLibraryPlaylist(ctx, playlistID, &appleapi.Options{
		Include: "tracks",
	})
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.Data) == 0 {
		return nil, nil
	}

	var out []playlist.Track
	for _, raw := range res.Data[0].Relationships.Tracks.Data {
		parsed, err := raw.Parse()
		if err != nil || parsed == nil {
			continue
		}
		switch v := parsed.(type) {
		case *appleapi.LibrarySong:
			if track, ok := librarySongToTrack(*v); ok {
				out = append(out, track)
			}
		case *appleapi.Song:
			if track, ok := songToTrack(*v); ok {
				out = append(out, track)
			}
		}
	}
	return out, nil
}

func librarySongToTrack(song appleapi.LibrarySong) (playlist.Track, bool) {
	// Library songs might not have a public URL directly in attributes.
	// Future improvement: use catalogId from PlayParams if available to find URL.
	return playlist.Track{}, false
}

func songToTrack(song appleapi.Song) (playlist.Track, bool) {
	url := strings.TrimSpace(song.Attributes.URL)
	if url == "" {
		return playlist.Track{}, false
	}

	return playlist.Track{
		Path:         "applemusic:" + url,
		Title:        song.Attributes.Name,
		Artist:       song.Attributes.ArtistName,
		Album:        song.Attributes.AlbumName,
		Year:         parseReleaseYear(song.Attributes.ReleaseDate),
		TrackNumber:  song.Attributes.TrackNumber,
		DurationSecs: int(song.Attributes.DurationInMillis / 1000),
	}, true
}

func musicVideoToTrack(video appleapi.MusicVideo) (playlist.Track, bool) {
	url := strings.TrimSpace(video.Attributes.URL)
	if url == "" {
		return playlist.Track{}, false
	}

	return playlist.Track{
		Path:         "applemusic:" + url,
		Title:        video.Attributes.Name,
		Artist:       video.Attributes.ArtistName,
		Year:         parseReleaseYear(video.Attributes.ReleaseDate),
		TrackNumber:  video.Attributes.TrackNumber,
		DurationSecs: int(video.Attributes.DurationInMillis / 1000),
	}, true
}

func parseReleaseYear(releaseDate string) int {
	if len(releaseDate) < 4 {
		return 0
	}
	year, err := strconv.Atoi(releaseDate[:4])
	if err != nil {
		return 0
	}
	return year
}
