package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVideoPrompt(t *testing.T) {
	if !parseVideoPrompt("") {
		t.Fatal("expected empty input to default to true")
	}
	if !parseVideoPrompt("y\n") {
		t.Fatal("expected 'y' to be true")
	}
	if parseVideoPrompt("n\n") {
		t.Fatal("expected 'n' to be false")
	}
	if parseVideoPrompt("no") {
		t.Fatal("expected 'no' to be false")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"ytosz://https://www.youtube.com/watch?v=dQw4w9WgXcQ":            "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"ytosz:https//www.youtube.com/watch?v=dQw4w9WgXcQ":               "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"ytosz://https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DdQw4w9WgXcQ": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"www.youtube.com/watch?v=dQw4w9WgXcQ":                            "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		`"https://youtu.be/dQw4w9WgXcQ"`:                                 "https://youtu.be/dQw4w9WgXcQ",
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := sanitizeFilename(`Artist/Name: "Test" |Song*?.`, "fallback")
	if got != "ArtistName Test Song" {
		t.Fatalf("unexpected sanitized filename: %q", got)
	}
	if empty := sanitizeFilename(`<>:"/\|?*...`, "video"); empty != "video" {
		t.Fatalf("expected fallback 'video', got %q", empty)
	}
}

func TestFindFilesWithPrefixInBracketedDir(t *testing.T) {
	base := t.TempDir()
	// Regression test: brackets in directory path broke filepath.Glob
	dir := filepath.Join(base, "The Larping Tombstone - Larping the Rooms [Green Version] (Looping The Rooms Remix)-1792492124")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	webmPath := filepath.Join(dir, "audio.webm")
	partPath := filepath.Join(dir, "audio.webm.part")
	if err := os.WriteFile(webmPath, []byte("dummy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partPath, []byte("partial"), 0644); err != nil {
		t.Fatal(err)
	}

	matches, err := findFilesWithPrefix(dir, "audio.")
	if err != nil {
		t.Fatalf("findFilesWithPrefix failed: %v", err)
	}
	if len(matches) != 1 || matches[0] != webmPath {
		t.Fatalf("expected [%q], got %v", webmPath, matches)
	}
}

func TestBuildOSUContent(t *testing.T) {
	content := buildOSUContent("Song [Green Version]", "Artist", true)
	if !strings.Contains(content, "AudioFilename: audio.ogg") {
		t.Error("missing AudioFilename: audio.ogg")
	}
	if !strings.Contains(content, `Video,0,"video.mp4"`) {
		t.Error("missing video event")
	}
}

func TestExtractBundledYtDlp(t *testing.T) {
	if len(embeddedYtDlp) == 0 {
		t.Skip("no embedded yt-dlp binary in this build environment")
	}
	path, err := extractBundledYtDlp()
	if err != nil {
		t.Fatalf("extractBundledYtDlp failed: %v", err)
	}
	if !fileExistsNonEmpty(path) {
		t.Fatalf("extracted binary does not exist or is empty: %s", path)
	}
}
