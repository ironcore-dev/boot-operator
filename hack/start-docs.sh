#!/usr/bin/env bash

npm ci

npx vitepress dev docs --host 0.0.0.0 &
VITEPRESS_PID=$!

handle_sigint() {
    echo "Stopping VitePress dev server..."
    kill -TERM "$VITEPRESS_PID"
    wait "$VITEPRESS_PID"
    exit 0
}

trap handle_sigint SIGINT

wait "$VITEPRESS_PID"

