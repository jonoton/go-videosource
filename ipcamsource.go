package videosource

import (
	"fmt"
	"os"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"gocv.io/x/gocv"
)

var ffmpegCaptureMu sync.Mutex

// IPCamSource is a ipcam source
type IPCamSource struct {
	BaseVideo
	url                 string
	captureTimeoutMs    int
	captureOptions      string
	consecutiveFailures int
	maxFailureTolerance int
}

// NewIPCamSource creates a new IPCamSource
func NewIPCamSource(name string, url string) *IPCamSource {
	i := &IPCamSource{
		BaseVideo:           *NewBaseVideo(name, IPCamSourceType),
		url:                 url,
		captureTimeoutMs:    0,
		captureOptions:      "",
		consecutiveFailures: 0,
		maxFailureTolerance: 10,
	}
	return i
}

func (i *IPCamSource) SetCaptureTimeoutMs(captureTimeoutMs int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.captureTimeoutMs = captureTimeoutMs
}

func (i *IPCamSource) SetCaptureOptions(options string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.captureOptions = options
}

func (i *IPCamSource) SetMaxFailureTolerance(tolerance int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.maxFailureTolerance = tolerance
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
	timeoutMs := i.captureTimeoutMs
	captureOpts := i.captureOptions
	url := i.url
	i.mu.Unlock()

	ffmpegCaptureMu.Lock()
	origEnv, hadEnv := os.LookupEnv("OPENCV_FFMPEG_CAPTURE_OPTIONS")

	var opts []string
	if timeoutMs > 0 {
		// Set ffmpeg timeout in microseconds
		opts = append(opts, fmt.Sprintf("stimeout;%d", timeoutMs*1000))
	}
	if captureOpts != "" {
		opts = append(opts, captureOpts)
	} else if hadEnv && origEnv != "" {
		opts = append(opts, origEnv)
	}
	if len(opts) > 0 {
		os.Setenv("OPENCV_FFMPEG_CAPTURE_OPTIONS", strings.Join(opts, "|"))
	}
	gocvVideoCapture, err := gocv.VideoCaptureFile(url)

	// Restore original process environment immediately to prevent cross-camera bleed
	if hadEnv {
		os.Setenv("OPENCV_FFMPEG_CAPTURE_OPTIONS", origEnv)
	} else {
		os.Unsetenv("OPENCV_FFMPEG_CAPTURE_OPTIONS")
	}
	ffmpegCaptureMu.Unlock()

	i.mu.Lock()
	defer i.mu.Unlock()
	if err != nil {
		i.status = Disconnected
		log.Warnf("Could not open video capture url: %s", url)
		return
	}
	i.gocvVideoCapture = gocvVideoCapture
	i.consecutiveFailures = 0
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
	if ok := i.gocvVideoCapture.Read(&mat); !ok || mat.Empty() {
		mat.Close()
		i.consecutiveFailures++
		maxTolerance := i.maxFailureTolerance
		if maxTolerance <= 0 {
			maxTolerance = 10
		}
		if i.consecutiveFailures >= maxTolerance {
			i.status = Disconnected
			done = true
		}
		return
	}
	i.consecutiveFailures = 0
	image = *NewImage(mat)
	return
}
