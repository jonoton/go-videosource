package videosource

import (
	"testing"

	"gocv.io/x/gocv"
)

func BenchmarkScaleToWidth(b *testing.B) {
	const srcW, srcH = 1280, 720
	mat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8U, make([]byte, srcW*srcH))
	img := NewImage(mat)
	defer img.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scaled := img.ScaleToWidth(640)
		scaled.Cleanup()
	}
}

func BenchmarkScaleToFit(b *testing.B) {
	const srcW, srcH = 1280, 720
	mat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8U, make([]byte, srcW*srcH))
	img := NewImage(mat)
	defer img.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scaled := img.ScaleToFit(640)
		scaled.Cleanup()
	}
}

func BenchmarkChangeQuality(b *testing.B) {
	const srcW, srcH = 640, 480
	// 3-channel BGR image
	mat, _ := gocv.NewMatFromBytes(srcH, srcW, gocv.MatTypeCV8UC3, make([]byte, srcW*srcH*3))
	img := NewImage(mat)
	defer img.Cleanup()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		q := img.ChangeQuality(80)
		q.Cleanup()
	}
}

func BenchmarkVideoStats(b *testing.B) {
	vs := NewVideoStats()
	vs.Start()
	defer vs.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		vs.AddAccepted()
		_ = vs.GetFrameStats()
	}
}
