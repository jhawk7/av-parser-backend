package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/lrstanley/go-ytdlp"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

const (
	TMP_VID_FOLDER   = "./yt-tmp/"
	TMP_AUDIO_FOLDER = "./audio-tmp/"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Missing url; Usage: 'go run main.go <url> [optional '-a|-v' flag to save audio or video only (both by default)]'")
		return
	}

	ytUrl := os.Args[1]
	audioFlag, videoFlag := true, true
	if len(os.Args) == 3 {
		switch os.Args[2] {
		case "-a":
			fmt.Println("saving audio only..")
			videoFlag = false
		case "-v":
			fmt.Println("saving video only..")
			audioFlag = false
		default:
			err := fmt.Errorf("invalid flag '%s'; use '-a' for audio only or '-v' for video only", os.Args[2])
			errorHandler(err, true)
		}
	}

	// prepare context to cancel on os interrupt - 'Ctrl+C'
	fmt.Println("press 'Ctrl+C' to terminate.")
	ctx, cancel := context.WithCancel(context.Background())
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	defer func() {
		signal.Stop(signalCh)
		cancel()
		cleanup()
	}()

	go func() {
		select {
		case <-signalCh:
			cancel()
			cleanup()
			panic("program terminated by user")
		case <-ctx.Done():
		}
	}()

	downloadContent(ytUrl, videoFlag, ctx)
	parseAV(audioFlag)
	transferFiles(audioFlag, videoFlag)
	fmt.Println("fin.")
}

func downloadContent(ytUrl string, videoFlag bool, ctx context.Context) {
	fmt.Printf("retrieving video file from url %s\n", ytUrl)

	// If yt-dlp isn't installed yet, download and cache it for further use.
	ytdlp.MustInstall(ctx, nil)

	if osErr := os.MkdirAll(TMP_VID_FOLDER, os.ModePerm); osErr != nil {
		err := fmt.Errorf("failed to make tmp video dir; [err: %v]", osErr)
		errorHandler(err, true)
		return
	}

	// yt-dlp will get best available format by default, but we want mp4 if we're saving it
	var dl *ytdlp.Command
	if videoFlag {
		dl = ytdlp.New().
			FormatSort("vcodec:h264,res,ext:mp4:m4a").
			RecodeVideo("mp4").
			Output(TMP_VID_FOLDER + "%(extractor)s - %(title)s.%(ext)s")
	} else {
		dl = ytdlp.New().Output(TMP_VID_FOLDER + "%(extractor)s - %(title)s.%(ext)s")
	}

	args := []string{ytUrl, "--no-playlist", "--progress"}
	_, dlErr := dl.Run(ctx, args...)
	if dlErr != nil {
		err := fmt.Errorf("failed to download content; [err: %v]", dlErr)
		errorHandler(err, true)
	}

	fmt.Println("vid download complete.")
}

func parseAV(audioFlag bool) {
	if !audioFlag {
		fmt.Println("audio parsing skipped.")
		return
	}

	fmt.Println("parsing audio from video..")

	if osErr := os.MkdirAll(TMP_AUDIO_FOLDER, os.ModePerm); osErr != nil {
		err := fmt.Errorf("failed to make tmp audio dir; [err: %v]", osErr)
		errorHandler(err, true)
		return
	}

	entries, osErr := os.ReadDir(TMP_VID_FOLDER)
	if osErr != nil {
		err := fmt.Errorf("failed to read dir; [err: %v]", osErr)
		errorHandler(err, true)
		return
	}

	if len(entries) == 0 {
		err := fmt.Errorf("no files in dir")
		errorHandler(err, true)
		return
	}

	filename := entries[0].Name()
	audioFilename := strings.Split(filename, ".")[0] + ".mp3"

	streamErr := ffmpeg.Input(fmt.Sprintf("%s%s", TMP_VID_FOLDER, filename)).
		Output(fmt.Sprintf("%s%s", TMP_AUDIO_FOLDER, audioFilename), ffmpeg.KwArgs{"q:a": 0, "map": "a", "loglevel": "error"}).
		OverWriteOutput().ErrorToStdOut().Run()

	if streamErr != nil {
		err := fmt.Errorf("failed to parse video to audio file; [err: %v]", streamErr)
		errorHandler(err, true)
		return
	}

	fmt.Println("av parsing complete.")
}

func transferFiles(audioFlag, videoFlag bool) {
	vidArchive, isSet := os.LookupEnv("AV_VIDEO_STORAGE_DIR")
	if !isSet {
		err := fmt.Errorf("video storage dir not set")
		errorHandler(err, true)
	}

	audoArchive, isSet := os.LookupEnv("AV_AUDIO_STORAGE_DIR")
	if !isSet {
		err := fmt.Errorf("audio storage dir not set")
		errorHandler(err, true)
	}

	copyFiles := func(srcFile, dstDir string) {
		entries, _ := os.ReadDir(srcFile)
		for _, entry := range entries {
			src, srcErr := os.Open(fmt.Sprintf("%s%s", srcFile, entry.Name()))
			errorHandler(srcErr, true)
			dst, dstErr := os.Create(dstDir + entry.Name())
			errorHandler(dstErr, true)

			defer func() {
				src.Close()
				dst.Close()
			}()

			if _, cpErr := io.Copy(dst, src); cpErr != nil {
				err := fmt.Errorf("failed to copy files; [err: %v]", cpErr)
				errorHandler(err, true)
			}
		}
	}

	if audioFlag {
		fmt.Println("transferring audio files..")
		copyFiles(TMP_AUDIO_FOLDER, audoArchive)
	}

	if videoFlag {
		fmt.Println("transferring video files..")
		copyFiles(TMP_VID_FOLDER, vidArchive)
	}

	fmt.Println("transfer complete")
}

func cleanup() {
	fmt.Println("cleaning up..")
	os.RemoveAll(TMP_AUDIO_FOLDER)
	os.RemoveAll(TMP_VID_FOLDER)
}

func errorHandler(err error, fatal bool) {
	if err != nil {
		if fatal {
			panic(err)
		} else {
			fmt.Printf("error %v", err)
		}
	}
}
