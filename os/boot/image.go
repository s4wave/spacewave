package boot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	os_image "github.com/s4wave/spacewave/os/image"
	"github.com/sirupsen/logrus"
)

// imageCursor returns a cursor on store at image's root that reads and writes
// blocks through image's transform. The caller releases it.
func imageCursor(ctx context.Context, le *logrus.Entry, store bucket.BucketOps, image *bucket.ObjectRef) (*bucket_lookup.Cursor, error) {
	// Build the image's transformer, then a cursor in the image bucket.
	sfs := transform_all.BuildFactorySet()
	conf := image.GetTransformConf()
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, conf)
	if err != nil {
		return nil, err
	}
	head := &bucket.ObjectRef{BucketId: imageBucket, RootRef: image.GetRootRef()}
	return bucket_lookup.NewCursor(ctx, nil, le, sfs, store, xfrm, head, &bucket.BucketOpArgs{BucketId: imageBucket}, conf), nil
}

// imageBlock returns the block that records image in the image store: the
// marshaled reference, with its root as the one child, so the named root that
// holds it holds the image's whole graph.
func imageBlock(image *bucket.ObjectRef) (*block.PutBatchEntry, error) {
	// Marshal the checked image and hash it as one block.
	data, err := os_image.MarshalImage(image)
	if err != nil {
		return nil, err
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: []*block.BlockRef{image.GetRootRef()}, RefsKnown: true}, nil
}

// WriteImage writes an image's UnixFS tree through cs and returns the image:
// the tree's root with cs's transform, which must be set. The tree holds
// executable as /workload and the probe payload. busybox, when set, adds the
// tool layout, and archive, when set, its regular files and directories.
func WriteImage(ctx context.Context, cs *bucket_lookup.Cursor, executable, busybox, archive io.Reader) (*bucket.ObjectRef, error) {
	// Refuse an untransformed image.
	conf := cs.GetTransformConf()
	if conf.GetEmpty() {
		return nil, errors.New("image: storage has no transform")
	}

	// Write an empty directory as the tree's first root.
	btx, bcs := cs.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	if _, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY); err != nil {
		return nil, err
	}
	root, _, err := btx.Write(ctx, true)
	if err != nil {
		return nil, err
	}

	// Open a writable view, which moves its cursor's root with each write.
	tree := cs.Clone()
	tree.SetRootRef(root)
	wr := unixfs_block_fs.NewFSWriter()
	fs := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, tree, wr)
	wr.SetFS(fs)
	defer fs.Release()

	// Write the layout through the writable view and return the moved root.
	handle, err := unixfs.NewFSHandle(fs)
	if err != nil {
		return nil, err
	}
	defer handle.Release()
	if err := buildImage(ctx, handle, executable, busybox, archive); err != nil {
		return nil, err
	}
	return &bucket.ObjectRef{RootRef: tree.GetRef().GetRootRef(), TransformConf: conf.Clone()}, nil
}

// buildImage writes the image layout into the empty directory root.
func buildImage(ctx context.Context, root *unixfs.FSHandle, executable, busybox, archive io.Reader) error {
	// Every image holds the workload and the probe payload.
	if err := writeImageFile(ctx, root, "workload", executable); err != nil {
		return err
	}
	if err := writeImageFile(ctx, root, "payload", newPayload()); err != nil {
		return err
	}

	// Retained images add tools. The diskless probe keeps its two files.
	if busybox != nil {
		if err := buildTools(ctx, root, busybox); err != nil {
			return err
		}
	}
	if archive != nil {
		return importImageArchive(ctx, root, archive)
	}
	return nil
}

// ExportImage creates the image store in the absent directory dir holding
// image as its built image, as a seed leaves it for a card, copying the
// image's encrypted blocks unchanged from src: a store that serves each block
// with its child list, such as the Space bucket the image was written to. It
// returns the number of blocks it copied. A failure leaves the partial store
// for inspection.
func ExportImage(ctx context.Context, le *logrus.Entry, dir string, src block.StoreOps, image *bucket.ObjectRef) (_ int, retErr error) {
	// Build the naming block first, so a bad reference creates nothing.
	built, err := imageBlock(image)
	if err != nil {
		return 0, err
	}
	vol, err := createImageStore(ctx, le, dir)
	if err != nil {
		return 0, err
	}
	defer func() { retErr = errors.Join(retErr, vol.Close()) }()

	// Copy the graph, then select the built image.
	copied, err := copyRevision(ctx, vol, src, image.GetRootRef())
	if err != nil {
		return copied, err
	}
	return copied, publishBuiltImage(ctx, vol, built)
}

