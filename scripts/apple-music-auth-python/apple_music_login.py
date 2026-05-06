"""
Apple Music Token Extractor for cliamp (Python/Playwright)

This script automates the capture of:
1. web_bearer_token (Authorization header)
2. media_user_token (Cookie)

Usage:
    pip install playwright tomlkit
    playwright install chromium
    python apple_music_login.py
"""

import os
import sys
from pathlib import Path
from playwright.sync_api import sync_playwright
import tomlkit

CONFIG_PATH = Path.home() / ".config" / "cliamp" / "config.toml"

def update_config(bearer, media_token):
    if not CONFIG_PATH.exists():
        print(f"Config not found at {CONFIG_PATH}. Creating new one.")
        CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
        doc = tomlkit.document()
    else:
        with open(CONFIG_PATH, "r") as f:
            doc = tomlkit.parse(f.read())

    if "apple_music" not in doc:
        doc.add("apple_music", tomlkit.table())
    
    am = doc["apple_music"]
    am["enabled"] = True
    am["web_bearer_token"] = bearer
    am["media_user_token"] = media_token
    if "storefront" not in am:
        am["storefront"] = "us"

    with open(CONFIG_PATH, "w") as f:
        f.write(tomlkit.dumps(doc))
    print(f"Successfully updated {CONFIG_PATH}")

def run():
    tokens = {"bearer": None, "media": None}

    print("Launching browser... Sign in to Apple Music when prompted.")
    with sync_playwright() as p:
        # Launch visible browser
        browser = p.chromium.launch(headless=False)
        context = browser.new_context()
        page = context.new_page()

        def handle_request(request):
            auth = request.headers.get("authorization")
            # Look for any catalog or search request that has the Bearer token
            if auth and auth.startswith("Bearer ") and "apple.com" in request.url:
                if tokens["bearer"] != auth:
                    tokens["bearer"] = auth
                    print("Captured Web Bearer Token.")

        page.on("request", handle_request)
        
        try:
            page.goto("https://music.apple.com/login")

            # Polling loop to check for the cookie
            print("Waiting for login and media-user-token cookie...")
            count = 0
            while not (tokens["bearer"] and tokens["media"]):
                cookies = context.cookies()
                for c in cookies:
                    if c["name"] == "media-user-token":
                        tokens["media"] = c["value"]
                        print("Captured Media User Token.")
                
                if tokens["bearer"] and tokens["media"]:
                    break
                
                page.wait_for_timeout(1000)
                count += 1
                if count > 300: # 5 minute timeout
                    print("Timeout waiting for tokens. Did you log in?")
                    sys.exit(1)

            print("Tokens captured. Closing browser.")
            browser.close()
            update_config(tokens["bearer"], tokens["media"])
        except Exception as e:
            print(f"Error: {e}")
            browser.close()
            sys.exit(1)

if __name__ == "__main__":
    run()
