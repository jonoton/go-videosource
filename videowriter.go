package videosource

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	pubsubmutex "github.com/jonoton/go-pubsubmutex"
	ringbuffer "github.com/jonoton/go-ringbuffer"
	"github.com/jonoton/go-sharedmat"
	log "github.com/sirupsen/logrus"
	"gocv.io/x/gocv"
)

const topicWriterImages = "topic-writer-images"

// SaveImage will save an Image
func SaveImage(img Image, t time.Time, saveDirectory string, jpegQuality int, name string, title string, percentage string) (savePath string) {
	savePath = GetImageFilename(t, saveDirectory, name, title, percentage)
	jpgParams := []int{gocv.IMWriteJpegQuality, jpegQuality}
	if img.SharedMat != nil {
		img.SharedMat.Guard.RLock()
		if sharedmat.Filled(&img.SharedMat.Mat) {
			gocv.IMWriteWithParams(savePath, img.SharedMat.Mat, jpgParams)
		}
		img.SharedMat.Guard.RUnlock()
	}
	return
}

// SavePreview will save a smaller Image
func SavePreview(img Image, t time.Time, saveDirectory string, name string, title string, percentage string) (savePath string) {
	savePath = GetImageFilename(t, saveDirectory, name, title, percentage)
	scaledImage := img.ScaleToWidth(128)
	if scaledImage.SharedMat != nil {
		scaledImage.SharedMat.Guard.RLock()
		if sharedmat.Filled(&scaledImage.SharedMat.Mat) {
			gocv.IMWrite(savePath, scaledImage.SharedMat.Mat)
		}
		scaledImage.SharedMat.Guard.RUnlock()
	}
	scaledImage.Cleanup()
	os.Rename(savePath, savePath+".preview")
	return
}

// VideoWriter constants
const (
	ActivityImage  = 0
	ActivityMotion = 1
	ActivityObject = 2
	ActivityFace   = 3
)

// VideoWriter writes Images
type VideoWriter struct {
	record           bool
	recording        bool
	startTime        time.Time
	name             string
	saveDirectory    string
	codec            string
	fileType         string
	maxSec           int
	outFps           int
	bufferSeconds    int
	pubsub           pubsubmutex.PubSub
	preRingBuffer    ringbuffer.RingBuffer[*statsImage]
	writerFull       *gocv.VideoWriter
	writerPortable   *gocv.VideoWriter
	activitySec      int
	lastActivityTime time.Time
	secTick          *time.Ticker
	recordChan       chan bool
	done             chan bool
	cancel           chan bool
	cancelOnce       sync.Once
	videoStats       *VideoStats
	savePreview      bool
	savePortable     bool
	saveFull         bool
	activityType     int
	PortableWidth    int
}

// NewVideoWriter creates a new VideoWriter
func NewVideoWriter(name string, saveDirectory string, codec string, fileType string, bufferSeconds int,
	maxPreSec int, timeoutSec int, maxSec int, outFps int, savePreview bool, savePortable bool, saveFull bool, activityType int) *VideoWriter {
	if saveDirectory == "" || codec == "" || fileType == "" || timeoutSec <= 0 || maxSec <= 0 || outFps <= 0 {
		return nil
	}

	preRingBufferSize := maxPreSec * outFps
	if preRingBufferSize < 0 {
		preRingBufferSize = 0
	}
	preRingBufferSize++ // add one to buffer image that triggers write start

	v := &VideoWriter{
		record:           false,
		recording:        false,
		startTime:        time.Time{},
		name:             name,
		saveDirectory:    saveDirectory,
		codec:            codec, // xvid, mp4v, hev1, hvc1, avc1, avc3
		fileType:         fileType,
		maxSec:           maxSec,
		outFps:           outFps,
		bufferSeconds:    bufferSeconds,
		pubsub:           *pubsubmutex.NewPubSub(),
		preRingBuffer:    *ringbuffer.New[*statsImage](preRingBufferSize),
		writerFull:       nil,
		writerPortable:   nil,
		activitySec:      timeoutSec,
		lastActivityTime: time.Time{},
		secTick:          time.NewTicker(time.Second),
		recordChan:       make(chan bool),
		done:             make(chan bool),
		cancel:           make(chan bool),
		videoStats:       NewVideoStats(),
		savePreview:      savePreview,
		savePortable:     savePortable,
		saveFull:         saveFull,
		activityType:     activityType,
		PortableWidth:    1080,
	}
	return v
}

