package videosource

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/jonoton/go-framebuffer"
	"github.com/jonoton/go-pubsubmutex"
	"github.com/jonoton/go-ringbuffer"
	"github.com/jonoton/go-sharedmat"
	"github.com/jonoton/go-temporalbuffer"
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
	stateMu          sync.RWMutex
	record           bool
	recording        bool
	startTime        time.Time
	name             string
	saveDirectory    string
	codec            string
	fileType         string
	maxSec           int
	outFps           int
	maxPreSec        int
	bufferSeconds    int
	pubsub           pubsubmutex.PubSub
	preWriteBuffer   ringbuffer.RingBuffer[*Image]
	writeBuffer      temporalbuffer.Buffer[*Image]
	previewImg       *Image
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

	preWriteBufferSize := maxPreSec * outFps
	if preWriteBufferSize < 0 {
		preWriteBufferSize = 0
	}
	writeBufferSize := bufferSeconds * outFps
	if writeBufferSize < 0 {
		writeBufferSize = 0
	}
	writeBufferSize += preWriteBufferSize

	v := &VideoWriter{
		record:         false,
		recording:      false,
		startTime:      time.Time{},
		name:           name,
		saveDirectory:  saveDirectory,
		codec:          codec, // xvid, mp4v, hev1, hvc1, avc1, avc3
		fileType:       fileType,
		maxSec:         maxSec,
		outFps:         outFps,
		maxPreSec:      maxPreSec,
		bufferSeconds:  bufferSeconds,
		pubsub:         *pubsubmutex.NewPubSub(),
		preWriteBuffer: *ringbuffer.New[*Image](preWriteBufferSize),
		writeBuffer: *temporalbuffer.New[*Image](writeBufferSize,
			temporalbuffer.WithReadContinuity(false),
			temporalbuffer.WithDropStrategy(temporalbuffer.DropClosest),
			temporalbuffer.WithFillStrategy(temporalbuffer.NoFill)),
		previewImg:       nil,
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
	pubsubmutex.RegisterTopic[*ProcessedImage](&v.pubsub, topicWriterImages)

	return v
}

func (v *VideoWriter) checkLastActivityTime(img *ProcessedImage) {
	if (v.activityType == ActivityImage && img.Original.IsFilled()) ||
		(v.activityType == ActivityMotion && img.HasMotion()) ||
		(v.activityType == ActivityObject && img.HasObject()) ||
		(v.activityType == ActivityFace && img.HasFace()) {
		v.stateMu.Lock()
		if v.record {
			v.lastActivityTime = time.Now()
		}
		v.stateMu.Unlock()
	}
}

// Start runs the processes
func (v *VideoWriter) Start() {
	go func() {
		v.videoStats.Start()
		wg := sync.WaitGroup{}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		frameBuffer := framebuffer.NewBuffer(ctx, uint(v.outFps),
			framebuffer.WithStaleFrameTolerance[*Image](time.Second))

		// input to framebuffer
		wg.Add(1)
		go func() {
			defer wg.Done()
			bufferSize := v.bufferSeconds * v.outFps
			if bufferSize < 0 {
				bufferSize = 0
			}
			processedImageSub, _ := pubsubmutex.Subscribe[*ProcessedImage](&v.pubsub,
				topicWriterImages, v.pubsub.GetUniqueSubscriberID(), bufferSize)
			addedFrames := 0
		InputLoop:
			for {
				select {
				case msg, ok := <-processedImageSub.Ch:
					processedImage := msg.Data
					if !ok {
						if processedImage != nil {
							processedImage.Cleanup()
						}
						break InputLoop
					}
					if processedImage == nil {
						v.videoStats.AddDropped()
						continue
					}
					if !processedImage.Original.IsFilled() {
						processedImage.Cleanup()
						v.videoStats.AddDropped()
						continue
					}
					v.checkLastActivityTime(processedImage)

					// buffer for writing
					img := processedImage.Original
					frameBuffer.AddFrame(img.Ref())
					addedFrames++
					processedImage.Cleanup()
				case <-v.cancel:
					break InputLoop
				}
			}
			processedImageSub.Unsubscribe()
			if addedFrames > 0 {
				frameBuffer.Drain()
			}
			frameBuffer.Close()
		}()

		// framebuffer to write buffers
		wg.Add(1)
		go func() {
			defer wg.Done()
			frameBufferChan := frameBuffer.Frames()
			for img := range frameBufferChan {
				if img == nil {
					continue
				}
				v.addImgBuffer(img.Ref())
				img.Cleanup()
			}
			v.preWriteBuffer.Stop()
			v.writeBuffer.Close()
		}()

		// write to file
		wg.Add(1)
		go func() {
			defer wg.Done()

			writeBufferChan := v.writeBuffer.GetOldestChan()
		WriteFileLoop:
			for {
				select {
				case <-v.recordChan:
					v.stateMu.Lock()
					v.record = true
					v.lastActivityTime = time.Now()
					v.stateMu.Unlock()
					if v.isStartRecord() {
						v.transferPreBuffer()
					}
				case img, ok := <-writeBufferChan:
					if !ok {
						if img != nil {
							img.Cleanup()
						}
						break WriteFileLoop
					}
					if img == nil {
						continue
					}

					if v.isStartRecord() {
						v.transferPreBuffer()
						previewImg := img
						if v.previewImg != nil {
							previewImg = v.previewImg
						}
						v.openRecord(*img, *previewImg)
						v.cleanupPreviewImg()
						v.writeRecord(*img)
					} else if v.isRecording() {
						v.writeRecord(*img)
					}
					img.Cleanup()

					if v.isCloseRecord() {
						v.closeRecord()
						v.cleanupPreviewImg()
						v.clearWriteBuffer()
					}
				case <-v.secTick.C:
					if v.isRecordExpired() {
						v.closeRecord()
						v.cleanupPreviewImg()
						v.clearWriteBuffer()
					}
					if v.isActivityExpired() {
						v.stateMu.Lock()
						v.record = false
						v.lastActivityTime = time.Time{}
						v.stateMu.Unlock()
						v.closeRecord()
						v.cleanupPreviewImg()
						v.clearWriteBuffer()
					}
				}
			}
			v.closeRecord()
			v.cleanupPreviewImg()
		}()

		// cleanup
		wg.Wait()
		v.secTick.Stop()
		v.videoStats.Close()
		v.pubsub.Close()
		close(v.done)
	}()
}

