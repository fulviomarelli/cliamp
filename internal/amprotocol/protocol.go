package amprotocol

type MessageType string

const (
	TypeSetQueue     MessageType = "set_queue"
	TypePlay         MessageType = "play"
	TypePause        MessageType = "pause"
	TypeStop         MessageType = "stop"
	TypeSeek         MessageType = "seek"
	TypeStatus       MessageType = "status"
	TypeTrackChanged MessageType = "track_changed"
	TypeError        MessageType = "error"
)

type Command struct {
	Type            MessageType `json:"type"`
	Tracks          []string    `json:"tracks,omitempty"`
	StartIndex      int         `json:"start_index,omitempty"`
	PositionSeconds float64     `json:"position_seconds,omitempty"`
}

type Event struct {
	Type     MessageType `json:"type"`
	State    string      `json:"state,omitempty"` // playing, paused, stopped
	Track    string      `json:"track,omitempty"`
	Position float64     `json:"position,omitempty"`
	Duration float64     `json:"duration,omitempty"`
	Message  string      `json:"message,omitempty"` // for errors
}
