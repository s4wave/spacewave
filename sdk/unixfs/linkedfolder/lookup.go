package unixfs_linkedfolder

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/world"
)

// The details a running link shows in its node status. The Device record
// carries them, so they change only on a transition.
const (
	// StatusStarting shows a link that has not read its folder yet.
	StatusStarting = "starting"
	// StatusIndexing shows a full sync of the folder.
	StatusIndexing = "indexing"
	// StatusWatching shows the link following the folder. A suffix after a
	// semicolon names the files waiting out a backoff.
	StatusWatching = "watching"
)

// RunningLink is a linked folder running on this Device.
type RunningLink interface {
	// GetConfig returns the link's config.
	GetConfig() *Config
	// GetLinkProgress returns the files and bytes the current or last full
	// sync linked, and a channel closed at their next change. The counts
	// stay on this Device: they change after each committed batch, and no
	// World record carries them.
	GetLinkProgress() (LinkProgress, <-chan struct{})
}

// LinkProgress counts the files and bytes a full sync linked so far.
type LinkProgress struct {
	// Files is the number of files linked.
	Files uint64
	// Bytes is the number of file bytes linked.
	Bytes uint64
}

// LookupLinkedFolder selects the linked folders running on this Device that
// write one Drive. Each value is a RunningLink, withdrawn when its controller
// stops.
type LookupLinkedFolder interface {
	directive.Directive

	// LookupLinkedFolderEngineID returns the World engine holding the Drive.
	LookupLinkedFolderEngineID() string
	// LookupLinkedFolderObjectKey returns the Drive's object key.
	LookupLinkedFolderObjectKey() string
}

// lookupLinkedFolder implements LookupLinkedFolder.
type lookupLinkedFolder struct {
	// engineID is the World engine holding the Drive.
	engineID string
	// objectKey is the Drive's object key.
	objectKey string
}

// NewLookupLinkedFolder constructs a LookupLinkedFolder for the Drive at
// objectKey in the World engine engineID.
func NewLookupLinkedFolder(engineID, objectKey string) LookupLinkedFolder {
	return &lookupLinkedFolder{engineID: engineID, objectKey: objectKey}
}

// WatchLinkedFolders calls cb with the links running on this Device that write
// the Drive, and again each time the set changes, until ctx ends. The release
// function stops the watch.
func WatchLinkedFolders(ctx context.Context, b bus.Bus, engineID, objectKey string, cb func([]RunningLink)) (func(), error) {
	_, release, err := bus.ExecCollectValuesWatch[RunningLink](ctx, b, NewLookupLinkedFolder(engineID, objectKey), true,
		func(_ []error, links []RunningLink) error {
			cb(links)
			return nil
		}, nil)
	return release, err
}

// ResolveLinkedFolder resolves dir with link when link writes its Drive.
func ResolveLinkedFolder(dir LookupLinkedFolder, link RunningLink) ([]directive.Resolver, error) {
	conf := link.GetConfig()
	if conf.GetEngineId() != dir.LookupLinkedFolderEngineID() || conf.GetObjectKey() != dir.LookupLinkedFolderObjectKey() {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]RunningLink{link}), nil)
}

// LookupLinkedFolderEngineID returns the World engine holding the Drive.
func (d *lookupLinkedFolder) LookupLinkedFolderEngineID() string { return d.engineID }

// LookupLinkedFolderObjectKey returns the Drive's object key.
func (d *lookupLinkedFolder) LookupLinkedFolderObjectKey() string { return d.objectKey }

// Validate requires the engine and the Drive.
func (d *lookupLinkedFolder) Validate() error {
	if d.engineID == "" {
		return world.ErrEmptyEngineID
	}
	if d.objectKey == "" {
		return world.ErrEmptyObjectKey
	}
	return nil
}

// GetValueOptions releases unused lookups immediately.
func (d *lookupLinkedFolder) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{UnrefDisposeEmptyImmediate: true}
}

// IsEquivalent shares lookups for the same Drive.
func (d *lookupLinkedFolder) IsEquivalent(other directive.Directive) bool {
	o, ok := other.(LookupLinkedFolder)
	return ok && o.LookupLinkedFolderEngineID() == d.engineID && o.LookupLinkedFolderObjectKey() == d.objectKey
}

// Superceeds leaves other lookups independent.
func (d *lookupLinkedFolder) Superceeds(directive.Directive) bool { return false }

// GetName identifies the lookup in diagnostics.
func (d *lookupLinkedFolder) GetName() string { return "LookupLinkedFolder" }

// GetDebugVals identifies the Drive.
func (d *lookupLinkedFolder) GetDebugVals() directive.DebugValues {
	return directive.DebugValues{"engine-id": []string{d.engineID}, "object-key": []string{d.objectKey}}
}

// _ is a type assertion
var _ LookupLinkedFolder = (*lookupLinkedFolder)(nil)
