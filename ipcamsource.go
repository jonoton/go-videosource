package videosource

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"gocv.io/x/gocv"
)

// IPCamSource is a ipcam source
type IPCamSource struct {
	BaseVideo
	url              string
	captureTimeoutMs int
}

// NewIPCamSource creates a new IPCamSource
func NewIPCamSource(name string, url string) *IPCamSource {
	i := &IPCamSource{
		BaseVideo:        *NewBaseVideo(name, IPCamSourceType),
		url:              url,
		captureTimeoutMs: 0,
	}
	return i
}

func (i *IPCamSource) SetCaptureTimeoutMs(captureTimeoutMs int) {
	i.captureTimeoutMs = captureTimeoutMs
}

// Initialize implements interface
func (i *IPCamSource) Initialize() (ok bool) {
	i.mu.Lock()
	if i.status == Connected {
		i.mu.Unlock()
		log.Infof("Already connected. Skipping.")
		return
	}
	if i.status == Connecting {
		i.mu.Unlock()
		log.Infof("Already connecting. Skipping.")
		return
	}
	i.status = Connecting
	i.mu.Unlock()

	if i.captureTimeoutMs > 0 {
		// Set ffmpeg timeout in microseconds
		captureTimeoutStr := fmt.Sprintf("stimeout;%d", i.captureTimeoutMs*1000)
		os.Setenv("OPENCV_FFMPEG_CAPTURE_OPTIONS", captureTimeoutStr)
	}
	gocvVideoCapture, err := gocv.VideoCaptureFile(i.url)

	i.mu.Lock()
	defer i.mu.Unlock()
	if err != nil {
		i.status = Disconnected
		log.Warnf("Could not open video capture url: %s", i.url)
		return
	}
	i.gocvVideoCapture = gocvVideoCapture
	i.status = Connected
	ok = true
	return
}

// Cleanup implements interface
func (i *IPCamSource) Cleanup() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.gocvVideoCapture != nil {
		i.gocvVideoCapture.Close()
		i.gocvVideoCapture = nil
	}
	i.status = Disconnected
}

// ReadImage implements interface
func (i *IPCamSource) ReadImage() (done bool, image Image) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.status != Connected || i.gocvVideoCapture == nil {
		done = true
		return
	}
	mat := gocv.NewMat()
	done = false
	if ok := i.gocvVideoCapture.Read(&mat); !ok {
		i.status = Disconnected
		done = true
	}
	image = *NewImage(mat)
	return
}
