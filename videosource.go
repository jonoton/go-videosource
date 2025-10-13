package videosource

import (
	"sync"

	"gocv.io/x/gocv"
)

// ConnectionStatus defines the state of the connection
type ConnectionStatus int

const (
	Disconnected ConnectionStatus = iota
	Connecting
	Connected
)

// SourceType defines the type of the source
type SourceType int

const (
	UnknownType SourceType = iota
	FileSourceType
	IPCamSourceType
)

// VideoSource interface for setting up and reading images
type VideoSource interface {
	GetName() string
	Initialize() (ok bool)
	Cleanup()
	ReadImage() (done bool, image Image)
	GetConnectionStatus() (status ConnectionStatus)
	GetSourceType() (sourceType SourceType)
}

// BaseVideo contains common video source info
type BaseVideo struct {
	name             string
	gocvVideoCapture *gocv.VideoCapture
	status           ConnectionStatus
	sourceType       SourceType
	mu               sync.Mutex
}

// NewBaseVideo creates a new BaseVideo
func NewBaseVideo(name string, sourceType SourceType) *BaseVideo {
	b := &BaseVideo{
		name:             name,
		gocvVideoCapture: nil,
		status:           Disconnected,
		sourceType:       sourceType,
	}
	return b
}

// GetName implements interface
func (b *BaseVideo) GetName() string {
	return b.name
}

// Initialize implements interface
func (b *BaseVideo) Initialize() (ok bool) {
	// implement in source type
	return
}

// Cleanup implements interface
func (b *BaseVideo) Cleanup() {
	// implement in source type
}

// ReadImage implements interface
func (b *BaseVideo) ReadImage() (done bool, image Image) {
	// implement in source type
	return
}

// GetConnectionStatus implements interface
func (b *BaseVideo) GetConnectionStatus() (status ConnectionStatus) {
	b.mu.Lock()
	status = b.status
	b.mu.Unlock()
	return
}

// GetSourceType implements interface
func (b *BaseVideo) GetSourceType() (sourceType SourceType) {
	b.mu.Lock()
	sourceType = b.sourceType
	b.mu.Unlock()
	return
}