func (v *VideoWriter) getTickMs(fps int) time.Duration {
	tickMs := 5
	if fps > 0 {
		tickMs = 1000 / fps
	}
	return time.Duration(tickMs)
}

func (v *VideoWriter) checkLastActivityTime(img *ProcessedImage) {
	if v.record &&
		((v.activityType == ActivityImage && img.Original.IsFilled()) ||
			(v.activityType == ActivityMotion && img.HasMotion()) ||
			(v.activityType == ActivityObject && img.HasObject()) ||
			(v.activityType == ActivityFace && img.HasFace())) {
		v.lastActivityTime = time.Now()
	}
}

// Start runs the processes
func (v *VideoWriter) Start() {
	go func() {
		v.videoStats.Start()

		writerTick := time.NewTicker(v.getTickMs(v.outFps) * time.Millisecond)

		bufferSize := v.bufferSeconds * v.outFps
		if bufferSize < 0 {
			bufferSize = 0
		}
		imageSub := v.pubsub.Subscribe(topicWriterImages, v.pubsub.GetUniqueSubscriberID(), bufferSize)
		var bufImg *Image

	Loop:
		for {
			select {
			case <-v.recordChan:
				v.record = true
				v.lastActivityTime = time.Now()
			case msg, ok := <-imageSub.Ch:
				processedImage := msg.Data.(*ProcessedImage)
				if !ok {
					if processedImage != nil {
						processedImage.Cleanup()
					}
					break Loop
				}
				if processedImage == nil {
					continue
				}
				v.checkLastActivityTime(processedImage)

				// buffer for writing
				origImg := processedImage.Original.Ref()
				processedImage.Cleanup()

				// current buffer image
				if bufImg != nil {
					bufImg.Cleanup()
				}
				bufImg = origImg
			case <-writerTick.C:
				if bufImg != nil && bufImg.IsFilled() {
					if v.recording {
						// write
						v.writeRecord(*bufImg)
					} else {
						// pre buffer
						v.preRingBuffer.Add(&statsImage{Image: *bufImg.Ref(), VideoStats: v.videoStats})
					}
				}
				if v.record && !v.recording {
					// open
					firstItem, ok := v.preRingBuffer.TryGet()
					if ok && firstItem.Image.IsFilled() {
						firstImg := firstItem.Image
						preFrames := v.preRingBuffer.GetAll()
						preview := firstImg
						if len(preFrames) > 0 {
							// set preview to most recent image when triggered
							preview = preFrames[len(preFrames)-1].Image
						}
						v.openRecord(firstImg, preview)
						v.writeRecord(firstImg)
						firstImg.Cleanup()
						for _, item := range preFrames {
							curImg := item.Image
							v.writeRecord(curImg)
							curImg.Cleanup()
						}
					}
				} else if !v.record && v.recording {
					// close
					v.closeRecord()
				}
			case <-v.secTick.C:
				if v.isRecordExpired() {
					v.closeRecord()
				}
				if v.isActivityExpired() {
					v.record = false
					v.lastActivityTime = time.Time{}
					v.closeRecord()
				}
			case <-v.cancel:
				break Loop
			}
		}

		// cleanup recording
		if v.recording {
			v.closeRecord()
		}
		if bufImg != nil {
			bufImg.Cleanup()
		}

		writerTick.Stop()
		imageSub.Unsubscribe()
		v.secTick.Stop()
		v.preRingBuffer.Stop()
		v.videoStats.Close()
		close(v.done)
		v.pubsub.Close()
	}()
}

// Trigger to start recording
func (v *VideoWriter) Trigger() {
	v.recordChan <- true
}

// Send Image to write
func (v *VideoWriter) Send(img ProcessedImage) {
	v.pubsub.Publish(pubsubmutex.Message{Topic: topicWriterImages, Data: &img})
}

// Close notified by caller that input stream is done/closed
func (v *VideoWriter) Close() {
	v.cancelOnce.Do(func() {
		close(v.cancel)
	})
}

// Wait until done
func (v *VideoWriter) Wait() {
	<-v.done
}

