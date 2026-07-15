# Youtube to OSZ converter
Automatically downloads a youtube video + audio + thumbnail + metadata 

Outputs a complete .osz (osu custom map file) ready to start mapping

## Dependencies

Ensure ffmpeg and yt-dlp are available in PATH

## aur
```
yay -Sy ffmpeg yt-dlp
```

## apt
```
apt update && apt install ffmpeg yt-dlp -y
```

### Windows
```
winget install Gyan.FFmpeg yt-dlp.yt-dlp --source winget
```

## Usage

You can run this program in a few different ways depending on what is most convenient for you.

1. Download the yt-osz binary for your platform, or build from source

2. Double click/execute the downloaded file at least once to register the URI handler

#### CLI

3. Double click/execute the downloaded file

4. paste youtube URL

#### URI

3. In any browser navigate to a youtube music video

4. Add this in the URL box
```
ytosz://
```
<img width="655" height="60" alt="image" src="https://github.com/user-attachments/assets/ca479f35-1529-4404-ab0d-4610f6a09f92" />


#### Terminal

3. Open a terminal

4. Pass a URL as the argument

```
./yt-osz https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

### Finally

5. Confirm choice to save video or just audio + metadata

6. Drag and drop the output .osz file into osu
