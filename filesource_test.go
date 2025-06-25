//go:build config

package videosource

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jonoton/go-sharedmat"
	"gocv.io/x/gocv"
)

func TestFileSource(t *testing.T) {
	f := NewVideoReader(NewFileSource("cam5", "/Videos/cam5.mp4"), 8, 6)
	images := f.Start()
	defer f.Stop()

	go func() {
		tick := time.NewTicker(35 * time.Second)
	Loop:
		for {
			select {
			case <-tick.C:
				f.Stop()
				break Loop
			}
		}
		tick.Stop()
	}()

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		sourceStatsSub := f.GetSourceStatsSub()
		defer sourceStatsSub.Unsubscribe()
		for msg := range sourceStatsSub.Ch {
			frameStats := msg.Data
			fmt.Println("Input  Stats: " + frameStats.String())
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		outputStatsSub := f.GetOutputStatsSub()
		defer outputStatsSub.Unsubscribe()
		for msg := range outputStatsSub.Ch {
			frameStats := msg.Data
			fmt.Println("Output Stats: " + frameStats.String())
		}
	}()

	window := gocv.NewWindow("TestFileSource")
	window.ResizeWindow(600, 400)
	defer window.Close()
	for img := range images {
		mat := img.SharedMat.Mat
		window.IMShow(mat)
		window.WaitKey(5)
		img.Cleanup()
		fmt.Println("SharedMat Profile Count:", sharedmat.SharedMatProfile.Count())
	}
	f.Wait()
	window.WaitKey(500)
	wg.Wait()

	// pubsub stats should be closed and return nil without panic
	testPubInStats := f.GetStatsSource(200)
	testPubOutStats := f.GetStatsOutput(200)
	if !(testPubInStats == nil && testPubOutStats == nil) {
		t.Error("Both pubsub stats should be closed and not get anything")
	}

	// check final stats directly
	finalInStats := f.GetFrameStatsSource()
	finalOutStats := f.GetFrameStatsOutput()
	fmt.Println("Input  Stats: " + finalInStats.String())
	fmt.Println("Output Stats: " + finalOutStats.String())
	if finalInStats.AcceptedTotal != finalOutStats.AcceptedTotal+finalOutStats.DroppedTotal {
		t.Error("Input Accepted Total does not match output Accepted and Dropped Total")
	}

	// check sharedmat profile count at the end of test
	fmt.Println("SharedMat Profile Count:", sharedmat.SharedMatProfile.Count())
	if sharedmat.SharedMatProfile.Count() > 0 {
		t.Errorf("Expected SharedMat Profile Count to be zero, but got %d", sharedmat.SharedMatProfile.Count())
	}
}
