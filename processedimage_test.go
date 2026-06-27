package videosource

import (
	"image"
	"sort"
	"testing"
	"time"
)

func TestProcessedImageContentSort(t *testing.T) {
	obj80 := NewObjectInfo(image.Rectangle{}, *NewColorThickness("blue", 1))
	obj80.Percentage = 80
	obj90 := NewObjectInfo(image.Rectangle{}, *NewColorThickness("blue", 1))
	obj90.Percentage = 90

	face80 := NewFaceInfo(image.Rectangle{}, *NewColorThickness("green", 1))
	face80.Percentage = 80
	face90 := NewFaceInfo(image.Rectangle{}, *NewColorThickness("green", 1))
	face90.Percentage = 90

	now := time.Now()
	// later: 2 objects at 80%, 2 faces at 80%
	later := NewProcessedImage(Image{createdTime: now.Add(time.Hour)})
	later.Objects = append(later.Objects, *obj80, *obj80)
	later.Faces = append(later.Faces, *face80, *face80)

	// earlier: 1 object at 90%, 1 face at 90%
	earlier := NewProcessedImage(Image{createdTime: now})
	earlier.Objects = append(earlier.Objects, *obj90)
	earlier.Faces = append(earlier.Faces, *face90)

	list := []ProcessedImage{*later, *earlier}

	// --- ProcessedImageByCreatedTime (ascending) ---
	sort.Sort(ProcessedImageByCreatedTime(list))
	if !list[0].Original.createdTime.Before(list[1].Original.createdTime) {
		t.Errorf("ProcessedImageByCreatedTime: index 0 (%v) should be before index 1 (%v)",
			list[0].Original.createdTime, list[1].Original.createdTime)
	}

	// --- ProcessedImageByObjLen (descending by object count) ---
	sort.Sort(ProcessedImageByObjLen(list))
	if len(list[0].Objects) < len(list[1].Objects) {
		t.Errorf("ProcessedImageByObjLen: index 0 has %d objects, expected >= index 1 (%d)",
			len(list[0].Objects), len(list[1].Objects))
	}

	// --- ProcessedImageByObjPercent (descending by highest object percent) ---
	sort.Sort(ProcessedImageByObjPercent(list))
	p0 := getHighestObjectPercentage(list[0].Objects)
	p1 := getHighestObjectPercentage(list[1].Objects)
	if p0 < p1 {
		t.Errorf("ProcessedImageByObjPercent: index 0 highest=%d, expected >= index 1 highest=%d", p0, p1)
	}

	// --- ProcessedImageByFaceLen (descending by face count) ---
	sort.Sort(ProcessedImageByFaceLen(list))
	if len(list[0].Faces) < len(list[1].Faces) {
		t.Errorf("ProcessedImageByFaceLen: index 0 has %d faces, expected >= index 1 (%d)",
			len(list[0].Faces), len(list[1].Faces))
	}

	// --- ProcessedImageByFacePercent (descending by highest face percent) ---
	sort.Sort(ProcessedImageByFacePercent(list))
	f0 := getHighestFacePercentage(list[0].Faces)
	f1 := getHighestFacePercentage(list[1].Faces)
	if f0 < f1 {
		t.Errorf("ProcessedImageByFacePercent: index 0 highest=%d, expected >= index 1 highest=%d", f0, f1)
	}
}

func TestProcessedImageCleanup(t *testing.T) {
	obj := NewObjectInfo(image.Rectangle{}, *NewColorThickness("blue", 1))
	obj.Percentage = 75
	face := NewFaceInfo(image.Rectangle{}, *NewColorThickness("green", 1))
	face.Percentage = 80

	p := NewProcessedImage(Image{})
	p.Objects = append(p.Objects, *obj)
	p.Faces = append(p.Faces, *face)

	p.Cleanup()

	if len(p.Objects) != 0 {
		t.Errorf("Cleanup: Objects len = %d, want 0", len(p.Objects))
	}
	if len(p.Faces) != 0 {
		t.Errorf("Cleanup: Faces len = %d, want 0", len(p.Faces))
	}
	if len(p.Motions) != 0 {
		t.Errorf("Cleanup: Motions len = %d, want 0", len(p.Motions))
	}
}

func TestHighlightedNilSafe(t *testing.T) {
	// ProcessedImage with no SharedMat (nil) should not panic on Highlighted* calls
	p := NewProcessedImage(Image{})
	obj := NewObjectInfo(image.Rect(0, 0, 10, 10), *NewColorThickness("blue", 1))
	p.Objects = append(p.Objects, *obj)
	face := NewFaceInfo(image.Rect(0, 0, 10, 10), *NewColorThickness("green", 1))
	p.Faces = append(p.Faces, *face)
	motion := NewMotionInfo(image.Rect(0, 0, 10, 10), *NewColorThickness("red", 1))
	p.Motions = append(p.Motions, *motion)

	// These should not panic
	_ = p.HighlightedMotion()
	_ = p.HighlightedObject()
	_ = p.HighlightedFace()
	_ = p.HighlightedAll()
}
