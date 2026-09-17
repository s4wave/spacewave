package block

// defaultBufferedStore* are the defaults applied when a setting is unset.
const (
	defaultBufferedStoreMaxPendingEntries       = 4096
	defaultBufferedStoreMaxPendingBytes         = 64 << 20
	defaultBufferedStoreMaxPendingMetadataBytes = 64 << 20
)

// BufferedStoreSettings configures buffered block writeback behavior.
type BufferedStoreSettings struct {
	// MaxPendingEntries is the maximum queued entries before a drain.
	MaxPendingEntries int
	// MaxPendingBytes is the maximum retained payload bytes before a drain.
	MaxPendingBytes int
	// MaxPendingMetadataBytes bounds retained reference data plus conservative
	// per-entry/reference accounting, independently of payload bytes.
	MaxPendingMetadataBytes int
	// DrainBatchEntries is the number of entries written per drain batch.
	DrainBatchEntries int
}

// DefaultBufferedStoreSettings returns the default buffered store settings.
func DefaultBufferedStoreSettings() *BufferedStoreSettings {
	return &BufferedStoreSettings{
		MaxPendingEntries:       defaultBufferedStoreMaxPendingEntries,
		MaxPendingBytes:         defaultBufferedStoreMaxPendingBytes,
		MaxPendingMetadataBytes: defaultBufferedStoreMaxPendingMetadataBytes,
	}
}

// normalizeBufferedStoreSettings applies defaults and clamps negative
// values, returning a copy when s is non-nil.
func normalizeBufferedStoreSettings(s *BufferedStoreSettings) *BufferedStoreSettings {
	if s == nil {
		return DefaultBufferedStoreSettings()
	}
	out := *s
	if out.MaxPendingEntries < 0 {
		out.MaxPendingEntries = 0
	}
	if out.MaxPendingBytes < 0 {
		out.MaxPendingBytes = 0
	}
	if out.MaxPendingEntries == 0 {
		out.MaxPendingEntries = defaultBufferedStoreMaxPendingEntries
	}
	if out.MaxPendingBytes == 0 {
		out.MaxPendingBytes = defaultBufferedStoreMaxPendingBytes
	}
	if out.MaxPendingMetadataBytes <= 0 {
		out.MaxPendingMetadataBytes = defaultBufferedStoreMaxPendingMetadataBytes
	}
	if out.DrainBatchEntries < 0 {
		out.DrainBatchEntries = 0
	}
	return &out
}
