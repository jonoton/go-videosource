package videosource

import (
	log "github.com/sirupsen/logrus"

	"gocv.io/x/gocv"
)

// FileSource is a file source
type FileSource struct {
	BaseVideo
	filename string
}

// NewFileSource creates a new FileSource
func NewFileSource(name string, filename string) *FileSource {
	f := &FileSource{
		BaseVideo: *NewBaseVideo(name, FileSourceType),
		filename:  filename,
	}
	return f
}

// Initialize implements interface
func (f *FileSource) Initialize() (ok bool) {
	gocvVideoCapture, err := gocv.VideoCaptureFile(f.filename)
	if err != nil {
		log.Warnf("Could not open video capture file: %s", f.filename)
		return
	}
	f.mu.Lock()
	f.gocvVideoCapture = gocvVideoCapture
	f.status = Connected
	f.mu.Unlock()
	ok = true
	return
}

// Cleanup implements interface
func (f *FileSource) Cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gocvVideoCapture != nil {
		f.gocvVideoCapture.Close()
		f.gocvVideoCapture = nil
	}
	f.status = Disconnected
}

// ReadImage implements interface
func (f *FileSource) ReadImage() (done bool, image Image) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != Connected || f.gocvVideoCapture == nil {
		done = true
		return
	}
	mat := gocv.NewMat()
	if !f.gocvVideoCapture.Read(&mat) || mat.Empty() {
		mat.Close()
		f.status = Disconnected
		done = true
		return
	}
	image = *NewImage(mat)
	return
}
