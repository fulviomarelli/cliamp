package applemusic

import (
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// AudioBuffer implements io.ReadWriter using an io.Pipe.
type AudioBuffer struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	once   sync.Once
}

func (b *AudioBuffer) Read(p []byte) (n int, err error) {
	return b.reader.Read(p)
}

func (b *AudioBuffer) Write(p []byte) (n int, err error) {
	return b.writer.Write(p)
}

func (b *AudioBuffer) Close() error {
	b.once.Do(func() {
		b.writer.Close()
	})
	return nil
}

// StartWSServer starts the WebSocket server and returns the AudioBuffer.
func StartWSServer() *AudioBuffer {
	pr, pw := io.Pipe()
	buf := &AudioBuffer{reader: pr, writer: pw}
	
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	http.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			buf.Write(msg)
		}
	})

	go http.ListenAndServe("127.0.0.1:41234", nil)
	return buf
}
