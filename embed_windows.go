//go:build windows

package main

import _ "embed"

// yt-dlp is released under The Unlicense (Public Domain).
//
//go:embed embedded/yt-dlp.exe
var embeddedYtDlp []byte

const embeddedYtDlpName = "yt-dlp.exe"
