package resource_unixfs

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
)

type uploadMetricRecorder struct {
	metrics []UploadMetric
}

func (r *uploadMetricRecorder) RecordUploadMetric(metric UploadMetric) {
	r.metrics = append(r.metrics, metric)
}

func (r *uploadMetricRecorder) stages() []string {
	stages := make([]string, 0, len(r.metrics))
	for _, metric := range r.metrics {
		stages = append(stages, metric.Stage)
	}
	return stages
}

func (r *uploadMetricRecorder) totalBytes(stage string) int {
	var total int
	for _, metric := range r.metrics {
		if metric.Stage == stage {
			total += metric.Bytes
		}
	}
	return total
}

func (r *uploadMetricRecorder) countStage(stage string) int {
	var count int
	for _, metric := range r.metrics {
		if metric.Stage == stage {
			count++
		}
	}
	return count
}

func TestFSHandleResourceUploadTreeMetrics(t *testing.T) {
	// Start the filesystem resource testbed for upload metric recording.
	ctx, _, rootHandle, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Upload a directory and file while recording every upload stage.
	recorder := &uploadMetricRecorder{}
	metricCtx := WithUploadMetricsRecorder(ctx, recorder)
	resp, err := rootResource.UploadTree(&uploadTreeMetricStream{
		ctx: metricCtx,
		messages: []*s4wave_unixfs.HandleUploadTreeRequest{
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
					Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
						Path: "metric-dir",
						Mode: 0o755,
					},
				},
			},
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
					FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
						Path:      "metric-dir/file.txt",
						TotalSize: 5,
						Mode:      0o644,
						Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
					},
				},
			},
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
					Data: []byte("hello"),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rootResource.GetHandle() != rootHandle {
		defer rootResource.GetHandle().Release()
	}
	assertUploadTreeCounters(t, resp, 5, 1, 1)

	// Verify the upload stages and their per-stage counts and byte totals.
	wantStages := []string{
		"receive-directory",
		"receive-file-start",
		"receive-data",
		"commit-start",
		"commit-complete",
		"reload-start",
		"reload-complete",
		"broadcast",
	}
	if !slices.Equal(recorder.stages(), wantStages) {
		t.Fatalf("stages = %v, want %v", recorder.stages(), wantStages)
	}
	for _, want := range []struct {
		stage string
		count int
		bytes int
	}{
		{stage: "receive-directory", count: 1},
		{stage: "receive-file-start", count: 1},
		{stage: "receive-data", count: 1, bytes: 5},
		{stage: "commit-start", count: 1},
		{stage: "commit-complete", count: 1},
		{stage: "reload-start", count: 1},
		{stage: "reload-complete", count: 1},
		{stage: "broadcast", count: 1},
	} {
		// Verify the expected event count and byte total for this upload stage.
		if got := recorder.countStage(want.stage); got != want.count {
			t.Fatalf("%s metric count = %d, want %d", want.stage, got, want.count)
		}
		if got := recorder.totalBytes(want.stage); got != want.bytes {
			t.Fatalf("%s metric bytes = %d, want %d", want.stage, got, want.bytes)
		}
	}

	// Read the uploaded file directly from the published filesystem root.
	bfs := unixfs_billy.NewBillyFS(ctx, rootResource.GetHandle(), "", time.Now())
	data, err := billy_util.ReadFile(bfs, "metric-dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the uploaded bytes and report the recorded stage measurements.
	if string(data) != "hello" {
		t.Fatalf("got data %q, want %q", string(data), "hello")
	}
	for _, metric := range recorder.metrics {
		t.Logf(
			"metric workload=resource-upload-tree file_class=resource-unixfs stage=%s bytes_written=%d files_written=%d directories_written=%d",
			metric.Stage,
			metric.Bytes,
			resp.GetWritten().GetFilesWritten(),
			resp.GetWritten().GetDirectoriesWritten(),
		)
	}
}

func TestFSHandleResourceUploadTreeMetricsAbortCleanup(t *testing.T) {
	// Start the filesystem resource testbed for failed upload metrics.
	ctx, _, _, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Upload an invalid absolute path while recording its cleanup stages.
	recorder := &uploadMetricRecorder{}
	metricCtx := WithUploadMetricsRecorder(ctx, recorder)
	_, err := rootResource.UploadTree(&uploadTreeMetricStream{
		ctx: metricCtx,
		messages: []*s4wave_unixfs.HandleUploadTreeRequest{
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
					Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
						Path: "/metric-dir",
						Mode: 0o755,
					},
				},
			},
		},
	})

	// Verify the upload fails and records its abort cleanup.
	if err == nil {
		t.Fatal("expected absolute upload path to fail")
	}
	if !slices.Equal(recorder.stages(), []string{"receive-directory", "abort-cleanup"}) {
		t.Fatalf("stages = %v, want [receive-directory abort-cleanup]", recorder.stages())
	}
}

type uploadTreeMetricStream struct {
	ctx      context.Context
	messages []*s4wave_unixfs.HandleUploadTreeRequest
}

func (s *uploadTreeMetricStream) Context() context.Context { return s.ctx }

func (s *uploadTreeMetricStream) MsgSend(srpc.Message) error { return nil }

func (s *uploadTreeMetricStream) MsgRecv(msg srpc.Message) error {
	out, ok := msg.(*s4wave_unixfs.HandleUploadTreeRequest)
	if !ok {
		return errors.New("unexpected upload tree message type")
	}
	return s.RecvTo(out)
}

func (s *uploadTreeMetricStream) CloseSend() error { return nil }

func (s *uploadTreeMetricStream) Close() error { return nil }

func (s *uploadTreeMetricStream) Recv() (*s4wave_unixfs.HandleUploadTreeRequest, error) {
	if len(s.messages) == 0 {
		return nil, io.EOF
	}
	msg := s.messages[0]
	s.messages = s.messages[1:]
	return msg, nil
}

func (s *uploadTreeMetricStream) RecvTo(out *s4wave_unixfs.HandleUploadTreeRequest) error {
	msg, err := s.Recv()
	if err != nil {
		return err
	}
	*out = *msg
	return nil
}
