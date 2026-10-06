//go:build goscript

package goscript_opfs_storage

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/opfs"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_test "github.com/s4wave/spacewave/db/store/test"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	device_opfs "github.com/s4wave/spacewave/db/volume/device/opfs"
	volume_idb "github.com/s4wave/spacewave/db/volume/idb"
)

const (
	// opfsVolume names the browser volume the device selection puts on OPFS.
	opfsVolume = "goscript-browser-volume-opfs"
	// idbVolume names the browser volume kept in its existing IndexedDB database.
	idbVolume = "goscript-browser-volume-idb"
	// volumeBlock is the block written before and read after the restart.
	volumeBlock = "browser volume block"
)

// writeVolumes runs the store contract on a fresh browser volume on each
// device and leaves one block in each for the restart.
func writeVolumes() error {
	// Clear earlier runs, then create the IndexedDB database so the device
	// selection keeps the second volume there.
	ctx := context.Background()
	for _, name := range []string{opfsVolume, idbVolume} {
		if err := device_opfs.Delete(name); err != nil {
			return err
		}
		if err := volume_idb.DeleteDatabase(name); err != nil {
			return err
		}
	}
	dev, err := volume_idb.OpenDevice(ctx, idbVolume)
	if err != nil {
		return err
	}
	if err := dev.Close(); err != nil {
		return err
	}

	// Run the contract and store the restart block on each device.
	for _, name := range []string{opfsVolume, idbVolume} {
		if err := writeVolume(ctx, name); err != nil {
			return errors.Wrap(err, name)
		}
	}
	return checkDevices(true)
}

// writeVolume runs the store contract on volume name and stores the restart
// block, checking the block statistics.
func writeVolume(ctx context.Context, name string) error {
	// Run the store contract on the opened volume.
	vol, err := openVolume(ctx, name)
	if err != nil {
		return err
	}
	defer vol.Close()
	if err := store_test.TestAll(ctx, vol); err != nil {
		return err
	}

	// Store the restart block and check that the statistics count it.
	before, err := vol.GetStorageStats(ctx)
	if err != nil {
		return err
	}
	if _, _, err := vol.PutBlock(ctx, []byte(volumeBlock), nil); err != nil {
		return err
	}
	if _, err := vol.Sync(ctx); err != nil {
		return err
	}
	after, err := vol.GetStorageStats(ctx)
	if err != nil {
		return err
	}
	if after.GetBlockCount() != before.GetBlockCount()+1 || after.GetTotalBytes() != before.GetTotalBytes()+uint64(len(volumeBlock)) {
		return errors.Errorf("stats %v after the put, %v before", after, before)
	}
	return nil
}

// readVolumes checks that each browser volume kept its block across the
// worker restart, then deletes both.
func readVolumes() error {
	// Read the restart block from each volume and delete it.
	ctx := context.Background()
	for _, name := range []string{opfsVolume, idbVolume} {
		if err := readVolume(ctx, name); err != nil {
			return errors.Wrap(err, name)
		}
	}
	return checkDevices(false)
}

// readVolume checks the restart block of volume name and deletes the volume.
func readVolume(ctx context.Context, name string) error {
	// Read the block stored before the restart.
	vol, err := openVolume(ctx, name)
	if err != nil {
		return err
	}
	ref, err := block.BuildBlockRef([]byte(volumeBlock), nil)
	if err != nil {
		vol.Close()
		return err
	}
	data, found, err := vol.GetBlock(ctx, ref)
	if err != nil {
		vol.Close()
		return err
	}
	if !found || string(data) != volumeBlock {
		vol.Close()
		return errors.New("block did not survive the worker restart")
	}

	// Delete the volume and its device data.
	return vol.Delete()
}

// checkDevices checks where the two volumes are: the first on OPFS only and
// the second in IndexedDB only while present is set, and neither otherwise.
func checkDevices(present bool) error {
	// Check the OPFS directories.
	root, err := opfs.GetRoot()
	if err != nil {
		return err
	}
	for name, want := range map[string]bool{opfsVolume: present, idbVolume: false} {
		_, err := opfs.GetDirectory(root, name, false)
		if err != nil && !opfs.IsNotFound(err) {
			return err
		}
		if got := err == nil; got != want {
			return errors.Errorf("OPFS directory %s present %v, want %v", name, got, want)
		}
	}

	// Check the IndexedDB databases.
	for name, want := range map[string]bool{opfsVolume: false, idbVolume: present} {
		got, err := volume_idb.DatabaseExists(name)
		if err != nil {
			return err
		}
		if got != want {
			return errors.Errorf("IndexedDB database %s present %v, want %v", name, got, want)
		}
	}
	return nil
}

// openVolume opens the browser volume name.
func openVolume(ctx context.Context, name string) (*volume_browser.Volume, error) {
	return volume_browser.NewVolume(ctx, nil, &volume_browser.Config{
		Name:        name,
		StoreConfig: &store_kvtx.Config{},
	})
}
