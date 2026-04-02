package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jhawk7/av-parser-service/internal/common"
	"github.com/jhawk7/av-parser-service/internal/mqttclient"
	"github.com/jhawk7/av-parser-service/internal/storage"
	"github.com/lrstanley/go-ytdlp"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

const (
	TMP_VID_FOLDER   = "./yt-tmp/"
	TMP_AUDIO_FOLDER = "./audio-tmp/"
)

var (
	avChan        chan mqttclient.AVMsg
	config        *common.Config
	mqttConsumer  mqttclient.IMQTTConsumer
	storageClient *storage.StorageClient
)

func main() {
	config = common.LoadConfig()
	storageClient = storage.InitStorageClient(config)
	mqttConsumer = mqttclient.InitClient(config)
	avChan = make(chan mqttclient.AVMsg)

	router := gin.Default()
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	router.GET("/jobs", func(c *gin.Context) {
		jobs := storageClient.GetAllJobs(c)

		c.JSON(http.StatusOK, gin.H{
			"response": jobs,
		})
	})

	// kicks off mqtt consumer to read mqtt messages and add to channel for processing
	go mqttConsumer.Listen(avChan)
	go AVParseHandler(avChan)

	router.Run(":8888")
}

func AVParseHandler(avChan <-chan mqttclient.AVMsg) {
	for avmsg := range avChan {
		storageClient.StoreRequest(&avmsg)
		ytUrl := avmsg.Url

		audioFlag, videoFlag := false, false
		if strings.ToLower(avmsg.Type) == "audio" {
			audioFlag = true
		} else if strings.ToLower(avmsg.Type) == "video" {
			videoFlag = true
		} else {
			err := fmt.Errorf("invalid av type flag: %v", avmsg.Type)
			common.LogError(err, false)
			continue
		}

		if videoFlag {
			fmt.Println("saving video only..")
		} else {
			fmt.Println("saving audio only..")
		}

		ctx := context.TODO()
		if err := downloadContent(ytUrl, videoFlag, ctx); err != nil {
			avmsg.Status = "failed"
			storageClient.UpdateRequest(&avmsg)
			continue
		}

		if err := parseAudio(audioFlag); err != nil {
			avmsg.Status = "failed"
			storageClient.UpdateRequest(&avmsg)
			continue
		}

		transferFiles(audioFlag, videoFlag)
		avmsg.Status = "completed"
		storageClient.UpdateRequest(&avmsg)
		fmt.Println("completed processing av request with id: ", avmsg.Id)
	}
}

func downloadContent(ytUrl string, videoFlag bool, ctx context.Context) error {
	fmt.Printf("retrieving video file from url %s\n", ytUrl)

	// If yt-dlp isn't installed yet, download and cache it for further use.
	ytdlp.MustInstall(ctx, nil)

	if osErr := os.MkdirAll(TMP_VID_FOLDER, os.ModePerm); osErr != nil {
		err := fmt.Errorf("failed to make tmp video dir; [err: %v]", osErr)
		common.LogError(err, false)
		return err
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
		common.LogError(err, false)
		return dlErr
	}

	fmt.Println("vid download complete.")
	return nil
}

func parseAudio(audioFlag bool) error {
	if !audioFlag {
		fmt.Println("audio parsing skipped.")
		return nil
	}

	fmt.Println("parsing audio from video..")

	if osErr := os.MkdirAll(TMP_AUDIO_FOLDER, os.ModePerm); osErr != nil {
		err := fmt.Errorf("failed to make tmp audio dir; [err: %v]", osErr)
		common.LogError(err, true)
	}

	entries, osErr := os.ReadDir(TMP_VID_FOLDER)
	if osErr != nil {
		err := fmt.Errorf("failed to read dir; [err: %v]", osErr)
		common.LogError(err, true)
	}

	if len(entries) == 0 {
		err := fmt.Errorf("no files in dir")
		common.LogError(err, true)
	}

	filename := entries[0].Name()
	audioFilename := strings.Split(filename, ".")[0] + ".mp3"

	streamErr := ffmpeg.Input(fmt.Sprintf("%s%s", TMP_VID_FOLDER, filename)).
		Output(fmt.Sprintf("%s%s", TMP_AUDIO_FOLDER, audioFilename), ffmpeg.KwArgs{"q:a": 0, "map": "a", "loglevel": "error"}).
		OverWriteOutput().ErrorToStdOut().Run()

	if streamErr != nil {
		err := fmt.Errorf("failed to parse video to audio file; [err: %v]", streamErr)
		common.LogError(err, false)
		return err
	}

	fmt.Println("av parsing complete.")
	return nil
}

func transferFiles(audioFlag, videoFlag bool) {
	copyFiles := func(srcFile, dstDir string) {
		entries, _ := os.ReadDir(srcFile)
		for _, entry := range entries {
			src, srcErr := os.Open(fmt.Sprintf("%s%s", srcFile, entry.Name()))
			common.LogError(srcErr, true)
			dst, dstErr := os.Create(dstDir + entry.Name())
			common.LogError(dstErr, true)

			defer func() {
				src.Close()
				dst.Close()
			}()

			if _, cpErr := io.Copy(dst, src); cpErr != nil {
				err := fmt.Errorf("failed to copy files; [err: %v]", cpErr)
				common.LogError(err, true)
			}
		}
	}

	if audioFlag {
		fmt.Println("transferring audio files..")
		copyFiles(TMP_AUDIO_FOLDER, "audio_archive/")
	}

	if videoFlag {
		fmt.Println("transferring video files..")
		copyFiles(TMP_VID_FOLDER, "video_archive/")
	}

	fmt.Println("transfer complete")
	cleanup()
}

func cleanup() {
	fmt.Println("cleaning up..")
	os.RemoveAll(TMP_AUDIO_FOLDER)
	os.RemoveAll(TMP_VID_FOLDER)
}