// Trigger to start recording
func (v *VideoWriter) Trigger() {
	if v.IsDone() {
		return
	}
	select {
	case <-v.cancel:
	case v.recordChan <- true:
	}
}

// Send Image to write
func (v *VideoWriter) Send(img ProcessedImage) {
	if v.IsDone() {
		img.Cleanup()
		return
	}
	pubsubmutex.Publish(&v.pubsub,
		pubsubmutex.Message[*ProcessedImage]{Topic: topicWriterImages, Data: &img})
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

func (v *VideoWriter) isStartRecord() bool {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return v.record && !v.recording
}

func (v *VideoWriter) isCloseRecord() bool {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return !v.record && v.recording
}

func (v *VideoWriter) isRecording() bool {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return v.recording
}

func (v *VideoWriter) hasPreBuffer() bool {
	return v.maxPreSec > 0
}

func (v *VideoWriter) addImgBuffer(img *Image) {
	if img == nil {
		return
	}
	v.stateMu.RLock()
	recording := v.recording
	startRecord := v.record && !v.recording
	v.stateMu.RUnlock()

	if !recording && v.hasPreBuffer() {
		v.preWriteBuffer.Add(img)
	} else if startRecord || recording {
		v.writeBuffer.Add(img)
	} else {
		img.Cleanup()
	}
}

func (v *VideoWriter) transferPreBuffer() {
	if !v.hasPreBuffer() {
		return
	}
	imgs := v.preWriteBuffer.GetAll()
	if len(imgs) > 0 {
		v.cleanupPreviewImg()
		v.previewImg = imgs[len(imgs)-1].Ref()
	}
	v.writeBuffer.AddAll(imgs)
}

func (v *VideoWriter) cleanupPreviewImg() {
	if v.previewImg != nil {
		v.previewImg.Cleanup()
		v.previewImg = nil
	}
}

func (v *VideoWriter) clearWriteBuffer() {
	curImgs := v.writeBuffer.GetAll()
	for _, img := range curImgs {
		if img != nil {
			img.Cleanup()
		}
	}
}

func (v *VideoWriter) openRecord(img Image, preview Image) {
	timeNow := time.Now()
	if v.saveFull {
		saveFilenameFull := GetVideoFilename(timeNow, v.saveDirectory, v.name, v.fileType, false)
		wFull, err := gocv.VideoWriterFile(saveFilenameFull,
			v.codec, float64(v.outFps),
			img.Width(), img.Height(), true)
		if err == nil {
			v.stateMu.Lock()
			v.recording = true
			v.stateMu.Unlock()
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
			v.stateMu.Lock()
			v.recording = true
			v.stateMu.Unlock()
			v.writerPortable = wPortable
		} else {
			log.Error("Could not open gocv writer portable")
		}
		scaledImage.Cleanup()
	}
	if v.isRecording() {
		v.stateMu.Lock()
		v.startTime = timeNow
		v.stateMu.Unlock()
		if v.savePreview {
			SavePreview(preview, timeNow, v.saveDirectory, v.name, "", "")
		}
	}
}

func (v *VideoWriter) closeRecord() {
	if v.writerFull != nil {
		v.writerFull.Close()
		v.writerFull = nil
	}
	if v.writerPortable != nil {
		v.writerPortable.Close()
		v.writerPortable = nil
	}
	v.stateMu.Lock()
	v.startTime = time.Time{}
	v.recording = false
	v.stateMu.Unlock()
}

func (v *VideoWriter) isRecordExpired() bool {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return !v.startTime.IsZero() && time.Since(v.startTime) > (time.Duration(v.maxSec)*time.Second)
}

func (v *VideoWriter) isActivityExpired() bool {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
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
	if v.IsDone() {
		return
	}
	result = v.videoStats.GetStats(timeoutMs)
	return
}

// GetStatsSub returns the subscriber
func (v *VideoWriter) GetStatsSub() (result *pubsubmutex.Subscriber[*FrameStats]) {
	if v.IsDone() {
		return
	}
	result = v.videoStats.GetStatsSub()
	return
}

func (v *VideoWriter) IsDone() bool {
	isDone := false
	select {
	case _, ok := <-v.cancel:
		if !ok {
			isDone = true
		}
	default:
		// not done and not ready
	}

	return isDone
}
