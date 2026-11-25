#include <gst/app/gstappsink.h>
#include <gst/video/video-event.h>

/* The pipeline owns its bus, sink and encoder through shutdown. */
typedef struct {
  GstElement *graph;
  GstAppSink *sink;
  GstPad *encoder_output;
  GstBus *bus;
} Pipeline;

/* A Picture borrows the sample's encoded buffer and optional AVC description. */
typedef struct {
  GstBuffer *buffer;
  GstBuffer *description;
  GstMapInfo bytes;
  GstMapInfo avcc;
  gint width;
  gint height;
  GstClockTime age;
  gboolean keyframe;
} Picture;

Pipeline *open_pipeline(GError **error);
GstSample *read_sample(Pipeline *pipeline, GError **error);
gboolean request_idr(Pipeline *pipeline);
void stop_pipeline(Pipeline *pipeline);
void close_pipeline(Pipeline *pipeline);
gboolean map_picture(Pipeline *pipeline, GstSample *sample, Picture *picture);
void unmap_picture(Picture *picture);
