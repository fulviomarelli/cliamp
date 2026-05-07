#!/usr/bin/env bash

echo "Cleaning Apple Music configuration and session data..."

rm -rf ~/cliamp-chrome-profile
rm -rf ~/.config/cliamp/applemusic-extension

echo "Done! You can now re-test the Apple Music login flow."
