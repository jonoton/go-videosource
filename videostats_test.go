package videosource

import (
	"sync"
	"testing"
	"time"
)

func TestVideoStatsConcurrentAccess(t *testing.T) {
	vs := NewVideoStats()
	vs.Start()
	defer vs.Close()

	var wg sync.WaitGroup
	// Concurrently add accepted and dropped
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				vs.AddAccepted()
				time.Sleep(time.Millisecond)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				vs.AddDropped()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Concurrently read GetFrameStats
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				stats := vs.GetFrameStats()
				if stats == nil {
					t.Errorf("expected non-nil stats")
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()
}
