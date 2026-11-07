package block_store_nodepush

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// TestNodeCopiesOffAndUploadPace turns read copies off and checks that an
// uploaded pack is not kept and a read miss queues no fetch, then that the
// upload's pace waits out each pack's share and that no pace waits nothing.
func TestNodeCopiesOffAndUploadPace(t *testing.T) {
	// Copies off: Keep stores nothing, and a miss queues no fill.
	packs := newPackFiles(t, t.TempDir(), &objectRemote{})
	packs.SetCopies(false)
	packs.Keep("p1", []byte("pack bytes"))
	packs.queueFill("p2")
	if st := packs.Stats(); st.Kept != 0 || len(packs.fills) != 0 {
		t.Fatalf("copies off kept %d packs and queued %d fills", st.Kept, len(packs.fills))
	}

	// Back on, a miss queues its fill again.
	packs.SetCopies(true)
	packs.queueFill("p2")
	if len(packs.fills) != 1 {
		t.Fatal("copies back on queued no fill")
	}

	// The pace waits a pack's share: 1 MiB at 10 MiB/s is 100 ms.
	n := &Node{}
	n.SetUploadRate(10 << 20)
	start := time.Now()
	if err := n.admitUpload(context.Background(), 1<<20); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(start); waited < 90*time.Millisecond || waited > 2*time.Second {
		t.Fatalf("paced upload waited %v, want about 100 ms", waited)
	}

	// No pace waits nothing.
	n.SetUploadRate(0)
	start = time.Now()
	if err := n.admitUpload(context.Background(), 1<<30); err != nil || time.Since(start) > 50*time.Millisecond {
		t.Fatalf("unpaced upload waited %v (%v)", time.Since(start), err)
	}
}

// TestNodeUploadOff holds the upload: pushes and journal records still
// flush, nothing uploads, reports or leaves the log; on again, it uploads and
// reports what it held.
func TestNodeUploadOff(t *testing.T) {
	// A node with its upload off.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	remote := &sftpFake{objectRemote: &objectRemote{}, uploads: map[string][]byte{}}
	node, err := OpenNode(ctx, logrus.NewEntry(logrus.New()), NodeConfig{
		Dir: t.TempDir(), Linger: 10 * time.Millisecond, Remote: remote, Source: remote,
		Bucket: "bucket", Prefix: "p/packs", List: remote.list,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	node.SetUpload(false)
	go node.Run(ctx)
	apply := &applier{applied: map[string][][]byte{}, changed: make(chan struct{}, 1)}
	client := NewClient(logrus.NewEntry(logrus.New()), apply)
	defer connect(ctx, t, client, node.server)()

	// Pushes and records flush.
	for i := range 4 {
		if err := client.Send(ctx, []*block.PutBatchEntry{entry(t, strings.Repeat("held", 20)+strconv.Itoa(i))}); err != nil {
			t.Fatal(err)
		}
		if err := client.SendRecord(ctx, &Record{Epoch: 7, Seq: uint64(i + 1)}); err != nil {
			t.Fatal(err)
		}
	}

	// Nothing uploads or reports while packs finalize.
	waitFor(t, func() bool { st := node.Stats(); return st.Log.Finalized >= 1 })
	time.Sleep(100 * time.Millisecond)
	if len(remote.uploaded()) != 0 || apply.count() != 0 {
		t.Fatalf("upload off uploaded %d packs and reported %d", len(remote.uploaded()), apply.count())
	}

	// On again, the held packs upload and report.
	node.SetUpload(true)
	waitFor(t, func() bool { return len(remote.uploaded()) >= 1 && apply.count() >= 1 })
}