func (v *VideoWriter) openRecord(img Image, preview Image) {
	timeNow := time.Now()
	if v.saveFull {
		saveFilenameFull := GetVideoFilename(timeNow, v.saveDirectory, v.name, v.fileType, false)
		wFull, err := gocv.VideoWriterFile(saveFilenameFull,
			v.codec, float64(v.outFps),
			img.Width(), img.Height(), true)
		if err == nil {
			v.recording = true
			v.writerFull = wFull

		} else {
			log.Error("Could not open gocv writer full")
		}
	}
	if v.savePortable {
		scaledImage := img.ScaleToWidth(v.PortableWidth)
		saveFilenamePortable := GetVideoFilename(timeNow, v.saveDirectory, v.name, v.fileType, true)
		wPortable, err := gocv.VideoWriterFile(saveFilenamePortable,
			v.codec, float64(v.outFps),
			scaledImage.Width(), scaledImage.Height(), true)
		if err == nil {
			v.recording = true
			v.writerPortable = wPortable
		} else {
			log.Error("Could not open gocv writer portable")
		}
		scaledImage.Cleanup()
	}
	if v.recording {
		v.startTime = timeNow
		if v.savePreview {
			SavePreview(preview, timeNow, v.saveDirectory, v.name, "", "")
		}
	}
}

func (v *VideoWriter) closeRecord() {
	if v.writerFull != nil {
		v.writerFull.Close()
	}
	if v.writerPortable != nil {
		v.writerPortable.Close()
	}
	v.startTime = time.Time{}
	v.recording = false
}

func (v *VideoWriter) isRecordExpired() bool {
	return !v.startTime.IsZero() && time.Since(v.startTime) > (time.Duration(v.maxSec)*time.Second)
}

func (v *VideoWriter) isActivityExpired() bool {
	return !v.lastActivityTime.IsZero() && time.Since(v.lastActivityTime) > (time.Duration(v.activitySec)*time.Second)
}

func (v *VideoWriter) writeRecord(img Image) {
	v.videoStats.AddAccepted()
	if v.writerFull != nil {
		if img.SharedMat != nil {
			img.SharedMat.Guard.RLock()
			if sharedmat.Filled(&img.SharedMat.Mat) {
				v.writerFull.Write(img.SharedMat.Mat)
			}
			img.SharedMat.Guard.RUnlock()
		}
	}
	if v.writerPortable != nil {
		scaledImage := img.ScaleToWidth(v.PortableWidth)
		if scaledImage.SharedMat != nil {
			scaledImage.SharedMat.Guard.RLock()
			if sharedmat.Filled(&scaledImage.SharedMat.Mat) {
				v.writerPortable.Write(scaledImage.SharedMat.Mat)
			}
			scaledImage.SharedMat.Guard.RUnlock()
		}
		scaledImage.Cleanup()
	}
}

// GetBaseFilename will return a formatted base filename with current date time
func GetBaseFilename(t time.Time, saveDirectory string, name string, title string, percentage string) string {
	filename := path.Join(saveDirectory, name)
	filename += "_" + fmt.Sprintf("%d_%02d_%02d_%02d_%02d_%02d_%09d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
	if len(title) > 0 {
		filename += "_" + title
	}
	if len(percentage) > 0 {
		filename += "_" + percentage
	}
	return filename
}

// GetVideoFilename will return a video filename
func GetVideoFilename(t time.Time, saveDirectory string, name string, fileType string, portable bool) (filename string) {
	if portable {
		filename = GetBaseFilename(t, saveDirectory, name, "Portable", "")
	} else {
		filename = GetBaseFilename(t, saveDirectory, name, "Full", "")
	}
	filename += "." + strings.ToLower(fileType)
	return filename
}

// GetImageFilename will return an image filename
func GetImageFilename(t time.Time, saveDirectory string, name string, title string, percentage string) string {
	filename := GetBaseFilename(t, saveDirectory, name, title, percentage)
	filename += ".jpg"
	return filename
}

// GetFrameStats returns the FrameStats directly
func (v *VideoWriter) GetFrameStats() (result *FrameStats) {
	result = v.videoStats.GetFrameStats()
	return
}

// GetStats returns the FrameStats using pubsub
func (v *VideoWriter) GetStats(timeoutMs int) (result *FrameStats) {
	result = v.videoStats.GetStats(timeoutMs)
	return
}

// GetStatsSub returns the subscriber
func (v *VideoWriter) GetStatsSub() (result *pubsubmutex.Subscriber) {
	result = v.videoStats.GetStatsSub()
	return
}
