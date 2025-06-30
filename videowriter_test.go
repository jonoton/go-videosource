//go:build config

package videosource

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jonoton/go-sharedmat"

	"gocv.io/x/gocv"
)

func TestVideoWriter(t *testing.T) {
	camName := "cam8"
	inFps := 8
	outFps := 30
	startWriteFrameCount1 := inFps * 6
	startWriteFrameCount2 := inFps * 30
	closeWriteFrameCount := inFps * 14
	shutdownFrameCount := inFps * 30

	writerBufferSec := 0
	writerMaxPreSec := 4
	writerTimeoutSec := 1
	writerMaxSec := 20
	writerOutFps := 8

	reader := NewVideoReader(NewFileSource(camName, "/Videos/"+camName+".mp4"), inFps, outFps)
	images := reader.Start()

	saveDir := filepath.Clean("/Videos/videowriter-test") + string(filepath.Separator)
	os.MkdirAll(saveDir, os.ModePerm)
	writer := NewVideoWriter(camName, saveDir, "mp4v", "mp4", writerBufferSec, writerMaxPreSec, writerTimeoutSec, writerMaxSec, writerOutFps, true, true, false, ActivityImage)
	writer.Start()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		outputStatsSub := reader.GetOutputStatsSub()
		defer outputStatsSub.Unsubscribe()
		for msg := range outputStatsSub.Ch {
			frameStats := msg.Data
			fmt.Println("Source Output Stats: " + frameStats.String())
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		writerStatsSub := writer.GetStatsSub()
		defer writerStatsSub.Unsubscribe()
		for msg := range writerStatsSub.Ch {
			frameStats := msg.Data
			fmt.Println("Writer Output Stats: " + frameStats.String())
		}
	}()

	window := gocv.NewWindow("TestVideoWriter")
	window.ResizeWindow(600, 400)
	defer window.Close()
	frameCount := 0
	for img := range images {
		frameCount++
		mat := img.SharedMat.Mat
		window.IMShow(mat)
		window.WaitKey(5)

		writer.Send(*NewProcessedImage(img))

		// continuous writing
		// writer.Trigger()

		// trigger after duration of frames
		if frameCount >= startWriteFrameCount1 {
			writer.Trigger()
		}
		if frameCount >= startWriteFrameCount2 {
			writer.Trigger()
		}
		fmt.Println("SharedMat Profile Count:", sharedmat.SharedMatProfile.Count())

		if frameCount >= closeWriteFrameCount {
			writer.Close()
		}

		if frameCount >= shutdownFrameCount {
			reader.Stop()
			break
		}
	}
	reader.Stop()
	reader.Wait()
	window.WaitKey(500)

	writer.Close()
	writer.Wait()

	wg.Wait()

	// check final stats directly
	finalReadStats := reader.GetFrameStatsOutput()
	finalWriteStats := writer.GetFrameStats()
	fmt.Println("Source Output Stats: " + finalReadStats.String())
	fmt.Println("Writer Output Stats: " + finalWriteStats.String())

	// check sharedmat profile count at the end of test
	fmt.Println("SharedMat Profile Count:", sharedmat.SharedMatProfile.Count())
	if sharedmat.SharedMatProfile.Count() > 0 {
		t.Errorf("Expected SharedMat Profile Count to be zero, but got %d", sharedmat.SharedMatProfile.Count())
	}
}