// SeedRoot creates the image store in the absent directory dir and writes an
// image into it under transform from the independent executable, a static
// BusyBox and an optional application archive, then selects it as the built
// image. A failure leaves the partial store for inspection, never a reset.
func SeedRoot(ctx context.Context, le *logrus.Entry, dir string, transform *block_transform.Config, executable, busybox, archive io.Reader) (retErr error) {
	// Retained images always include explicit tools, never a partial probe image.
	if executable == nil || busybox == nil {
		return errors.New("seed requires independent executable and static BusyBox inputs")
	}
	vol, err := createImageStore(ctx, le, dir)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, vol.Close()) }()

	// Write the tree under transform.
	cs, err := imageCursor(ctx, le, vol, &bucket.ObjectRef{TransformConf: transform})
	if err != nil {
		return err
	}
	defer cs.Release()
	image, err := WriteImage(ctx, cs, executable, busybox, archive)
	if err != nil {
		return errors.Join(errors.New("seed content"), err)
	}

	// Select the image as the built image.
	built, err := imageBlock(image)
	if err != nil {
		return err
	}
	if err := publishBuiltImage(ctx, vol, built); err != nil {
		return errors.Join(errors.New("select image"), err)
	}
	println("SPACEWAVE_OS_SEEDED volume=" + vol.GetID() + " root=" + image.GetRootRef().MarshalString())
	return nil
}

// createImageStore creates an empty image store in the absent directory dir.
// Exclusive creation keeps it from replacing anyone's existing store.
func createImageStore(ctx context.Context, le *logrus.Entry, dir string) (*volume_s4db.Volume, error) {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}
	return volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: filepath.Join(dir, "image.s4wave")})
}

// publishBuiltImage stores built and selects it as the built image, refusing a
// store that already selects a built image.
func publishBuiltImage(ctx context.Context, vol *volume_s4db.Volume, built *block.PutBatchEntry) error {
	return vol.SetBucketRoots(ctx, imageBucket, []*block.PutBatchEntry{built}, []block.NamedRoot{{Name: builtImageRoot, Ref: built.Ref}})
}

// writeImageFile streams one bootstrap input into the UnixFS tree as an
// executable file, with bounded memory.
func writeImageFile(ctx context.Context, root *unixfs.FSHandle, name string, src io.Reader) error {
	// Create and retain the file handle until its writes are committed.
	ts := time.Now()
	if err := root.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), 0o755, ts); err != nil {
		return err
	}
	file, err := root.Lookup(ctx, name)
	if err != nil {
		return err
	}
	defer file.Release()

	// Copy in bounded chunks, independent of the executable's size.
	buf := make([]byte, 1<<20)
	var offset int64
	for {
		n, err := src.Read(buf)
		if n != 0 {
			if err := file.WriteAt(ctx, offset, buf[:n], ts); err != nil {
				return err
			}
			offset += int64(n)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return file.Sync(ctx)
}

// verifyImageFile reads complete regular executable content with bounded memory.
// A nil expected stream checks readability and length against immutable metadata.
func verifyImageFile(ctx context.Context, root *unixfs.FSHandle, name string, expected io.Reader) (retErr error) {
	// Borrow an ordinary io/fs reader on the frozen tree.
	file, err := unixfs_iofs.NewFS(ctx, root).Open(name)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode() != 0o755 || info.Size() == 0 {
		return errors.New("invalid image executable: " + name)
	}

	// Hash the entire stored stream without collecting its contents.
	actualHash := sha256.New()
	actualBytes, err := io.Copy(actualHash, file)
	if err != nil {
		return err
	}
	if actualBytes != info.Size() {
		return errors.New("incomplete image file: " + name)
	}

	// Independent probe inputs additionally prove the complete expected content.
	if expected != nil {
		expectedHash := sha256.New()
		expectedBytes, err := io.Copy(expectedHash, expected)
		if err != nil {
			return err
		}
		if actualBytes != expectedBytes || !bytes.Equal(actualHash.Sum(nil), expectedHash.Sum(nil)) {
			return errors.New("incomplete or changed image file: " + name)
		}
	}

	// Record the complete immutable bytes, including tools on every runtime reopen.
	println("SPACEWAVE_OS_CONTENT file=" + name + " bytes=" + strconv.FormatInt(actualBytes, 10) +
		" sha256=" + hex.EncodeToString(actualHash.Sum(nil)))
	return nil
}

// newPayload provides distinct pages so faults must fetch more than one block.
func newPayload() *bytes.Reader {
	// Mix page and byte offsets across a bounded deterministic input.
	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i*31 + i/4096)
	}
	return bytes.NewReader(data)
}
