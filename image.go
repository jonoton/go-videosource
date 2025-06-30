package videosource

import (
	"image"
	"math"
	"time"

	"github.com/jonoton/go-sharedmat"
	"gocv.io/x/gocv"
)

// Image contains an image
type Image struct {
	SharedMat   *sharedmat.SharedMat
	createdTime time.Time
}

// NewImage creates a new Image
func NewImage(mat gocv.Mat) *Image {
	i := &Image{
		SharedMat:   sharedmat.NewSharedMat(mat),
		createdTime: time.Now(),
	}
	return i
}

func (i *Image) CreatedTime() time.Time {
	return i.createdTime
}

// IsFilled checks the underlying image not empty
func (i *Image) IsFilled() bool {
	if i.SharedMat == nil {
		return false
	}
	i.SharedMat.Guard.RLock()
	defer i.SharedMat.Guard.RUnlock()
	return sharedmat.Filled(&i.SharedMat.Mat)
}

// Height returns the Image height or -1
func (i *Image) Height() int {
	if i.SharedMat == nil {
		return -1
	}
	i.SharedMat.Guard.RLock()
	defer i.SharedMat.Guard.RUnlock()
	result := -1
	if sharedmat.Filled(&i.SharedMat.Mat) {
		result = i.SharedMat.Mat.Rows()
	}
	return result
}

// Width returns the Image width or -1
func (i *Image) Width() int {
	if i.SharedMat == nil {
		return -1
	}
	i.SharedMat.Guard.RLock()
	defer i.SharedMat.Guard.RUnlock()
	result := -1
	if sharedmat.Filled(&i.SharedMat.Mat) {
		result = i.SharedMat.Mat.Cols()
	}
	return result
}

// Ref will create a copy and reference the underlying SharedMat
func (i *Image) Ref() *Image {
	copy := &Image{
		createdTime: i.createdTime,
	}
	if i.SharedMat != nil {
		copy.SharedMat = i.SharedMat.Ref()
	}
	return copy
}

// Clone will clone the Image
func (i *Image) Clone() *Image {
	clone := &Image{
		createdTime: i.createdTime,
	}
	if i.SharedMat != nil {
		clone.SharedMat = i.SharedMat.Clone()
	}
	return clone
}

// Cleanup will cleanup the Image
func (i *Image) Cleanup() {
	if i.SharedMat != nil {
		_, closed := i.SharedMat.Cleanup()
		if closed {
			i.SharedMat = nil
		}
	}
}

// GetRegion will return a new Image per rectangle parameter
func (i *Image) GetRegion(rect image.Rectangle) (region Image) {
	if i.SharedMat == nil {
		return
	}
	i.SharedMat.Guard.RLock()
	defer i.SharedMat.Guard.RUnlock()
	corrRect := CorrectRectangle(*i, rect)
	if !corrRect.Empty() && sharedmat.Filled(&i.SharedMat.Mat) {
		matRegion := i.SharedMat.Mat.Region(corrRect)
		region = *NewImage(matRegion.Clone())
		matRegion.Close()
	}
	return
}

// ChangeQuality will return a copy of the Image with the quality set to percent
func (i *Image) ChangeQuality(percent int) Image {
	if percent <= 0 || percent >= 100 {
		return *i.Ref()
	}
	if i.SharedMat == nil {
		return *i.Ref()
	}
	var updatedImage Image
	i.SharedMat.Guard.RLock()
	if sharedmat.Filled(&i.SharedMat.Mat) {
		jpgParams := []int{gocv.IMWriteJpegQuality, percent}
		encoded, err := gocv.IMEncodeWithParams(gocv.JPEGFileExt, i.SharedMat.Mat, jpgParams)
		if err == nil {
			newMat, err := gocv.IMDecode(encoded.GetBytes(), gocv.IMReadUnchanged)
			if err == nil {
				updatedImage = *NewImage(newMat.Clone())
				updatedImage.createdTime = i.createdTime
			}
			newMat.Close()
		}
		encoded.Close()
	}
	i.SharedMat.Guard.RUnlock()
	return updatedImage
}

// EncodedQuality returns a JPEG byte array with the given quality percentage
func (i *Image) EncodedQuality(percent int) []byte {
	imgArray := make([]byte, 0)
	if i.SharedMat == nil {
		return imgArray
	}
	i.SharedMat.Guard.RLock()
	if sharedmat.Filled(&i.SharedMat.Mat) {
		jpgParams := []int{gocv.IMWriteJpegQuality, percent}
		encoded, err := gocv.IMEncodeWithParams(gocv.JPEGFileExt, i.SharedMat.Mat, jpgParams)
		if err == nil {
			imgArray = append([]byte{}, encoded.GetBytes()...)
		}
		encoded.Close()
	}
	i.SharedMat.Guard.RUnlock()
	return imgArray
}

// ScaleToWidth will return a copy of the Image to scale given the width
func (i *Image) ScaleToWidth(width int) Image {
	if width <= 0 || width == i.Width() {
		return *i.Ref()
	}
	if i.SharedMat == nil {
		return *i.Ref()
	}
	var scaled Image
	// scale down
	var interpolationFlags = gocv.InterpolationArea
	// scale up
	if width > i.Width() {
		interpolationFlags = gocv.InterpolationCubic
	}
	scaleWidth := float64(width) / float64(i.Width())
	scaleHeight := float64(width) / float64(i.Height())
	scaleEvenly := math.Min(scaleWidth, scaleHeight)
	dstMat := gocv.NewMat()
	i.SharedMat.Guard.RLock()
	if sharedmat.Filled(&i.SharedMat.Mat) {
		gocv.Resize(i.SharedMat.Mat, &dstMat, image.Point{}, scaleEvenly, scaleEvenly, interpolationFlags)
		scaled = *NewImage(dstMat.Clone())
		scaled.createdTime = i.createdTime
	} else {
		scaled = *NewImage(dstMat.Clone())
	}
	i.SharedMat.Guard.RUnlock()
	dstMat.Close()
	return scaled
}

// ImageByCreatedTime sorting ascending order
type ImageByCreatedTime []Image

func (b ImageByCreatedTime) Len() int           { return len(b) }
func (b ImageByCreatedTime) Less(i, j int) bool { return b[i].createdTime.Before(b[j].createdTime) }
func (b ImageByCreatedTime) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

type StatsImage struct {
	Image      Image
	VideoStats *VideoStats
}

func (s *StatsImage) CreatedTime() time.Time {
	return s.Image.CreatedTime()
}

func (s *StatsImage) Ref() *StatsImage {
	copy := &StatsImage{
		Image:      *s.Image.Ref(),
		VideoStats: s.VideoStats,
	}
	return copy
}

func (s *StatsImage) Cleanup() {
	s.Image.Cleanup()
	if s.Image.SharedMat == nil && s.VideoStats != nil {
		s.VideoStats.AddDropped()
	}
}
