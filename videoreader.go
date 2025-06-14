package videosource

import (
	"sync"
	"time"

	chanLimiter "github.com/jonoton/go-chanlimiter"
	pubsubmutex "github.com/jonoton/go-pubsubmutex"
	log "github.com/sirupsen/logrus"
)

// VideoReader reads a VideoSource
type VideoReader struct {
	videoSource  VideoSource
	sourceStats  VideoStats
	outputStats  VideoStats
	done         chan bool
	cancel       chan bool
	cancelOnce   sync.Once
	MaxSourceFps int
	MaxOutputFps int
	Quality      int
}

// NewVideoReader creates a new VideoReader
func NewVideoReader(videoSource VideoSource, maxSourceFps int, maxOutputFps int) *VideoReader {
	if videoSource == nil || maxSourceFps <= 0 || maxOutputFps <= 0 {
		return nil
	}
	v := &VideoReader{
		videoSource:  videoSource,
		sourceStats:  *NewVideoStats(),
		outputStats:  *NewVideoStats(),
		done:         make(chan bool),
		cancel:       make(chan bool),
		MaxSourceFps: maxSourceFps,
		MaxOutputFps: maxOutputFps,
		Quality:      100,
	}
	return v
}

// SetQuality sets the Image quality
func (v *VideoReader) SetQuality(percent int) {
	if percent > 0 && percent < 100 {
		v.Quality = percent
	}
}

type statsImage struct {
	Image      Image
	VideoStats *VideoStats
}

func (s *statsImage) Cleanup() {
	s.Image.Cleanup()
	s.VideoStats.AddDropped()
}

func (v *VideoReader) getTickMs(fps int) time.Duration {
	tickMs := 5
	if fps > 0 {
		tickMs = 1000 / fps
	}
	return time.Duration(tickMs)
}

// Start runs the processes
func (v *VideoReader) Start() <-chan Image {
	images := make(chan Image)
	go func() {
		if !v.videoSource.Initialize() {
			log.Warnln("VideoReader could not initialize", v.videoSource.GetName())
		}
		inFps := v.MaxSourceFps
		inputLimiter := chanLimiter.New[*statsImage](inFps)

		outFps := v.MaxOutputFps
		outputLimiter := chanLimiter.New[*statsImage](outFps)

		go func() {
			defer outputLimiter.Stop()
			for statsImg := range inputLimiter.Output() {
				v.sourceStats.AddAccepted()
				outputLimiter.Send(&statsImage{Image: statsImg.Image, VideoStats: &v.outputStats})
			}
		}()

		go func() {
			defer inputLimiter.Stop()
			v.sourceStats.Start()
			tick := time.NewTicker(v.getTickMs(inFps) * time.Millisecond)
			defer tick.Stop()
		SourceLoop:
			for {
				select {
				case <-tick.C:
					done, image := v.videoSource.ReadImage()
					if done {
						image.Cleanup()
						log.Infoln("Done source", v.videoSource.GetName())
						break SourceLoop
					} else if image.IsFilled() {
						if v.Quality > 0 && v.Quality < 100 {
							image.ChangeQuality(v.Quality)
						}
						inputLimiter.Send(&statsImage{Image: image, VideoStats: &v.sourceStats})
					}
				case <-v.cancel:
					break SourceLoop
				}
			}
		}()

		v.outputStats.Start()
		for statsImg := range outputLimiter.Output() {
			if statsImg.Image.IsFilled() {
				images <- statsImg.Image
				v.outputStats.AddAccepted()
			}
		}

		v.videoSource.Cleanup()
		v.outputStats.Close()
		v.sourceStats.Close()
		close(images)
		close(v.done)
	}()

	return images
}

// Stop will stop the processes
func (v *VideoReader) Stop() {
	v.cancelOnce.Do(func() {
		close(v.cancel)
	})
}

// Wait for done
func (v *VideoReader) Wait() {
	<-v.done
}

// GetFrameStatsSource returns the FrameStats directly
func (v *VideoReader) GetFrameStatsSource() (result *FrameStats) {
	result = v.sourceStats.GetFrameStats()
	return
}

// GetStatsSource returns the FrameStats using pubsub
func (v *VideoReader) GetStatsSource(timeoutMs int) (result *FrameStats) {
	result = v.sourceStats.GetStats(timeoutMs)
	return
}

// GetSourceStatsSub returns the subscriber
func (v *VideoReader) GetSourceStatsSub() (result *pubsubmutex.Subscriber) {
	result = v.sourceStats.GetStatsSub()
	return
}

// GetFrameStatsOutput returns the FrameStats directly
func (v *VideoReader) GetFrameStatsOutput() (result *FrameStats) {
	result = v.outputStats.GetFrameStats()
	return
}

// GetStatsOutput returns the FrameStats using pubsub
func (v *VideoReader) GetStatsOutput(timeoutMs int) (result *FrameStats) {
	result = v.outputStats.GetStats(timeoutMs)
	return
}

// GetOutputStatsSub returns the subscriber
func (v *VideoReader) GetOutputStatsSub() (result *pubsubmutex.Subscriber) {
	result = v.outputStats.GetStatsSub()
	return
}
