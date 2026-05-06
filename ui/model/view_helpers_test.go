package model

import (
	"strings"
	"testing"

	"cliamp/playlist"
)

func TestFormatTrackTime(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{0, ""},
		{-5, ""},
		{1, "0:01"},
		{59, "0:59"},
		{60, "1:00"},
		{222, "3:42"},
		{3599, "59:59"},
		{3600, "1:00:00"},
		{3661, "1:01:01"},
		{36000, "10:00:00"},
	}
	for _, tt := range tests {
		if got := formatTrackTime(tt.secs); got != tt.want {
			t.Errorf("formatTrackTime(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestFormatPlaylistDuration(t *testing.T) {
	tests := []struct {
		secs int
		want string
	}{
		{0, ""},
		{-1, ""},
		{45, "45s"},
		{59, "59s"},
		{60, "1m"},
		{600, "10m"},
		{3540, "59m"},
		{3600, "1h"},
		{3660, "1h 1m"},
		{7200, "2h"},
		{7320, "2h 2m"},
	}
	for _, tt := range tests {
		if got := formatPlaylistDuration(tt.secs); got != tt.want {
			t.Errorf("formatPlaylistDuration(%d) = %q, want %q", tt.secs, got, tt.want)
		}
	}
}

func TestPlaylistLabel(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		info   playlist.PlaylistInfo
		want   string
	}{
		{
			"name only when both unknown",
			"  ",
			playlist.PlaylistInfo{Name: "Mix"},
			"  Mix",
		},
		{
			"track count only",
			"> ",
			playlist.PlaylistInfo{Name: "Mix", TrackCount: 12},
			"> Mix · 12 tracks",
		},
		{
			"duration only",
			"  ",
			playlist.PlaylistInfo{Name: "Mix", DurationSecs: 3660},
			"  Mix · 1h 1m",
		},
		{
			"both",
			"  ",
			playlist.PlaylistInfo{Name: "Mix", TrackCount: 12, DurationSecs: 2700},
			"  Mix · 12 tracks · 45m",
		},
	}
	for _, tt := range tests {
		got := playlistLabel(tt.prefix, tt.info)
		if got != tt.want {
			t.Errorf("%s: playlistLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFormatTrackRow(t *testing.T) {
	// No duration: returns just "N. title".
	row := formatTrackRow(3, "Song", 0)
	if row != "3. Song" {
		t.Errorf("no-duration row = %q, want %q", row, "3. Song")
	}

	// With duration: ends with the time string.
	row = formatTrackRow(3, "Song", 222)
	if !strings.HasSuffix(row, "3:42") {
		t.Errorf("with-duration row %q does not end with %q", row, "3:42")
	}
	if !strings.HasPrefix(row, "3. Song") {
		t.Errorf("with-duration row %q does not start with %q", row, "3. Song")
	}
}

func TestProviderKeyForShortcut(t *testing.T) {
	tests := map[string]string{
		"S": "spotify",
		"M": "applemusic",
		"N": "navidrome",
		"P": "plex",
		"J": "jellyfin",
		"E": "emby",
		"Y": "yt",
		"C": "soundcloud",
		"L": "local",
		"R": "radio",
		"x": "",
		"":  "",
	}
	for in, want := range tests {
		if got := providerKeyForShortcut(in); got != want {
			t.Errorf("providerKeyForShortcut(%q) = %q, want %q", in, got, want)
		}
	}
}
