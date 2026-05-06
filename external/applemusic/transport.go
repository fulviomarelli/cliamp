package applemusic

import (
	"net/http"
	"strings"
)

// headerTransport is a custom http.RoundTripper that injects Apple Music
// authentication headers into every request.
type headerTransport struct {
	base           http.RoundTripper
	webBearerToken string
	mediaUserToken string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add Bearer token
	bearer := t.webBearerToken
	if !strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
		bearer = "Bearer " + bearer
	}
	req.Header.Set("Authorization", bearer)

	// Add Media User Token
	if t.mediaUserToken != "" {
		req.Header.Set("Music-User-Token", t.mediaUserToken)
	}

	// Some endpoints might require Origin or Referer for the web player
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "https://music.apple.com")
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", "https://music.apple.com/")
	}

	return t.base.RoundTrip(req)
}

func newHTTPClient(webBearerToken, mediaUserToken string) *http.Client {
	return &http.Client{
		Transport: &headerTransport{
			base:           http.DefaultTransport,
			webBearerToken: webBearerToken,
			mediaUserToken: mediaUserToken,
		},
	}
}
