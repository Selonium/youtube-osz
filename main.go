package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func parseVideoPrompt(input string) bool {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return true
	}
	return !(strings.HasPrefix(s, "n") || strings.HasPrefix(s, "no"))
}

func buildOSUContent(title, artist string, includeVideo bool) string {
	if title == "" {
		title = "video"
	}
	if artist == "" {
		artist = "Unknown Artist"
	}

	v := ""
	if includeVideo {
		v = "Video,0,\"video.mp4\"\n"
	}

	return fmt.Sprintf(`osu file format v128

[General]
AudioFilename: audio.ogg
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
0,0,"bg.png",0,0
%s
[TimingPoints]
`, title, title, artist, artist, v)
}

func main() {
	defer bufio.NewReader(os.Stdin).ReadBytes('\n')

	p, _ := os.Executable()
	if runtime.GOOS == "windows" {
		f, _ := os.CreateTemp("", "*.reg")
		f.WriteString(fmt.Sprintf("Windows Registry Editor Version 5.00\n[HKEY_CURRENT_USER\\Software\\Classes\\ytosz]\n@=\"URL:yt-osz Protocol\"\n\"URL Protocol\"=\"\"\n[HKEY_CURRENT_USER\\Software\\Classes\\ytosz\\shell\\open\\command]\n@=\"\\\"%s\\\" \\\"%%1\\\"\"", strings.ReplaceAll(p, `\`, `\\`)))
		f.Close()
		exec.Command("reg", "import", f.Name()).Run()
		os.Remove(f.Name())
	} else if runtime.GOOS == "linux" {
		d := filepath.Join(os.Getenv("HOME"), ".local/share/applications")
		desk := filepath.Join(d, "yt-osz.desktop")
		if _, err := os.Stat(desk); err != nil {
			os.MkdirAll(d, 0755)
			os.WriteFile(desk, []byte(fmt.Sprintf("[Desktop Entry]\nType=Application\nName=yt-osz\nExec=\"%s\" %%u\nTerminal=true\nMimeType=x-scheme-handler/ytosz;\nNoDisplay=true\n", p)), 0644)
			exec.Command("xdg-mime", "default", "yt-osz.desktop", "x-scheme-handler/ytosz").Run()
		}
	}

	u := strings.Join(os.Args[1:], " ")
	if u == "" {
		fmt.Print("URL: ")
		u, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}

	u = strings.TrimSpace(u)
	u = strings.TrimPrefix(u, "ytosz://")
	u = strings.TrimPrefix(u, "ytosz:")
	u = strings.ReplaceAll(u, "https//", "https://")
	u = strings.ReplaceAll(u, "http//", "http://")
	if !strings.HasPrefix(u, "http") {
		u = "https://" + u
	}

	fmt.Print("Video? [Y/n]: ")
	ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	vid := parseVideoPrompt(ans)

	mOut, _ := exec.Command("yt-dlp", "--print-json", "--skip-download", u).Output()
	var m struct {
		Title string `json:"title"`
		Up    string `json:"uploader"`
	}
	json.Unmarshal(mOut, &m)

	t := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, m.Title)
	if t == "" {
		t = "video"
	}

	root := filepath.Join(os.TempDir(), "yt-osz")
	dir := filepath.Join(root, fmt.Sprintf("%s-%d", t, time.Now().Unix()))
	os.MkdirAll(dir, 0755)
	fmt.Println(dir)

	c := exec.Command("yt-dlp", "--write-thumbnail", "--skip-download", "--convert-thumbnails", "png", "-o", filepath.Join(dir, "bg"), u)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Run()

	c = exec.Command("yt-dlp", "-f", "bestaudio/best", "-o", filepath.Join(dir, "audio.%(ext)s"), u)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Run()

	if f, _ := filepath.Glob(filepath.Join(dir, "audio.*")); len(f) > 0 {
		for _, a := range f {
			if filepath.Ext(a) == ".ogg" {
				continue
			}
			c = exec.Command("ffmpeg", "-y", "-i", a, "-vn", "-c:a", "libvorbis", "-q:a", "6", filepath.Join(dir, "audio.ogg"))
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			c.Run()
			os.Remove(a)
			break
		}
	}

	if vid {
		vt := filepath.Join(dir, "temp")
		c = exec.Command("yt-dlp", "-f", "bestvideo[height<=720][ext=mp4]/bestvideo[height<=720]", "-o", vt+".%(ext)s", u)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		c.Run()

		if f, _ := filepath.Glob(vt + ".*"); len(f) > 0 {
			c = exec.Command("ffmpeg", "-y", "-i", f[0], "-vf", "scale=-2:480", "-c:v", "libx264", "-crf", "28", "-tune", "animation", "-an", filepath.Join(dir, "video.mp4"))
			c.Stdout, c.Stderr = os.Stdout, os.Stderr
			c.Run()
			os.Remove(f[0])
		}
	}

	osu := buildOSUContent(t, m.Up, vid)
	os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s - %s.osu", m.Up, t)), []byte(osu), 0644)

	osz := filepath.Join(root, t+".osz")
	os.Remove(osz)
	zf, _ := os.Create(osz)
	w := zip.NewWriter(zf)

	filepath.Walk(dir, func(path string, i os.FileInfo, _ error) error {
		if i != nil && !i.IsDir() {
			f, _ := os.Open(path)
			h, _ := zip.FileInfoHeader(i)
			h.Name, h.Method = filepath.Base(path), zip.Deflate
			zw, _ := w.CreateHeader(h)
			io.Copy(zw, f)
			f.Close()
		}
		return nil
	})

	w.Close()
	zf.Close()

	if runtime.GOOS == "windows" {
		exec.Command("explorer", "/select,", osz).Run()
	} else if runtime.GOOS == "linux" {
		for _, cmd := range []string{"dolphin", "nautilus", "thunar", "nemo", "caja", "xdg-open"} {
			if exec.Command(cmd, root).Run() == nil {
				break
			}
		}
	} else if runtime.GOOS == "darwin" {
		exec.Command("open", root).Run()
	}
}
