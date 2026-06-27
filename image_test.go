package videosource

import (
	"testing"

	"gocv.io/x/gocv"
)

func TestCleanup(t *testing.T) {
	newMat, _ := gocv.NewMatFromBytes(1, 1, gocv.MatTypeCV16S, make([]byte, 10))
	img := NewImage(newMat)
	secondImage := img.Ref()
	secondImage.Cleanup()
	if !img.IsFilled() {
		t.Fatal("original image should still be filled after ref cleanup")
	}
	if img.SharedMat.NumRefs() != 1 {
		t.Fatalf("sharedMat refs = %d, expected 1", img.SharedMat.NumRefs())
	}
	img.Cleanup()
	if img.SharedMat != nil {
		t.Errorf("sharedMat should be nil after Cleanup, got refs = %d", img.SharedMat.NumRefs())
	}
}

func TestScale(t *testing.T) {
	const srcW, srcH = 100, 200
	newMat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8U, make([]byte, srcW*srcH))
	img := NewImage(newMat)

	// Same width: should return a ref, not a new mat
	same := img.ScaleToWidth(img.Width())
	if same.Width() != srcW {
		t.Errorf("ScaleToWidth(same) width = %d, want %d", same.Width(), srcW)
	}
	same.Cleanup()

	// Zero: should return a ref unchanged
	zero := img.ScaleToWidth(0)
	if zero.Width() != srcW {
		t.Errorf("ScaleToWidth(0) width = %d, want %d", zero.Width(), srcW)
	}
	zero.Cleanup()

	// Scale down to half width — aspect ratio must be preserved
	halfW := srcW / 2
	scaled := img.ScaleToWidth(halfW)
	if scaled.Width() != halfW {
		t.Errorf("ScaleToWidth(%d) width = %d, want %d", halfW, scaled.Width(), halfW)
	}
	if scaled.Height() != srcH/2 {
		t.Errorf("ScaleToWidth(%d) height = %d, want %d (aspect ratio preserved)", halfW, scaled.Height(), srcH/2)
	}
	scaled.Cleanup()

	// Scale up
	doubleW := srcW * 2
	upscaled := img.ScaleToWidth(doubleW)
	if upscaled.Width() != doubleW {
		t.Errorf("ScaleToWidth(%d) width = %d, want %d", doubleW, upscaled.Width(), doubleW)
	}
	if upscaled.Height() != srcH*2 {
		t.Errorf("ScaleToWidth(%d) height = %d, want %d (aspect ratio preserved)", doubleW, upscaled.Height(), srcH*2)
	}
	upscaled.Cleanup()

	// Original should still be intact
	if !img.IsFilled() {
		t.Fatal("original image should still be filled after all ScaleToWidth calls")
	}
	if img.SharedMat.NumRefs() != 1 {
		t.Fatalf("sharedMat refs = %d, expected 1", img.SharedMat.NumRefs())
	}
	img.Cleanup()
	if img.SharedMat != nil {
		t.Errorf("sharedMat should be nil after Cleanup, got refs = %d", img.SharedMat.NumRefs())
	}
}

func TestScaleToFit(t *testing.T) {
	// Test portrait source (100x200) - largest is height (200)
	{
		const srcW, srcH = 100, 200
		newMat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8U, make([]byte, srcW*srcH))
		img := NewImage(newMat)

		// Same max dimension: should return a ref, not a new mat
		same := img.ScaleToFit(srcH)
		if same.Width() != srcW || same.Height() != srcH {
			t.Errorf("ScaleToFit(same) = %dx%d, want %dx%d", same.Width(), same.Height(), srcW, srcH)
		}
		same.Cleanup()

		// Zero: should return a ref unchanged
		zero := img.ScaleToFit(0)
		if zero.Width() != srcW || zero.Height() != srcH {
			t.Errorf("ScaleToFit(0) = %dx%d, want %dx%d", zero.Width(), zero.Height(), srcW, srcH)
		}
		zero.Cleanup()

		// Scale down to maxDimension = 100 (should result in 50x100)
		scaled := img.ScaleToFit(100)
		if scaled.Width() != 50 || scaled.Height() != 100 {
			t.Errorf("ScaleToFit(100) = %dx%d, want 50x100", scaled.Width(), scaled.Height())
		}
		scaled.Cleanup()

		// Scale up to maxDimension = 400 (should result in 200x400)
		upscaled := img.ScaleToFit(400)
		if upscaled.Width() != 200 || upscaled.Height() != 400 {
			t.Errorf("ScaleToFit(400) = %dx%d, want 200x400", upscaled.Width(), upscaled.Height())
		}
		upscaled.Cleanup()

		img.Cleanup()
	}

	// Test landscape source (200x100) - largest is width (200)
	{
		const srcW, srcH = 200, 100
		newMat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8U, make([]byte, srcW*srcH))
		img := NewImage(newMat)

		// Scale down to maxDimension = 100 (should result in 100x50)
		scaled := img.ScaleToFit(100)
		if scaled.Width() != 100 || scaled.Height() != 50 {
			t.Errorf("ScaleToFit(100) on landscape = %dx%d, want 100x50", scaled.Width(), scaled.Height())
		}
		scaled.Cleanup()

		img.Cleanup()
	}
}

