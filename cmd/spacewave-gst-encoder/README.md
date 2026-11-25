# Display encoder

Build the helper on Linux with GStreamer app and video development libraries.

```sh
CGO_ENABLED=1 go build -tags=gstreamer ./cmd/spacewave-gst-encoder
```

Put `spacewave-gst-encoder` on the Device process's PATH. The helper needs a VA-API
H.264 encoder (`vah264enc`) and the GStreamer `h264parse` element. Set
`source=gstreamer-test` on a Display node to serve 1920×1080 synthetic pictures
at 30 frames per second. The node uses AVC and the existing Display subscription.
