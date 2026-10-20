package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

func parseVideoPrompt(input string) bool {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return true
	}
	return !(strings.HasPrefix(s, "n") || strings.HasPrefix(s, "no"))
}

func cleanMetadataValue(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return fallback
	}
	return s
}

func sanitizeFilename(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " .")
	runes := []rune(s)
	if len(runes) > 100 {
		s = strings.Trim(string(runes[:100]), " .")
	}
	if s == "" {
		return fallback
	}
	return s
}

func normalizeURL(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.Trim(u, `"'`)
	u = strings.TrimSpace(u)

	lower := strings.ToLower(u)
	if strings.HasPrefix(lower, "ytosz://") {
		u = u[len("ytosz://"):]
	} else if strings.HasPrefix(lower, "ytosz:") {
		u = u[len("ytosz:"):]
	}
	u = strings.TrimSpace(u)

	// Handle URL-encoded payloads from browsers (e.g. ytosz://https%3A%2F%2F...)
	if strings.Contains(u, "%3A") || strings.Contains(u, "%3a") || strings.Contains(u, "%2F") || strings.Contains(u, "%2f") {
		if decoded, err := url.QueryUnescape(u); err == nil && decoded != "" {
			u = decoded
		}
	}

	u = strings.ReplaceAll(u, "https//", "https://")
	u = strings.ReplaceAll(u, "http//", "http://")
	u = strings.TrimPrefix(u, "//")

	if u != "" && !strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://") {
		u = "https://" + u
	}
	return u
}

func buildOSUContent(title, artist string, includeVideo bool) string {
	videoFile := ""
	if includeVideo {
		videoFile = "video.mp4"
	}
	return buildOSUContentWithFiles(title, artist, "audio.ogg", "bg.png", videoFile)
}

func buildOSUContentWithFiles(title, artist, audioFile, bgFile, videoFile string) string {
	title = cleanMetadataValue(title, "video")
	artist = cleanMetadataValue(artist, "Unknown Artist")
	if audioFile == "" {
		audioFile = "audio.ogg"
	}

	var events strings.Builder
	if bgFile != "" {
		events.WriteString(fmt.Sprintf("0,0,%q,0,0\n", bgFile))
	}
	if videoFile != "" {
		events.WriteString(fmt.Sprintf("Video,0,%q\n", videoFile))
	}

	return fmt.Sprintf(`osu file format v128

[General]
AudioFilename: %s
AudioLeadIn: 0
Countdown: 0
SampleSet: Auto
StackLeniency: 0.7
Mode: 0
LetterboxInBreaks: 0
WidescreenStoryboard: 1

[Metadata]
Title:%s
TitleUnicode:%s
Artist:%s
ArtistUnicode:%s
Creator:yt-osz
Version:Normal
BeatmapID:0
BeatmapSetID:-1

[Difficulty]
HPDrainRate:5
CircleSize:5
OverallDifficulty:5
ApproachRate:8
SliderMultiplier:1
SliderTickRate:1

[Events]
%s
[TimingPoints]
`, audioFile, title, title, artist, artist, events.String())
}

func findFilesWithPrefix(dir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".ytdl") || strings.HasSuffix(lower, ".temp") {
			continue
		}
		matches = append(matches, filepath.Join(dir, name))
	}
	return matches, nil
}

func fileExistsNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func extractBundledYtDlp() (string, error) {
	if len(embeddedYtDlp) == 0 {
		return "", errors.New("no bundled yt-dlp binary available in this build")
	}

	sum := sha256.Sum256(embeddedYtDlp)
	shortHash := hex.EncodeToString(sum[:8])

	binDir := filepath.Join(os.TempDir(), "yt-osz", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", err
	}

	ext := filepath.Ext(embeddedYtDlpName)
	base := strings.TrimSuffix(embeddedYtDlpName, ext)
	targetPath := filepath.Join(binDir, fmt.Sprintf("%s-%s%s", base, shortHash, ext))

	if info, err := os.Stat(targetPath); err == nil && info.Size() == int64(len(embeddedYtDlp)) {
		if runtime.GOOS != "windows" {
			_ = os.Chmod(targetPath, 0755)
		}
		return targetPath, nil
	}

	tmpFile, err := os.CreateTemp(binDir, "yt-dlp-extract-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(embeddedYtDlp); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	if err := os.Chmod(tmpPath, 0755); err != nil && runtime.GOOS != "windows" {
		os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		os.Remove(tmpPath)
		// Another process may have extracted it concurrently
		if info, statErr := os.Stat(targetPath); statErr == nil && info.Size() == int64(len(embeddedYtDlp)) {
			return targetPath, nil
		}
		return "", err
	}

	return targetPath, nil
}

