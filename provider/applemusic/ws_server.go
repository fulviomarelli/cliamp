package applemusic

import (
	"io"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// AudioBuffer provides a thread-safe buffer for incoming audio chunks.
// It implements io.Reader and io.Writer.
type AudioBuffer struct {
	pr *io.PipeReader
	pw *io.PipeWriter
}

func NewAudioBuffer() *AudioBuffer {
	pr, pw := io.Pipe()
	return &AudioBuffer{
		pr: pr,
		pw: pw,
	}
}

// Read implements io.Reader. It reads from the internal pipe.
func (ab *AudioBuffer) Read(p []byte) (n int, err error) {
	return ab.pr.Read(p)
}

// Write implements io.Writer. It writes to the internal pipe.
func (ab *AudioBuffer) Write(p []byte) (n int, err error) {
	return ab.pw.Write(p)
}

// Close closes both ends of the pipe.
func (ab *AudioBuffer) Close() error {
	ab.pw.Close()
	return ab.pr.Close()
}

var upgrader = websocket.Upgrader{
	// Allow any origin since this is a local extension connecting
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// StartWSServer starts the local WebSocket server to receive audio chunks
// from the Chrome Extension. It returns an AudioBuffer which can be read from.
func StartWSServer() *AudioBuffer {
	buffer := NewAudioBuffer()

	http.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("applemusic: failed to upgrade websocket: %v\n", err)
			return
		}
		defer conn.Close()

		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("applemusic: websocket error: %v\n", err)
				}
				break
			}
			
			// We expect binary messages containing the WebM/Opus chunks
			if messageType == websocket.BinaryMessage {
				_, wErr := buffer.Write(p)
				if wErr != nil {
					log.Printf("applemusic: failed to write to audio buffer: %v\n", wErr)
					break
				}
			}
		}
	})

	go func() {
		// Listen on localhost only to ensure security
		err := http.ListenAndServe("127.0.0.1:41234", nil)
		if err != nil {
			log.Printf("applemusic: websocket server failed: %v\n", err)
		}
	}()

	return buffer
}
