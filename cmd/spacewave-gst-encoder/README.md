# Display encoder

Build the helper on Linux with GStreamer app and video development libraries.

```sh
CGO_ENABLED=1 go build -tags=gstreamer ./cmd/spacewave-gst-encoder
```

Put `spacewave-gst-encoder` on the Device process's PATH. The helper needs a VA-API
H.264 encoder (`vah264enc`) and the GStreamer `h264parse` element. Set
`source=gstreamer-test` on a Display node to serve 1920×1080 synthetic pictures
at 30 frames per second. The node uses AVC and the existing Display subscription.

## KDE Plasma capture

Run the Device and the helper as the Plasma session user, with its session bus
and PipeWire runtime. Build the helper with the PipeWire GStreamer source
installed on that machine. Set `source=plasma` on the Display node to use its
local Plasma session.

The helper prefers PipeWire DMA-BUFs and uses VA post-processing to pass
NV12 surfaces to the hardware encoder. It uses `videoconvert` for streams
that supply system memory. The helper reports its selected path on stderr.

Run the provisioning command once from the checkout as the Plasma session user.

```sh
./cmd/spacewave-gst-encoder/spacewave-authorize-display
```

The command installs the hidden desktop entry used to register
`org.s4wave.Spacewave.DisplayCapture` and writes the app's KDE remote desktop
grant. The entry names the running daemon for the portal. It has no launch
action. On restart, the node reads the same grant. It closes capture and input
when the portal closes the session or the grant is removed. If KDE fails to
answer Start within four seconds, the node closes the request and session and
reports that provisioning is required.

The Display's admitted viewers send motion, buttons and keys on the frame
track's control stream. Plasma receives positions in the selected monitor's
logical coordinates and keys as Linux input codes. A viewer disconnect
releases its held keys and buttons. The portal session ends input for all
viewers when capture closes.
