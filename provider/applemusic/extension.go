package applemusic

import (
	"fmt"
	"os"
	"path/filepath"
)

const manifestJSON = `{
  "manifest_version": 3,
  "name": "cliamp Apple Music Audio Capture",
  "version": "1.0",
  "permissions": ["tabCapture", "activeTab", "scripting", "offscreen"],
  "host_permissions": ["*://music.apple.com/*"],
  "background": {
    "service_worker": "background.js"
  }
}`

const backgroundJS = `// background.js

// Listen for messages from the externally injected content script (via Go)
chrome.runtime.onMessageExternal.addListener((request, sender, sendResponse) => {
  if (request.action === "startCapture" && sender.tab) {
    console.log("Received startCapture request for tab:", sender.tab.id);
    
    // Request a MediaStreamId for the current tab
    chrome.tabCapture.getMediaStreamId({ targetTabId: sender.tab.id }, (streamId) => {
      if (chrome.runtime.lastError || !streamId) {
        console.error("Failed to get media stream ID:", chrome.runtime.lastError);
        return;
      }
      console.log("Got streamId:", streamId);
      
      // Setup the offscreen document
      setupOffscreenDocument(streamId);
    });
  }
});

async function setupOffscreenDocument(streamId) {
  // Check if an offscreen document already exists
  const existingContexts = await chrome.runtime.getContexts({
    contextTypes: ['OFFSCREEN_DOCUMENT'],
    documentUrls: [chrome.runtime.getURL('offscreen.html')]
  });

  if (existingContexts.length === 0) {
    // Create new offscreen document
    await chrome.offscreen.createDocument({
      url: 'offscreen.html',
      reasons: ['USER_MEDIA'],
      justification: 'Recording audio for playback in cliamp'
    });
  }

  // Send the streamId to the offscreen document so it can start recording
  chrome.runtime.sendMessage({
    type: 'START_RECORDING',
    streamId: streamId
  });
}
`

const offscreenHTML = `<!DOCTYPE html>
<html>
<head>
  <title>cliamp Audio Capture Offscreen Document</title>
</head>
<body>
  <script src="offscreen.js"></script>
</body>
</html>
`

const offscreenJS = `// offscreen.js

let websocket;
let mediaRecorder;

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.type === 'START_RECORDING') {
    startRecording(message.streamId);
  }
});

async function startRecording(streamId) {
  try {
    // Connect to the local WebSocket server running in cliamp
    websocket = new WebSocket("ws://127.0.0.1:41234/audio");
    
    websocket.onopen = () => {
      console.log("WebSocket connection opened.");
    };

    websocket.onerror = (error) => {
      console.error("WebSocket error:", error);
    };

    // Use the streamId to get the actual tab audio stream
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        mandatory: {
          chromeMediaSource: 'tab',
          chromeMediaSourceId: streamId
        }
      },
      video: false
    });

    console.log("Successfully got user media stream");

    // Initialize the MediaRecorder
    // We capture audio as Opus/WebM which can be sent incrementally
    mediaRecorder = new MediaRecorder(stream, {
      mimeType: "audio/webm;codecs=opus",
      audioBitsPerSecond: 128000
    });

    // Send data chunks to the WebSocket server
    mediaRecorder.ondataavailable = (event) => {
      if (event.data && event.data.size > 0) {
        websocket.send(event.data);
      }
    };

    // Request data every 100ms
    mediaRecorder.start(100);

  } catch (error) {
    console.error("Error starting recording:", error);
  }
}
`

// WriteExtensionToDisk writes the necessary Chrome extension files to a temporary
// directory and returns its absolute path.
func WriteExtensionToDisk() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home dir: %w", err)
	}

	extPath := filepath.Join(home, ".config", "cliamp", "applemusic-extension")
	if err := os.MkdirAll(extPath, 0755); err != nil {
		return "", fmt.Errorf("could not create extension directory: %w", err)
	}

	files := map[string]string{
		"manifest.json": manifestJSON,
		"background.js": backgroundJS,
		"offscreen.html": offscreenHTML,
		"offscreen.js":  offscreenJS,
	}

	for name, content := range files {
		path := filepath.Join(extPath, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", name, err)
		}
	}

	return extPath, nil
}
