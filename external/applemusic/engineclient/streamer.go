package engineclient

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
)

// PCMStreamer reads raw 16-bit LE stereo PCM from an io.Reader and
// implements beep.Streamer.
type PCMStreamer struct {
	r      io.Reader
	format beep.Format

	mu   sync.Mutex
	err  error
	buf  []byte
	pool *sync.Pool
}

func NewPCMStreamer(r io.Reader, sampleRate beep.SampleRate) *PCMStreamer {
	return &PCMStreamer{
		r: r,
		format: beep.Format{
			SampleRate:  sampleRate,
			NumChannels: 2,
			Precision:   2, // 16-bit
		},
		buf: make([]byte, 4096), // 1024 samples * 2 channels * 2 bytes
		pool: &sync.Pool{
			New: func() any {
				return make([]byte, 4096)
			},
		},
	}
}

func (s *PCMStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return 0, false
	}

	totalRead := 0
	needed := len(samples) * 4 // 2 channels * 2 bytes per sample

	// Ensure our internal buffer is large enough
	if len(s.buf) < needed {
		s.buf = make([]byte, needed)
	}

	for totalRead < needed {
		rn, err := s.r.Read(s.buf[totalRead:needed])
		if rn > 0 {
			totalRead += rn
		}
		if err != nil {
			if err != io.EOF {
				s.err = err
			}
			break
		}
	}

	// Convert bytes to float64 samples
	n = totalRead / 4
	for i := 0; i < n; i++ {
		left := int16(binary.LittleEndian.Uint16(s.buf[i*4 : i*4+2]))
		right := int16(binary.LittleEndian.Uint16(s.buf[i*4+2 : i*4+4]))
		samples[i][0] = float64(left) / 32768.0
		samples[i][1] = float64(right) / 32768.0
	}

	return n, n > 0 || totalRead > 0
}

func (s *PCMStreamer) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *PCMStreamer) Len() int {
	return 0 // live stream
}

func (s *PCMStreamer) Position() int {
	return 0 // not easily tracked for live stream
}

func (s *PCMStreamer) Seek(pos int) error {
	return nil // Seeking is handled via control channel command to engine
}

func (s *PCMStreamer) Close() error {
	if closer, ok := s.r.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (s *PCMStreamer) Format() beep.Format {
	return s.format
}

func (s *PCMStreamer) Duration() time.Duration {
	return 0 // unknown for live stream
}
