//go:build linux && cgo && gstreamer

#include "pipeline.h"

/* Wake the reader for each sample. The sink bounds queued samples to one. */
static GstFlowReturn sample_ready(GstAppSink *sink, gpointer bus) {
  gst_bus_post(bus, gst_message_new_application(
      GST_OBJECT(sink), gst_structure_new_empty("sample")));
  return GST_FLOW_OK;
}

Pipeline *open_pipeline(GError **error) {
  gst_init(NULL, NULL);
  GstElement *graph = gst_parse_launch(
      "videotestsrc is-live=true ! "
      "video/x-raw,format=NV12,width=1920,height=1080,framerate=30/1 ! "
      "vah264enc name=encoder b-frames=0 key-int-max=120 ! h264parse ! "
      "video/x-h264,stream-format=avc,alignment=au ! "
      "appsink name=encoded emit-signals=true sync=false max-buffers=1",
      error);
  if (*error != NULL) {
    if (graph != NULL) {
      gst_object_unref(graph);
    }
    return NULL;
  }

  Pipeline *pipeline = g_new0(Pipeline, 1);
  pipeline->graph = graph;
  pipeline->bus = gst_element_get_bus(graph);
  pipeline->sink = GST_APP_SINK(gst_bin_get_by_name(GST_BIN(graph), "encoded"));
  GstElement *encoder = gst_bin_get_by_name(GST_BIN(graph), "encoder");
  pipeline->encoder_output = gst_element_get_static_pad(encoder, "src");
  gst_object_unref(encoder);
  g_signal_connect(pipeline->sink, "new-sample", G_CALLBACK(sample_ready),
                   pipeline->bus);
  if (gst_element_set_state(graph, GST_STATE_PLAYING) ==
      GST_STATE_CHANGE_FAILURE) {
    GstMessage *message = gst_bus_pop_filtered(pipeline->bus, GST_MESSAGE_ERROR);
    if (message != NULL) {
      gst_message_parse_error(message, error, NULL);
      gst_message_unref(message);
    } else {
      g_set_error_literal(error, GST_CORE_ERROR, GST_CORE_ERROR_STATE_CHANGE,
                          "GStreamer pipeline failed to start");
    }
    close_pipeline(pipeline);
    return NULL;
  }
  return pipeline;
}

GstSample *read_sample(Pipeline *pipeline, GError **error) {
  GstMessage *event = gst_bus_timed_pop_filtered(
      pipeline->bus, GST_CLOCK_TIME_NONE,
      GST_MESSAGE_APPLICATION | GST_MESSAGE_EOS | GST_MESSAGE_ERROR);
  if (event == NULL) {
    return NULL;
  }

  GstSample *sample = NULL;
  switch (GST_MESSAGE_TYPE(event)) {
  case GST_MESSAGE_APPLICATION:
    if (gst_message_has_name(event, "sample")) {
      sample = gst_app_sink_pull_sample(pipeline->sink);
    }
    break;
  case GST_MESSAGE_ERROR:
    gst_message_parse_error(event, error, NULL);
    break;
  default:
    break;
  }
  gst_message_unref(event);
  return sample;
}

gboolean request_idr(Pipeline *pipeline) {
  return gst_pad_send_event(pipeline->encoder_output,
                            gst_video_event_new_upstream_force_key_unit(
                                GST_CLOCK_TIME_NONE, TRUE, 0));
}

void stop_pipeline(Pipeline *pipeline) {
  gst_bus_post(pipeline->bus, gst_message_new_application(
      GST_OBJECT(pipeline->graph), gst_structure_new_empty("stop")));
}

void close_pipeline(Pipeline *pipeline) {
  gst_element_set_state(pipeline->graph, GST_STATE_NULL);
  gst_object_unref(pipeline->encoder_output);
  gst_object_unref(pipeline->sink);
  gst_object_unref(pipeline->bus);
  gst_object_unref(pipeline->graph);
  g_free(pipeline);
}

gboolean map_picture(Pipeline *pipeline, GstSample *sample, Picture *picture) {
  picture->buffer = gst_sample_get_buffer(sample);
  picture->keyframe =
      !GST_BUFFER_FLAG_IS_SET(picture->buffer, GST_BUFFER_FLAG_DELTA_UNIT);
  if (!gst_buffer_map(picture->buffer, &picture->bytes, GST_MAP_READ)) {
    return FALSE;
  }

  if (picture->keyframe) {
    const GstStructure *caps =
        gst_caps_get_structure(gst_sample_get_caps(sample), 0);
    const GValue *description = gst_structure_get_value(caps, "codec_data");
    if (description == NULL ||
        !gst_structure_get_int(caps, "width", &picture->width) ||
        !gst_structure_get_int(caps, "height", &picture->height)) {
      gst_buffer_unmap(picture->buffer, &picture->bytes);
      return FALSE;
    }
    picture->description = gst_value_get_buffer(description);
    if (!gst_buffer_map(picture->description, &picture->avcc, GST_MAP_READ)) {
      gst_buffer_unmap(picture->buffer, &picture->bytes);
      return FALSE;
    }
  }

  /* Convert the sample's presentation timestamp to age in the pipeline clock. */
  GstClockTime pts = gst_segment_to_running_time(
      gst_sample_get_segment(sample), GST_FORMAT_TIME,
      GST_BUFFER_PTS(picture->buffer));
  GstClockTime now = gst_element_get_current_running_time(pipeline->graph);
  if (GST_CLOCK_TIME_IS_VALID(pts) && GST_CLOCK_TIME_IS_VALID(now) && now > pts) {
    picture->age = now - pts;
  }
  return TRUE;
}

void unmap_picture(Picture *picture) {
  if (picture->description != NULL) {
    gst_buffer_unmap(picture->description, &picture->avcc);
  }
  gst_buffer_unmap(picture->buffer, &picture->bytes);
}
