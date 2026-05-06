# Apple Music Provider

The Apple Music provider in cliamp supports catalog searching and browsing. Due to DRM restrictions, it does not support direct streaming in the terminal; instead, choosing a track will open the official Apple Music web player or desktop app at that track's URL.

## Setup

Since cliamp uses the public web API directly, you do not need a paid Developer Account. However, you must extract two tokens from your browser while logged into the Apple Music web player.

### Option 1: Automated Login (Recommended)

Use one of the helper scripts in the `scripts/` directory to capture tokens automatically.

#### Python (Playwright)
```bash
cd scripts/apple-music-auth-python
pip install playwright tomlkit
playwright install chromium
python apple_music_login.py
```

#### Go (Chromedp)
```bash
cd scripts/apple-music-auth-go
go mod init apple-music-auth
go get github.com/chromedp/chromedp
go run apple_music_login.go
```

The script will open a browser. Log in normally. Once tokens are captured, the browser closes and your `config.toml` is updated automatically.

### Option 2: Manual Extraction (F12 Method)

If you prefer not to run automation scripts, you can extract tokens manually:

#### Step 1: Login
Go to [music.apple.com](https://music.apple.com) and log in.

#### Step 2: Extract `media-user-token`
1. Open Developer Tools (**F12**).
2. Go to **Application** (Chrome) or **Storage** (Firefox) -> **Cookies** -> `https://music.apple.com`.
3. Copy the value of `media-user-token`.
4. Paste it into your `config.toml` under `[apple_music]`.

#### Step 3: Extract `web_bearer_token`
1. In Developer Tools, go to the **Network** tab (Fetch/XHR filter).
2. Refresh the page.
3. Click any request starting with `catalog` or `search`.
4. Copy the `Authorization` request header value (e.g., `Bearer eyJh...`).
5. Paste it into your `config.toml` under `[apple_music]`.

## Configuration Summary

```toml
[apple_music]
enabled = true
web_bearer_token = "Bearer eyJh..."
media_user_token = "your-media-user-token-cookie"
storefront = "us" # default is us
```

## Important Notes

*   **Expiration:** These tokens are temporary. When search or browsing stops working, you will need to repeat the extraction steps.
*   **DRM:** cliamp cannot play Apple Music tracks directly. It uses your system's `open` (macOS) or `xdg-open` (Linux) command to launch the URL.
*   **User Library:** If `media_user_token` is valid, library features (Likes, Playlists) may become available in future updates.