type ytDlpRunner struct {
	primary     string
	usedBundled bool
}

func newYtDlpRunner() (*ytDlpRunner, error) {
	if pathBin, err := exec.LookPath("yt-dlp"); err == nil && pathBin != "" {
		fmt.Printf("Using shared yt-dlp from PATH: %s\n", pathBin)
		return &ytDlpRunner{primary: pathBin, usedBundled: false}, nil
	}

	bundled, err := extractBundledYtDlp()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp not found in PATH and bundled fallback failed: %w", err)
	}
	fmt.Printf("Using bundled yt-dlp nightly: %s\n", bundled)
	return &ytDlpRunner{primary: bundled, usedBundled: true}, nil
}

func (r *ytDlpRunner) fallbackToBundled() bool {
	if r.usedBundled || len(embeddedYtDlp) == 0 {
		return false
	}
	bundled, err := extractBundledYtDlp()
	if err != nil {
		return false
	}
	fmt.Printf("Shared yt-dlp failed; falling back to bundled yt-dlp nightly: %s\n", bundled)
	r.primary = bundled
	r.usedBundled = true
	return true
}

func (r *ytDlpRunner) Output(args ...string) ([]byte, error) {
	c := exec.Command(r.primary, args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	if err != nil && r.fallbackToBundled() {
		c2 := exec.Command(r.primary, args...)
		c2.Stderr = os.Stderr
		return c2.Output()
	}
	return out, err
}

func (r *ytDlpRunner) Run(args ...string) error {
	c := exec.Command(r.primary, args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	err := c.Run()
	if err != nil && r.fallbackToBundled() {
		c2 := exec.Command(r.primary, args...)
		c2.Stdout, c2.Stderr = os.Stdout, os.Stderr
		return c2.Run()
	}
	return err
}

type videoMetadata struct {
	Title    string `json:"title"`
	Uploader string `json:"uploader"`
	Channel  string `json:"channel"`
	Creator  string `json:"creator"`
	Artist   string `json:"artist"`
}

func parseMetadataJSON(data []byte) (videoMetadata, error) {
	var meta videoMetadata
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) > 0 && line[0] == '{' {
			if err := json.Unmarshal(line, &meta); err == nil {
				return meta, nil
			}
		}
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &meta); err != nil {
		return meta, err
	}
	return meta, nil
}

func (m videoMetadata) bestArtist() string {
	for _, candidate := range []string{m.Artist, m.Uploader, m.Channel, m.Creator} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return "Unknown Artist"
}

