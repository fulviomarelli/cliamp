package applemusic

import (
	"os"
	"path/filepath"
)

const (
	manifestJSON = `{
  "manifest_version": 3,
  "name": "Cliamp Apple Music Capture",
  "version": "1.0",
  "permissions": ["tabCapture", "activeTab", "scripting", "offscreen"],
  "host_permissions": ["*://music.apple.com/*"],
  "background": {
    "service_worker": "background.js"
  },
  "externally_connectable": {
    "matches": ["*://music.apple.com/*"]
  }
}`

	backgroundJS = `
chrome.runtime.onMessageExternal.addListener(async (message, sender, sendResponse) => {
  if (message.action === "startCapture") {
    const streamId = await chrome.tabCapture.getMediaStreamId({
      targetTabId: sender.tab.id
    });
    
    await chrome.offscreen.createDocument({
      url: 'offscreen.html',
      reasons: ['USER_MEDIA'],
      justification: 'Capture tab audio for CLI player'
    });

    chrome.runtime.sendMessage({
      target: 'offscreen',
      type: 'START_RECORDING',
      streamId: streamId
    });
  }
});
`

	offscreenHTML = `<!DOCTYPE html><html><head><script src="offscreen.js"></script></head><body></body></html>`

	offscreenJS = `
chrome.runtime.onMessage.addListener(async (message) => {
  if (message.target !== 'offscreen' || message.type !== 'START_RECORDING') return;

  const stream = await navigator.mediaDevices.getUserMedia({
    audio: {
      mandatory: {
        chromeMediaSource: 'tab',
        chromeMediaSourceId: message.streamId
      }
    },
    video: false
  });

  // Connect to speakers so user hears music + visualizer works
  const audioContext = new AudioContext();
  const source = audioContext.createMediaStreamSource(stream);
  source.connect(audioContext.destination);

  const recorder = new MediaRecorder(stream, { 
    mimeType: 'audio/webm;codecs=opus',
    audioBitsPerSecond: 128000 
  });

  const ws = new WebSocket('ws://127.0.0.1:41234/audio');

  ws.onopen = () => {
    recorder.ondataavailable = async (e) => {
      if (e.data.size > 0) {
        ws.send(await e.data.arrayBuffer());
      }
    };
    recorder.start(100);
  };
});
`
)

// WriteExtensionToDisk writes the embedded extension files to a temporary directory.
func WriteExtensionToDisk() (string, error) {
	home, _ := os.UserHomeDir()
	extPath := filepath.Join(home, ".config", "cliamp", "applemusic-extension")
	
	if err := os.MkdirAll(extPath, 0755); err != nil {
		return "", err
	}

	files := map[string]string{
		"manifest.json":  manifestJSON,
		"background.js":  backgroundJS,
		"offscreen.html": offscreenHTML,
		"offscreen.js":   offscreenJS,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(extPath, name), []byte(content), 0644); err != nil {
			return "", err
		}
	}

	return extPath, nil
}
