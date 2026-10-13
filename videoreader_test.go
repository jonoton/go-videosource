package videosource

import (
	"testing"
	"time"
)

func TestVideoReaderZeroFramesDoesNotDeadlock(t *testing.T) {
	reader := NewVideoReader(NewFileSource("nonexistent", "/nonexistent/video.mp4"), 10, 10)
	images := reader.Start()

	done := make(chan struct{})
	go func() {
		for img := range images {
			img.Cleanup()
		}
		reader.Stop()
		reader.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("VideoReader timed out waiting to finish when zero frames were read")
	}
}

func TestVideoWriterZeroFramesDoesNotDeadlock(t *testing.T) {
	writer := NewVideoWriter("test-writer", t.TempDir(), "mp4v", "mp4", 0, 2, 10, 10, 8, true, true, false, ActivityImage)
	if writer == nil {
		t.Fatal("Expected non-nil VideoWriter")
	}
	writer.Start()

	done := make(chan struct{})
	go func() {
		writer.Close()
		writer.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("VideoWriter timed out waiting to finish when zero frames were written")
	}
}
