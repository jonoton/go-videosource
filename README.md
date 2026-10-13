# go-videosource

Package `videosource` provides a set of tools for working with video sources (RTSP/IP cameras and video files), frame buffering, image processing, and video recording using [GoCV](https://gocv.io/).

[![Go Reference](https://pkg.go.dev/badge/github.com/jonoton/go-videosource.svg)](https://pkg.go.dev/github.com/jonoton/go-videosource)

## Features

- **Video Sources (`VideoSource`)**:
  - `IPCamSource`: Reads network streams (e.g., RTSP) with configurable FFmpeg capture timeouts, custom `OPENCV_FFMPEG_CAPTURE_OPTIONS`, and transient frame failure tolerance.
  - `FileSource`: Reads local video files.
- **`VideoReader`**: Reads from a `VideoSource` at a configurable source FPS and outputs frames through a rate-matched frame buffer (`MaxOutputFps`), tracking source and output `FrameStats`.
- **`VideoWriter`**: Buffers and records processed frames (`ProcessedImage`) to video clips and snapshot images on trigger events or continuously.
- **`Image` & `ProcessedImage`**: Reference-counted `gocv.Mat` wrappers (`go-sharedmat`) with scaling, cropping, quality adjustment, and motion/object highlight overlays.
- **Rectangle Utilities**: Bounds clamping, padding, scaling, aspect-ratio adjustment, and overlap calculation.

## IPCamSource & FFmpeg Capture Options

`IPCamSource` configures `OPENCV_FFMPEG_CAPTURE_OPTIONS` using pipe-delimited `key;value` pairs when initializing the stream.

```go
ipcam := videosource.NewIPCamSource("front-cam", "rtsp://camera-ip:554/stream1")

// Set socket timeout in milliseconds (converted to stimeout in microseconds)
ipcam.SetCaptureTimeoutMs(3000)

// Set custom FFmpeg capture options (combined with stimeout via "|")
ipcam.SetCaptureOptions("rtsp_transport;tcp|err_detect;explode")

// Set how many consecutive failed frame reads are tolerated before disconnecting (default: 10)
ipcam.SetMaxFailureTolerance(10)

reader := videosource.NewVideoReader(ipcam, 30, 10)
images := reader.Start()
defer reader.Stop()

for img := range images {
    // Process frame...
    img.Cleanup()
}
reader.Wait()
```

### Using `err_detect;explode`

When `err_detect;explode` is enabled, FFmpeg aborts decoding corrupted frames instead of concealing bitstream errors (which can cause grey/smeared frames), causing `Read(&mat)` to fail for the corrupted frame:
- `IPCamSource.ReadImage()` drops single corrupted frames (`done = false`, unfilled `Image`) so `VideoReader` records them as dropped frames without tearing down the stream.
- If `Read(&mat)` fails for `maxFailureTolerance` consecutive frames (default `10`), the source transitions to `Disconnected` and returns `done = true`.
- Always pair `err_detect;explode` with `rtsp_transport;tcp` (`rtsp_transport;tcp|err_detect;explode`) so normal UDP packet loss does not trigger frame drops.

