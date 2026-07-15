rm yt-osz
rm yt-osz.exe
GOOS=windows GOARCH=amd64 go build -o yt-osz.exe main.go
go build -o yt-osz main.go