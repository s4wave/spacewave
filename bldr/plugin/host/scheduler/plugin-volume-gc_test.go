package plugin_host_scheduler

import (
	"context"
	"maps"
	"strconv"
	"sync"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	"github.com/sirupsen/logrus"
)

// callCounter wraps an RPC client and counts the calls made through it.
type callCounter struct {
	srpc.Client

	mtx   sync.Mutex
	calls map[string]int
}

// newCallCounter wraps the client.
func newCallCounter(client srpc.Client) *callCounter {
	return &callCounter{Client: client, calls: make(map[string]int)}
}

// ExecCall counts the call and executes it.
func (c *callCounter) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	c.count(method)
	return c.Client.ExecCall(ctx, service, method, in, out)
}

// NewStream counts the call and starts it.
func (c *callCounter) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	c.count(method)
	return c.Client.NewStream(ctx, service, method, firstMsg)
}

// count records a call of the method.
func (c *callCounter) count(method string) {
	// Increment the method's count under the lock.
	c.mtx.Lock()
	c.calls[method]++
	c.mtx.Unlock()
}

// take returns the counts since the last take and resets them.
func (c *callCounter) take() map[string]int {
	// Swap in an empty count under the lock.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	calls := c.calls
	c.calls = make(map[string]int)
	return calls
}

// servePluginBucket serves the volume of a plugin, runs the volume controller
// of the plugin's process, and returns the plugin's handle on the bucket along
// with the counter of the plugin's RPC calls.
func servePluginBucket(
	ctx context.Context,
	t testing.TB,
	c *Controller,
	tb *testbed.Testbed,
	pluginID string,
	bucketID string,
) (bucket.Bucket, *callCounter) {
	t.Helper()

	// Read the served volume's info through the counted client.
	counter := newCallCounter(servePluginClient(ctx, t, c, tb.Volume, pluginID))
	info, err := volume_rpc.NewSRPCProxyVolumeClient(counter).GetVolumeInfo(ctx, &volume_rpc.GetVolumeInfoRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Run the plugin's volume controller until the test ends.
	ctrl := volume_rpc_client.NewProxyVolumeControllerWithClient(
		tb.Bus,
		logrus.NewEntry(logrus.New()),
		info.GetVolumeInfo(),
		nil,
		counter,
		"",
	)
	ctrlCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctrlCtx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// Create the plugin's bucket and open a handle on it.
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: bucketID, Rev: 1}); err != nil {
		t.Fatal(err.Error())
	}
	handle, release, err := ctrl.BuildBucketAPI(ctx, bucketID)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(release)
	return handle.GetBucket(), counter
}

// TestPluginBucketKeepsChildLists checks a block a plugin writes through its
// volume keeps its child list in the host volume.
func TestPluginBucketKeepsChildLists(t *testing.T) {
	tests := []struct {
		name string
		// instanceKey selects a Space's scoped plugin volume when set.
		instanceKey string
	}{
		{name: "app plugin"},
		{name: "space plugin", instanceKey: "space-a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a host volume.
			ctx := t.Context()
			tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
			if err != nil {
				t.Fatal(err.Error())
			}
			defer tb.Release()
			lists, ok := tb.Volume.(interface {
				GetChildren(context.Context, *block.BlockRef) ([]*block.BlockRef, bool, error)
			})
			if !ok {
				t.Fatalf("host volume %T keeps no child lists", tb.Volume)
			}

			// The plugin writes a parent block that references a child block.
			c := &Controller{conf: &Config{InstanceKey: tt.instanceKey}}
			plugin, _ := servePluginBucket(ctx, t, c, tb, "plugin", "world")
			child, _, err := plugin.PutBlock(ctx, []byte("child"), &block.PutOpts{RefsKnown: true})
			if err != nil {
				t.Fatal(err.Error())
			}
			parent, _, err := plugin.PutBlock(ctx, []byte("parent"), &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true})
			if err != nil {
				t.Fatal(err.Error())
			}

			// The host keeps the parent's child list.
			children, listed, err := lists.GetChildren(ctx, parent)
			if err != nil || !listed || len(children) != 1 || !children[0].EqualsRef(child) {
				t.Fatalf("parent child list = %v listed %v err %v, want the child", children, listed, err)
			}
		})
	}
}

// TestPluginBucketPutBlockRPCCount checks the RPC calls of a plugin's block
// write: the block store call carries its child list, so it is the only one.
func TestPluginBucketPutBlockRPCCount(t *testing.T) {
	// Open the plugin's bucket and the volume its handle writes through.
	ctx := t.Context()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()
	c := &Controller{conf: &Config{InstanceKey: "space-a"}}
	plugin, counter := servePluginBucket(ctx, t, c, tb, "plugin", "world")
	counter.take()

	// A write through the bucket handle with a child edge.
	child, _, err := plugin.PutBlock(ctx, []byte("child"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	counter.take()
	if _, _, err := plugin.PutBlock(ctx, []byte("parent"), &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true}); err != nil {
		t.Fatal(err.Error())
	}
	got := counter.take()

	// The block store call alone carries the child list.
	want := map[string]int{"PutBlock": 1}
	if !maps.Equal(got, want) {
		t.Fatalf("RPC calls of a plugin PutBlock = %v, want %v", got, want)
	}
}

// BenchmarkPluginBucketPutBlock measures a plugin's block write through its
// bucket handle, which tracks references, against the same write straight to
// the proxy volume's block store, which does not.
func BenchmarkPluginBucketPutBlock(b *testing.B) {
	for _, tracked := range []bool{false, true} {
		name := "untracked"
		if tracked {
			name = "tracked"
		}
		b.Run(name, func(b *testing.B) {
			// Build the host volume the plugin writes through.
			ctx := b.Context()
			tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
			if err != nil {
				b.Fatal(err.Error())
			}
			defer tb.Release()

			// Open the plugin's store: its bucket handle or its proxy volume.
			c := &Controller{conf: &Config{InstanceKey: "space-a"}}
			var store block.StoreOps = servePluginVolume(ctx, b, c, tb.Volume, "plugin")
			if tracked {
				store, _ = servePluginBucket(ctx, b, c, tb, "plugin", "world")
			}

			// Write the child the parents reference.
			child, _, err := store.PutBlock(ctx, []byte("child"), &block.PutOpts{RefsKnown: true})
			if err != nil {
				b.Fatal(err.Error())
			}

			// Write a new parent of the child per iteration.
			opts := &block.PutOpts{Refs: []*block.BlockRef{child}, RefsKnown: true}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				data := []byte("parent-" + strconv.Itoa(i))
				if _, _, err := store.PutBlock(ctx, data, opts); err != nil {
					b.Fatal(err.Error())
				}
			}
		})
	}
}
