package videosource

import (
	"image"
	"testing"

	"gocv.io/x/gocv"
)

// newTestImage creates a filled Image of the given dimensions for testing.
// The caller is responsible for calling img.Cleanup().
func newTestImage(width, height int) *Image {
	mat := gocv.NewMatWithSize(height, width, gocv.MatTypeCV8U)
	return NewImage(mat)
}

// newEmptyImage returns an unfilled Image to test the !IsFilled() guard paths.
func newEmptyImage() *Image {
	return &Image{}
}

// ----------------------------------------------------------------------------
// CorrectRectangle
// ----------------------------------------------------------------------------

func TestCorrectRectangle(t *testing.T) {
	img := newTestImage(100, 80)
	defer img.Cleanup()

	tests := []struct {
		name string
		in   image.Rectangle
		want image.Rectangle
	}{
		{
			name: "already within bounds",
			in:   image.Rect(10, 10, 50, 40),
			want: image.Rect(10, 10, 50, 40),
		},
		{
			name: "negative Min clamped to 0",
			in:   image.Rect(-5, -10, 50, 40),
			want: image.Rect(0, 0, 50, 40),
		},
		{
			name: "Max beyond image clamped",
			in:   image.Rect(10, 10, 150, 200),
			want: image.Rect(10, 10, 100, 80),
		},
		{
			// Regression: inverted rect (Min > Max) must be normalised via Canon().
			name: "inverted rect normalised via Canon",
			in:   image.Rect(60, 50, 20, 10),
			want: image.Rect(20, 10, 60, 50),
		},
		{
			name: "zero-size rect passthrough",
			in:   image.Rect(5, 5, 5, 5),
			want: image.Rect(5, 5, 5, 5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CorrectRectangle(*img, tt.in)
			if got != tt.want {
				t.Errorf("CorrectRectangle() = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("unfilled image returns zero rect", func(t *testing.T) {
		empty := newEmptyImage()
		got := CorrectRectangle(*empty, image.Rect(10, 10, 50, 50))
		if got != (image.Rectangle{}) {
			t.Errorf("expected zero rect for unfilled image, got %v", got)
		}
	})
}

// ----------------------------------------------------------------------------
// RectAddWidth
// ----------------------------------------------------------------------------

func TestRectAddWidth(t *testing.T) {
	img := newTestImage(100, 80)
	defer img.Cleanup()

	tests := []struct {
		name     string
		rect     image.Rectangle
		addWidth int
		wantDx   int
	}{
		{
			name:     "even width added symmetrically",
			rect:     image.Rect(30, 20, 70, 60), // 30 left, 30 right available
			addWidth: 10,
			wantDx:   50,
		},
		{
			// Regression: odd addWidth previously dropped 1 pixel due to integer
			// truncation; the fixed code always adds the full requested amount.
			name:     "odd width full amount added",
			rect:     image.Rect(30, 20, 70, 60), // 30 left, 30 right available
			addWidth: 5,
			wantDx:   45,
		},
		{
			name:     "more space on left than right",
			rect:     image.Rect(60, 10, 90, 50), // 60 left, 10 right
			addWidth: 20,
			wantDx:   50,
		},
		{
			name:     "more space on right than left",
			rect:     image.Rect(5, 10, 40, 50), // 5 left, 60 right; else branch: remain=20-5=15, Max+=15 => Dx=55
			addWidth: 20,
			wantDx:   55,
		},
		{
			name:     "zero width is no-op",
			rect:     image.Rect(10, 10, 50, 50),
			addWidth: 0,
			wantDx:   40,
		},
		{
			name:     "negative width is no-op",
			rect:     image.Rect(10, 10, 50, 50),
			addWidth: -5,
			wantDx:   40,
		},
		{
			name:     "width larger than available space clamped to image width",
			rect:     image.Rect(10, 10, 90, 50), // only 10+10=20 pixels available
			addWidth: 100,
			wantDx:   100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RectAddWidth(*img, tt.rect, tt.addWidth)
			if got.Dx() != tt.wantDx {
				t.Errorf("RectAddWidth() Dx = %d, want %d (rect=%v)", got.Dx(), tt.wantDx, got)
			}
			if got.Min.X < 0 || got.Max.X > img.Width() {
				t.Errorf("RectAddWidth() result %v out of image bounds [0,%d]", got, img.Width())
			}
		})
	}
}

// ----------------------------------------------------------------------------
// RectAddHeight
// ----------------------------------------------------------------------------

func TestRectAddHeight(t *testing.T) {
	img := newTestImage(100, 80)
	defer img.Cleanup()

	tests := []struct {
		name      string
		rect      image.Rectangle
		addHeight int
		wantDy    int
	}{
		{
			name:      "even height added symmetrically",
			rect:      image.Rect(10, 20, 50, 60), // 20 above, 20 below available
			addHeight: 10,
			wantDy:    50,
		},
		{
			// Regression: same odd-value fix as RectAddWidth.
			name:      "odd height full amount added",
			rect:      image.Rect(10, 20, 50, 60),
			addHeight: 7,
			wantDy:    47,
		},
		{
			name:      "more space below than above",
			rect:      image.Rect(10, 5, 50, 40), // 5 above, 40 below; else branch: remain=20-5=15, Max+=15 => Dy=55
			addHeight: 20,
			wantDy:    55,
		},
		{
			name:      "zero height is no-op",
			rect:      image.Rect(10, 10, 50, 60),
			addHeight: 0,
			wantDy:    50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RectAddHeight(*img, tt.rect, tt.addHeight)
			if got.Dy() != tt.wantDy {
				t.Errorf("RectAddHeight() Dy = %d, want %d (rect=%v)", got.Dy(), tt.wantDy, got)
			}
			if got.Min.Y < 0 || got.Max.Y > img.Height() {
				t.Errorf("RectAddHeight() result %v out of image bounds [0,%d]", got, img.Height())
			}
		})
	}
}

// ----------------------------------------------------------------------------
// RectScale
// ----------------------------------------------------------------------------

func TestRectScale(t *testing.T) {
	img := newTestImage(200, 200)
	defer img.Cleanup()

	tests := []struct {
		name    string
		rect    image.Rectangle
		scale   float64
		wantMin image.Point
		wantMax image.Point
	}{
		{
			name:    "scale 2x integer",
			rect:    image.Rect(10, 20, 30, 40),
			scale:   2.0,
			wantMin: image.Point{X: 20, Y: 40},
			wantMax: image.Point{X: 60, Y: 80},
		},
		{
			// Regression: the old math.Ceil path turned 1.5 into 2; per-coordinate
			// float math now preserves the fractional factor correctly.
			name:    "fractional scale 1.5x not rounded to 2x",
			rect:    image.Rect(10, 10, 20, 20),
			scale:   1.5,
			wantMin: image.Point{X: 15, Y: 15},
			wantMax: image.Point{X: 30, Y: 30},
		},
		{
			name:    "scale 0.5x shrink",
			rect:    image.Rect(10, 10, 20, 20),
			scale:   0.5,
			wantMin: image.Point{X: 5, Y: 5},
			wantMax: image.Point{X: 10, Y: 10},
		},
		{
			name:    "scale 1.0x identity",
			rect:    image.Rect(5, 5, 15, 15),
			scale:   1.0,
			wantMin: image.Point{X: 5, Y: 5},
			wantMax: image.Point{X: 15, Y: 15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RectScale(*img, tt.rect, tt.scale)
			if got.Min != tt.wantMin || got.Max != tt.wantMax {
				t.Errorf("RectScale(scale=%.2f) = %v, want Min=%v Max=%v",
					tt.scale, got, tt.wantMin, tt.wantMax)
			}
		})
	}

	t.Run("zero scale returns zero rect", func(t *testing.T) {
		got := RectScale(*img, image.Rect(10, 10, 50, 50), 0.0)
		if got != (image.Rectangle{}) {
			t.Errorf("expected zero rect for scale=0, got %v", got)
		}
	})

	t.Run("negative scale returns zero rect", func(t *testing.T) {
		got := RectScale(*img, image.Rect(10, 10, 50, 50), -1.0)
		if got != (image.Rectangle{}) {
			t.Errorf("expected zero rect for scale<0, got %v", got)
		}
	})

	t.Run("result clamped to image bounds", func(t *testing.T) {
		got := RectScale(*img, image.Rect(10, 10, 180, 180), 10.0)
		if got.Max.X > img.Width() || got.Max.Y > img.Height() {
			t.Errorf("RectScale() result %v exceeds image bounds %dx%d", got, img.Width(), img.Height())
		}
	})

	t.Run("unfilled image returns zero rect", func(t *testing.T) {
		empty := newEmptyImage()
		got := RectScale(*empty, image.Rect(10, 10, 50, 50), 2.0)
		if got != (image.Rectangle{}) {
			t.Errorf("expected zero rect for unfilled image, got %v", got)
		}
	})
}

// ----------------------------------------------------------------------------
// RectPadded
// ----------------------------------------------------------------------------

func TestRectPadded(t *testing.T) {
	img := newTestImage(200, 200)
	defer img.Cleanup()

	tests := []struct {
		name           string
		rect           image.Rectangle
		paddingPercent int
		minDx          int
		minDy          int
	}{
		{
			name:           "10 percent padding grows rect",
			rect:           image.Rect(50, 50, 150, 150), // 100x100
			paddingPercent: 10,
			minDx:          109, // 100 + floor(100*10/100) = 110; allow 109 for int division
			minDy:          109,
		},
		{
			name:           "zero padding is no-op",
			rect:           image.Rect(50, 50, 150, 150),
			paddingPercent: 0,
			minDx:          100,
			minDy:          100,
		},
		{
			name:           "negative padding is no-op",
			rect:           image.Rect(50, 50, 150, 150),
			paddingPercent: -5,
			minDx:          100,
			minDy:          100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RectPadded(*img, tt.rect, tt.paddingPercent)
			if got.Dx() < tt.minDx {
				t.Errorf("RectPadded() Dx = %d, want >= %d", got.Dx(), tt.minDx)
			}
			if got.Dy() < tt.minDy {
				t.Errorf("RectPadded() Dy = %d, want >= %d", got.Dy(), tt.minDy)
			}
			if got.Min.X < 0 || got.Min.Y < 0 || got.Max.X > img.Width() || got.Max.Y > img.Height() {
				t.Errorf("RectPadded() result %v out of image bounds", got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// RectSquare
// ----------------------------------------------------------------------------

func TestRectSquare(t *testing.T) {
	img := newTestImage(200, 200)
	defer img.Cleanup()

	tests := []struct {
		name string
		rect image.Rectangle
	}{
		{name: "wider than tall", rect: image.Rect(10, 50, 110, 100)},  // 100 wide, 50 tall
		{name: "taller than wide", rect: image.Rect(50, 10, 100, 110)}, // 50 wide, 100 tall
		{name: "already square", rect: image.Rect(20, 20, 80, 80)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RectSquare(*img, tt.rect)
			if got.Dx() != got.Dy() {
				t.Errorf("RectSquare() not square: Dx=%d Dy=%d (rect=%v)", got.Dx(), got.Dy(), got)
			}
			if got.Min.X < 0 || got.Min.Y < 0 || got.Max.X > img.Width() || got.Max.Y > img.Height() {
				t.Errorf("RectSquare() result %v out of image bounds", got)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// RectRect (16:9)
// ----------------------------------------------------------------------------

func TestRectRect(t *testing.T) {
	img := newTestImage(1920, 1080)
	defer img.Cleanup()

	t.Run("narrower rect expanded to 16:9", func(t *testing.T) {
		// height=720 => target width = 720*16/9 = 1280
		got := RectRect(*img, image.Rect(300, 180, 660, 900))
		targetW := got.Dy() * 16 / 9
		if got.Dx() < targetW {
			t.Errorf("RectRect() Dx=%d, want >= %d for height %d", got.Dx(), targetW, got.Dy())
		}
	})

	t.Run("already-wider rect is unchanged", func(t *testing.T) {
		in := image.Rect(0, 100, 1920, 200)
		got := RectRect(*img, in)
		corrected := CorrectRectangle(*img, in)
		if got.Dx() != corrected.Dx() {
			t.Errorf("RectRect() changed width on already-wide rect: got %d, want %d", got.Dx(), corrected.Dx())
		}
	})

	t.Run("result within image bounds", func(t *testing.T) {
		got := RectRect(*img, image.Rect(100, 100, 500, 700))
		if got.Min.X < 0 || got.Min.Y < 0 || got.Max.X > img.Width() || got.Max.Y > img.Height() {
			t.Errorf("RectRect() result %v out of image bounds %dx%d", got, img.Width(), img.Height())
		}
	})
}

// ----------------------------------------------------------------------------
// RectRelative
// ----------------------------------------------------------------------------

func TestRectRelative(t *testing.T) {
	img := newTestImage(200, 200)
	defer img.Cleanup()

	t.Run("child offset by parent min", func(t *testing.T) {
		child := image.Rect(5, 5, 20, 20)
		parent := image.Rect(50, 50, 150, 150)
		got := RectRelative(*img, child, parent)
		want := image.Rect(55, 55, 70, 70)
		if got != want {
			t.Errorf("RectRelative() = %v, want %v", got, want)
		}
	})

	t.Run("result clamped to image bounds", func(t *testing.T) {
		child := image.Rect(100, 100, 180, 180)
		parent := image.Rect(100, 100, 190, 190)
		got := RectRelative(*img, child, parent)
		if got.Min.X < 0 || got.Min.Y < 0 || got.Max.X > img.Width() || got.Max.Y > img.Height() {
			t.Errorf("RectRelative() result %v out of image bounds", got)
		}
	})
}

// ----------------------------------------------------------------------------
// RectOverlap
// ----------------------------------------------------------------------------

func TestRectOverlap(t *testing.T) {
	tests := []struct {
		name   string
		r1     image.Rectangle
		r2     image.Rectangle
		wantP1 int
		wantP2 int
	}{
		{
			name:   "no overlap",
			r1:     image.Rect(0, 0, 10, 10),
			r2:     image.Rect(20, 20, 30, 30),
			wantP1: 0,
			wantP2: 0,
		},
		{
			name:   "full overlap identical rects",
			r1:     image.Rect(0, 0, 10, 10),
			r2:     image.Rect(0, 0, 10, 10),
			wantP1: 100,
			wantP2: 100,
		},
		{
			name:   "r2 fully inside r1",
			r1:     image.Rect(0, 0, 100, 100),
			r2:     image.Rect(25, 25, 75, 75),
			wantP1: 25,  // 50*50 / 100*100
			wantP2: 100, // r2 fully covered
		},
		{
			name:   "50 percent of each rect overlapped",
			r1:     image.Rect(0, 0, 10, 10),
			r2:     image.Rect(5, 0, 15, 10),
			wantP1: 50,
			wantP2: 50,
		},
		{
			name:   "zero-area rect returns 0",
			r1:     image.Rect(5, 5, 5, 5),
			r2:     image.Rect(0, 0, 10, 10),
			wantP1: 0,
			wantP2: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p1, p2 := RectOverlap(tt.r1, tt.r2)
			if p1 != tt.wantP1 || p2 != tt.wantP2 {
				t.Errorf("RectOverlap() = (%d, %d), want (%d, %d)", p1, p2, tt.wantP1, tt.wantP2)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// RectAspectRatio
// ----------------------------------------------------------------------------

func TestRectAspectRatio(t *testing.T) {
	img := newTestImage(1920, 1080)
	defer img.Cleanup()

	t.Run("4:3 aspect ratio applied", func(t *testing.T) {
		// height=300 => target width = 300*4/3 = 400
		got := RectAspectRatio(*img, image.Rect(100, 100, 200, 400), 4, 3)
		targetW := got.Dy() * 4 / 3
		if got.Dx() < targetW {
			t.Errorf("RectAspectRatio(4:3) Dx=%d, want >= %d", got.Dx(), targetW)
		}
	})

	t.Run("16:9 output matches RectRect", func(t *testing.T) {
		rect := image.Rect(100, 100, 400, 700)
		want := RectRect(*img, rect)
		got := RectAspectRatio(*img, rect, 16, 9)
		if got != want {
			t.Errorf("RectAspectRatio(16:9) = %v, want %v (same as RectRect)", got, want)
		}
	})

	t.Run("already correct ratio is unchanged", func(t *testing.T) {
		// 160 wide, 90 tall => exact 16:9
		in := image.Rect(100, 100, 260, 190)
		got := RectAspectRatio(*img, in, 16, 9)
		corrected := CorrectRectangle(*img, in)
		if got.Dx() != corrected.Dx() {
			t.Errorf("RectAspectRatio() changed width on already-correct ratio: got %d, want %d", got.Dx(), corrected.Dx())
		}
	})

	t.Run("zero aspectW returns corrected rect only", func(t *testing.T) {
		in := image.Rect(10, 10, 50, 50)
		got := RectAspectRatio(*img, in, 0, 9)
		want := CorrectRectangle(*img, in)
		if got != want {
			t.Errorf("RectAspectRatio(0:9) = %v, want %v", got, want)
		}
	})

	t.Run("result within image bounds", func(t *testing.T) {
		got := RectAspectRatio(*img, image.Rect(100, 100, 300, 800), 16, 9)
		if got.Min.X < 0 || got.Min.Y < 0 || got.Max.X > img.Width() || got.Max.Y > img.Height() {
			t.Errorf("RectAspectRatio() result %v out of image bounds", got)
		}
	})

	t.Run("unfilled image returns zero rect", func(t *testing.T) {
		empty := newEmptyImage()
		got := RectAspectRatio(*empty, image.Rect(10, 10, 50, 50), 16, 9)
		if got != (image.Rectangle{}) {
			t.Errorf("expected zero rect for unfilled image, got %v", got)
		}
	})
}
