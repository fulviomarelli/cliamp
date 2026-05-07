package applemusic

import (
	"io"
)

// OpusStreamer wraps an io.Reader containing an Opus/WebM stream and
// decodes it into a beep.Streamer.
type OpusStreamer struct {
	reader io.Reader
	err    error
}

// NewOpusStreamer creates a new OpusStreamer.
func NewOpusStreamer(r io.Reader) *OpusStreamer {
	s := &OpusStreamer{
		reader: r,
	}
	
	// Since we haven't implemented the real Opus decoder yet, we must drain 
	// the websocket pipe asynchronously so we don't block the audio pipeline
	// or the Chrome extension.
	go func() {
		_, _ = io.Copy(io.Discard, r)
	}()
	
	return s
}

// Stream decodes the next batch of audio samples.
// It fills the provided samples buffer with 2-channel float64 audio data.
func (s *OpusStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if s.err != nil {
		return 0, false
	}

	// For now, we stub this out by returning silence. To keep the visualizer 
	// running and the player advancing without blocking, we just zero out the buffer.
	for i := range samples {
		samples[i][0] = 0
		samples[i][1] = 0
	}

	return len(samples), true
}

// Err returns the last encountered error, if any.
func (s *OpusStreamer) Err() error {
	return s.err
}

// Close closes the underlying reader if it implements io.ReadCloser.
func (s *OpusStreamer) Close() error {
	if rc, ok := s.reader.(io.ReadCloser); ok {
		return rc.Close()
	}
	return nil
}
