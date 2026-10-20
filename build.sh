#!/usr/bin/env bash
set -euo pipefail

mkdir -p embedded
curl -fL https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp_linux -o embedded/yt-dlp_linux
chmod +x embedded/yt-dlp_linux
curl -fL https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp.exe -o embedded/yt-dlp.exe

rm -f yt-osz yt-osz.exe
GOOS=windows GOARCH=amd64 go build -o yt-osz.exe .
go build -o yt-osz .
