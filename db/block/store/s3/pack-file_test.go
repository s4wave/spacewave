package block_store_s3

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// TestPackStorePutPackFile uploads a pack a packahead Log finalized, as it
// is, and a store that only lists the bucket reads every block back with its
// refs; uploading it again changes nothing.
func TestPackStorePutPackFile(t *testing.T) {
	ctx := t.Context()

	// Write leaf and parent blocks as block objects into a packahead Log.
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	var refs []*block.BlockRef
	var datas [][]byte
	var batch []packahead.Block
	for i := range 20 {
		data := bytes.Repeat([]byte{byte(i)}, 100+i)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		var children []*block.BlockRef
		if i > 0 {
			children = refs[i-1 : i]
		}
		value, err := block.EncodeBlockObject(data, children, true)
		if err != nil {
			t.Fatal(err)
		}
		refs, datas = append(refs, ref), append(datas, data)
		batch = append(batch, packahead.Block{Key: packfile.BlockKey(ref.GetHash()), Value: value})
	}
	if err := log.Append(ctx, batch); err != nil {
		t.Fatal(err)
	}

	// Wait for the lingering pack to finalize and read its file.
	var infos []packahead.PackInfo
	for len(infos) == 0 {
		var waitCh <-chan struct{}
		infos, waitCh = log.GetPacks()
		if len(infos) == 0 {
			select {
			case <-waitCh:
			case <-time.After(10 * time.Second):
				t.Fatal("pack did not finalize")
			}
		}
	}
	data, err := os.ReadFile(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}

	// Upload it twice under its own id.
	_, client := newFakeBucket(t, nil)
	store := newTestPackStore(t, client)
	for range 2 {
		entry, err := store.PutPackFile(ctx, infos[0].ID, data)
		if err != nil {
			t.Fatal(err)
		}
		if entry.GetId() != infos[0].ID || entry.GetBlockCount() != 20 || entry.GetSizeBytes() != uint64(len(data)) {
			t.Fatalf("entry = %v", entry)
		}
	}

	// A store that lists the bucket reads every block and its refs.
	reader := newTestPackStore(t, client)
	for i, ref := range refs {
		stored, err := reader.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !bytes.Equal(stored.Data, datas[i]) {
			t.Fatalf("block %d: %v, %v", i, stored, err)
		}
		if want := min(i, 1); len(stored.Refs) != want || !stored.RefsKnown {
			t.Fatalf("block %d refs: %v known %v", i, stored.Refs, stored.RefsKnown)
		}
	}
}
