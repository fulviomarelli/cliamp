package applemusic

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
)

// OpusStreamer decodes WebM/Opus data from an io.Reader using ffmpeg.
type OpusStreamer struct {
	input  io.Reader
	cmd    *exec.Cmd
	pipe   io.ReadCloser
	reader *bufio.Reader
	err    error
	pos    int
}

func (s *OpusStreamer) start() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is required to decode Apple Music audio")
	}

	// We use s16le, 44100Hz, stereo.
	cmd := exec.Command("ffmpeg",
		"-i", "pipe:0",
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", "44100",
		"-ac", "2",
		"-loglevel", "error",
		"pipe:1",
	)
	cmd.Stdin = s.input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd = cmd
	s.pipe = stdout
	s.reader = bufio.NewReaderSize(stdout, 64*1024)
	return nil
}

func (s *OpusStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if s.reader == nil {
		if err := s.start(); err != nil {
			s.err = err
			return 0, false
		}
	}

	buf := make([]byte, 4) // s16le stereo = 4 bytes per sample
	for i := range samples {
		_, err := io.ReadFull(s.reader, buf)
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				s.err = err
			}
			return i, i > 0
		}
		
		left := int16(binary.LittleEndian.Uint16(buf[0:2]))
		right := int16(binary.LittleEndian.Uint16(buf[2:4]))
		samples[i] = [2]float64{float64(left) / 32768, float64(right) / 32768}
		n++
		s.pos++
	}
	return n, true
}

func (s *OpusStreamer) Err() error {
	return s.err
}

func (s *OpusStreamer) Close() error {
	if s.pipe != nil {
		s.pipe.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if closer, ok := s.input.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
