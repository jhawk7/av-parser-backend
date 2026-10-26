package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"strings"

	"github.com/gin-contrib/cors"
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
	common.InitLogger()
	config = common.LoadConfig()
	storageClient = storage.InitStorageClient(config)
	mqttConsumer = mqttclient.InitClient(config)
	avChan = make(chan mqttclient.AVMsg)

	// Forces go-ytdlp to check and download the absolute newest version from GitHub
	ytdlp.MustInstall(context.TODO(), &ytdlp.InstallOptions{})

	router := gin.Default()
	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET"},
	}))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	streamer := router.Group("/stream")
	{
		streamer.Use(func(c *gin.Context) {
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			c.Writer.Header().Set("Cache-Control", "no-cache")
			c.Writer.Header().Set("Connection", "keep-alive")
			c.Next()
		})

		streamer.GET("/jobs", StreamJobs)
	}

	router.GET("/jobs", func(c *gin.Context) {
		jobs := storageClient.GetAllJobs(c)
		c.JSON(http.StatusOK, gin.H{
			"response": jobs,
		})
	})

	go mqttConsumer.Listen(avChan)
	go AVParseHandler(avChan)

	router.Run(":8888")
}

func StreamJobs(c *gin.Context) {
	common.LogInfo("client connected to stream jobs endpoint")
	c.Stream(func(w io.Writer) bool {
		time.Sleep(30 * time.Second)
		recentJobs := storageClient.GetRecentJobs(c)

		c.SSEvent("message", gin.H{"response": recentJobs})
		common.LogInfo(fmt.Sprintf("streamed %d recent jobs to client", len(recentJobs)))

		if len(recentJobs) == 0 || storageClient.PendingCount == 0 {
			return false // No recent jobs to send; close the stream
		}

		return true
	})
}

func AVParseHandler(avChan <-chan mqttclient.AVMsg) {
	for avmsg := range avChan {
		storageClient.StoreRequest(&avmsg)
		ytUrl := avmsg.Url

		audioFlag, videoFlag := false, false

		switch avmsg.Type {
		case "both":
			audioFlag = true
			videoFlag = true
			common.LogInfo("saving both audio and video..")
		case "audio":
			audioFlag = true
			common.LogInfo("saving audio only")
		case "video":
			videoFlag = true
			common.LogInfo("saving video only")
		default:
			err := fmt.Errorf("invalid av type flag: %v", avmsg.Type)
			common.LogError(err, false)
			continue
		}

		ctx := context.TODO()
		if err := downloadContent(ytUrl, videoFlag, ctx); err != nil {
			reason := fmt.Sprintf("failed to download content; [err: %v]", err)
			storageClient.UpdateRequest(&avmsg, storage.JOB_FAILED, reason)
			continue
		}

		if err := parseAudio(audioFlag); err != nil {
			reason := fmt.Sprintf("failed to parse audio; [err: %v]", err)
			storageClient.UpdateRequest(&avmsg, storage.JOB_FAILED, reason)
			continue
		}

		if err := transferFiles(audioFlag, videoFlag); err != nil {
			reason := fmt.Sprintf("failed to transfer files; [err: %v]", err)
			storageClient.UpdateRequest(&avmsg, storage.JOB_FAILED, reason)
			continue
		}

		storageClient.UpdateRequest(&avmsg, storage.JOB_COMPLETED, "")
		common.LogInfo(fmt.Sprintf("completed processing av request with id: %v", avmsg.Id))
	}
}

func downloadContent(ytUrl string, videoFlag bool, ctx context.Context) error {
	fmt.Printf("retrieving video file from url %s\n", ytUrl)

	if osErr := os.MkdirAll(TMP_VID_FOLDER, os.ModePerm); osErr != nil {
		err := fmt.Errorf("failed to make tmp video dir; [err: %v]", osErr)
		common.LogError(err, false)
		return err
	}

	// Alpine Docker installs bun to /usr/local/bin/bun by default
	var dl *ytdlp.Command
	if videoFlag {
		dl = ytdlp.New().
			JsRuntimes("deno:/usr/bin/deno").
			FormatSort("vcodec:h264,res:1080,ext:mp4:m4a").
			RecodeVideo("mp4").
			Output(TMP_VID_FOLDER + "%(extractor)s - %(title)s.%(ext)s")
	} else {
		dl = ytdlp.New().
			JsRuntimes("deno:/usr/bin/deno").
			Output(TMP_VID_FOLDER + "%(extractor)s - %(title)s.%(ext)s")
	}

	args := []string{ytUrl, "--no-playlist", "--progress", "--update"}
	_, dlErr := dl.Run(ctx, args...)
	if dlErr != nil {
		err := fmt.Errorf("failed to download content; [err: %v]", dlErr)
		common.LogError(err, false)
		return dlErr
	}

	common.LogInfo("vid download complete.")
	return nil
}

func parseAudio(audioFlag bool) error {
	if !audioFlag {
		common.LogInfo("audio parsing skipped.")
		return nil
	}

	common.LogInfo("parsing audio from video..")

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

	common.LogInfo("av parsing complete.")
	return nil
}

func transferFiles(audioFlag, videoFlag bool) (err error) {
	copyFiles := func(srcFile, dstDir string) error {
		entries, _ := os.ReadDir(srcFile)
		for _, entry := range entries {
			src, srcErr := os.Open(fmt.Sprintf("%s%s", srcFile, entry.Name()))
			if srcErr != nil {
				return srcErr
			}
			dst, dstErr := os.Create(dstDir + entry.Name())
			if dstErr != nil {
				return dstErr
			}

			defer func() {
				src.Close()
				dst.Close()
			}()

			if _, cpErr := io.Copy(dst, src); cpErr != nil {
				err := fmt.Errorf("failed to copy files; [err: %v]", cpErr)
				common.LogError(err, true)
			}
		}
		return nil
	}

	if audioFlag {
		common.LogInfo("transferring audio files..")
		if err := copyFiles(TMP_AUDIO_FOLDER, "audio_archive/"); err != nil {
			return err
		}
	}

	if videoFlag {
		common.LogInfo("transferring video files..")
		if err := copyFiles(TMP_VID_FOLDER, "video_archive/"); err != nil {
			return err
		}
	}

	common.LogInfo("transfer complete")
	cleanup()
	return nil
}

func cleanup() {
	common.LogInfo("cleaning up..")
	os.RemoveAll(TMP_AUDIO_FOLDER)
	os.RemoveAll(TMP_VID_FOLDER)
}