func registerProtocolHandler() {
	p, err := os.Executable()
	if err != nil || p == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}

	if runtime.GOOS == "windows" {
		f, err := os.CreateTemp("", "*.reg")
		if err != nil {
			return
		}
		escaped := strings.ReplaceAll(p, `\`, `\\`)
		_, _ = f.WriteString(fmt.Sprintf("Windows Registry Editor Version 5.00\n[HKEY_CURRENT_USER\\Software\\Classes\\ytosz]\n@=\"URL:yt-osz Protocol\"\n\"URL Protocol\"=\"\"\n[HKEY_CURRENT_USER\\Software\\Classes\\ytosz\\shell\\open\\command]\n@=\"\\\"%s\\\" \\\"%%1\\\"\"", escaped))
		_ = f.Close()
		_ = exec.Command("reg", "import", f.Name()).Run()
		_ = os.Remove(f.Name())
	} else if runtime.GOOS == "linux" {
		home := os.Getenv("HOME")
		if home == "" {
			if h, err := os.UserHomeDir(); err == nil {
				home = h
			}
		}
		if home == "" {
			return
		}
		d := filepath.Join(home, ".local/share/applications")
		desk := filepath.Join(d, "yt-osz.desktop")
		expected := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=yt-osz\nExec=\"%s\" %%u\nTerminal=true\nMimeType=x-scheme-handler/ytosz;\nNoDisplay=true\n", p)
		existing, err := os.ReadFile(desk)
		if err != nil || string(existing) != expected {
			_ = os.MkdirAll(d, 0755)
			_ = os.WriteFile(desk, []byte(expected), 0644)
			_ = exec.Command("xdg-mime", "default", "yt-osz.desktop", "x-scheme-handler/ytosz").Run()
		}
	}
}

func downloadAndConvertThumbnail(ytdlp *ytDlpRunner, ffmpegPath, dir, targetURL string) string {
	_ = ytdlp.Run("--no-playlist", "--write-thumbnail", "--skip-download", "--convert-thumbnails", "png", "-o", filepath.Join(dir, "bg"), targetURL)

	bgPng := filepath.Join(dir, "bg.png")
	if fileExistsNonEmpty(bgPng) {
		if matches, err := findFilesWithPrefix(dir, "bg."); err == nil {
			for _, m := range matches {
				if filepath.Clean(m) != filepath.Clean(bgPng) {
					_ = os.Remove(m)
				}
			}
		}
		return "bg.png"
	}

	// Fallback: if yt-dlp left bg.webp / bg.jpg without converting to png, convert with ffmpeg
	matches, _ := findFilesWithPrefix(dir, "bg.")
	for _, src := range matches {
		c := exec.Command(ffmpegPath, "-y", "-i", src, "-frames:v", "1", bgPng)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		_ = c.Run()
		_ = os.Remove(src)
		if fileExistsNonEmpty(bgPng) {
			return "bg.png"
		}
	}
	return ""
}

func downloadAndConvertAudio(ytdlp *ytDlpRunner, ffmpegPath, dir, targetURL string) (string, error) {
	if err := ytdlp.Run("--no-playlist", "-f", "bestaudio/best", "-o", filepath.Join(dir, "audio.%(ext)s"), targetURL); err != nil {
		return "", fmt.Errorf("failed to download audio: %w", err)
	}

	matches, err := findFilesWithPrefix(dir, "audio.")
	if err != nil || len(matches) == 0 {
		return "", errors.New("downloaded audio file not found")
	}

	oggPath := filepath.Join(dir, "audio.ogg")
	var srcPath string
	for _, m := range matches {
		if strings.EqualFold(filepath.Ext(m), ".ogg") && fileExistsNonEmpty(m) {
			srcPath = m
			break
		}
	}
	if srcPath == "" {
		srcPath = matches[0]
	}

	if filepath.Clean(srcPath) != filepath.Clean(oggPath) {
		encoderAttempts := [][]string{
			{"-y", "-i", srcPath, "-vn", "-c:a", "libvorbis", "-q:a", "6", oggPath},
			{"-y", "-i", srcPath, "-vn", "-c:a", "vorbis", "-strict", "-2", "-q:a", "6", oggPath},
			{"-y", "-i", srcPath, "-vn", "-c:a", "libopus", "-b:a", "192k", oggPath},
		}

		converted := false
		for _, ffArgs := range encoderAttempts {
			_ = os.Remove(oggPath)
			c := exec.Command(ffmpegPath, ffArgs...)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err == nil && fileExistsNonEmpty(oggPath) {
				converted = true
				break
			}
		}

		finalAudioFile := "audio.ogg"
		finalAudioPath := oggPath
		if !converted {
			mp3Path := filepath.Join(dir, "audio.mp3")
			_ = os.Remove(mp3Path)
			c := exec.Command(ffmpegPath, "-y", "-i", srcPath, "-vn", "-c:a", "libmp3lame", "-q:a", "2", mp3Path)
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			if err := c.Run(); err == nil && fileExistsNonEmpty(mp3Path) {
				converted = true
				finalAudioFile = "audio.mp3"
				finalAudioPath = mp3Path
			}
		}

		if !converted {
			return "", errors.New("failed to convert audio with ffmpeg")
		}

		// Clean up all other audio.* source files (e.g. audio.webm, audio.m4a)
		if allAudio, err := findFilesWithPrefix(dir, "audio."); err == nil {
			for _, a := range allAudio {
				if filepath.Clean(a) != filepath.Clean(finalAudioPath) {
					_ = os.Remove(a)
				}
			}
		}
		return finalAudioFile, nil
	}

	return "audio.ogg", nil
}

func downloadAndConvertVideo(ytdlp *ytDlpRunner, ffmpegPath, dir, targetURL string) string {
	vt := filepath.Join(dir, "temp")
	formatSelector := "bestvideo[height<=720][ext=mp4]/bestvideo[height<=720]/best[height<=720]/bestvideo/best"
	if err := ytdlp.Run("--no-playlist", "-f", formatSelector, "-o", vt+".%(ext)s", targetURL); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: video download failed (%v), continuing without video.\n", err)
		return ""
	}

	matches, err := findFilesWithPrefix(dir, "temp.")
	if err != nil || len(matches) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: downloaded temp video file not found, continuing without video.")
		return ""
	}
	defer func() {
		// Always clean up temp.* files so they never end up inside the .osz archive
		if leftovers, err := findFilesWithPrefix(dir, "temp."); err == nil {
			for _, f := range leftovers {
				_ = os.Remove(f)
			}
		}
	}()

	srcVideo := matches[0]
	outVideo := filepath.Join(dir, "video.mp4")

	videoAttempts := [][]string{
		{"-y", "-i", srcVideo, "-vf", "scale=-2:480", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "28", "-tune", "animation", "-movflags", "+faststart", "-an", outVideo},
		{"-y", "-i", srcVideo, "-vf", "scale=-2:480", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", "-q:v", "5", "-an", outVideo},
	}

	for _, ffArgs := range videoAttempts {
		_ = os.Remove(outVideo)
		c := exec.Command(ffmpegPath, ffArgs...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		if err := c.Run(); err == nil && fileExistsNonEmpty(outVideo) {
			return "video.mp4"
		}
	}

	_ = os.Remove(outVideo)
	fmt.Fprintln(os.Stderr, "Warning: failed to convert video to mp4, continuing without video.")
	return ""
}

func createOSZArchive(dir, oszPath string) error {
	_ = os.Remove(oszPath)
	zf, err := os.Create(oszPath)
	if err != nil {
		return err
	}
	defer zf.Close()

	w := zip.NewWriter(zf)
	walkErr := filepath.Walk(dir, func(path string, i os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if i == nil || i.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		h, err := zip.FileInfoHeader(i)
		if err != nil {
			return err
		}
		h.Name, h.Method = filepath.Base(path), zip.Deflate
		zw, err := w.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(zw, f)
		return err
	})
	if walkErr != nil {
		_ = w.Close()
		return walkErr
	}
	return w.Close()
}

func openOutputDirectory(root, osz string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer", "/select,", osz).Run()
	} else if runtime.GOOS == "linux" {
		for _, cmd := range []string{"dolphin", "nautilus", "thunar", "nemo", "caja", "xdg-open"} {
			if exec.Command(cmd, root).Run() == nil {
				break
			}
		}
	} else if runtime.GOOS == "darwin" {
		_ = exec.Command("open", root).Run()
	}
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	defer func() {
		fmt.Println("\nPress Enter to exit...")
		_, _ = reader.ReadBytes('\n')
	}()

	registerProtocolHandler()

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: ffmpeg was not found in PATH. Please install ffmpeg first.")
		return
	}

	ytdlp, err := newYtDlpRunner()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	u := strings.Join(os.Args[1:], " ")
	if strings.TrimSpace(u) == "" {
		fmt.Print("URL: ")
		u, _ = reader.ReadString('\n')
	}

	u = normalizeURL(u)
	if u == "" {
		fmt.Fprintln(os.Stderr, "Error: empty URL provided.")
		return
	}

	fmt.Print("Video? [Y/n]: ")
	ans, _ := reader.ReadString('\n')
	vid := parseVideoPrompt(ans)

	fmt.Println("Fetching video metadata...")
	mOut, err := ytdlp.Output("--no-playlist", "--no-warnings", "--print-json", "--skip-download", u)
	var m videoMetadata
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: metadata fetch returned an error (%v), attempting to continue...\n", err)
	}
	if len(mOut) > 0 {
		m, _ = parseMetadataJSON(mOut)
	}

	title := sanitizeFilename(m.Title, "video")
	artist := sanitizeFilename(m.bestArtist(), "Unknown Artist")

	root := filepath.Join(os.TempDir(), "yt-osz")
	dir := filepath.Join(root, fmt.Sprintf("%s-%d", title, time.Now().Unix()))
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating working directory: %v\n", err)
		return
	}
	defer os.RemoveAll(dir)
	fmt.Println("Working directory:", dir)

	bgFile := downloadAndConvertThumbnail(ytdlp, ffmpegPath, dir, u)

	audioFile, err := downloadAndConvertAudio(ytdlp, ffmpegPath, dir, u)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing audio: %v\n", err)
		return
	}

	videoFile := ""
	if vid {
		videoFile = downloadAndConvertVideo(ytdlp, ffmpegPath, dir, u)
	}

	osu := buildOSUContentWithFiles(title, artist, audioFile, bgFile, videoFile)
	osuFilename := filepath.Join(dir, fmt.Sprintf("%s - %s.osu", artist, title))
	if err := os.WriteFile(osuFilename, []byte(osu), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing .osu file: %v\n", err)
		return
	}

	osz := filepath.Join(root, title+".osz")
	if err := createOSZArchive(dir, osz); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating .osz archive: %v\n", err)
		return
	}

	fmt.Println("Created:", osz)
	openOutputDirectory(root, osz)
}
