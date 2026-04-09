package block

const (
	defaultBufferedStoreMaxPendingEntries = 4096
	defaultBufferedStoreMaxPendingBytes   = 64 << 20
)

// BufferedStoreSettings configures buffered block writeback behavior.
type BufferedStoreSettings struct {
	MaxPendingEntries int
	MaxPendingBytes   int
	DrainBatchEntries int
}

// DefaultBufferedStoreSettings returns the default buffered store settings.
func DefaultBufferedStoreSettings() *BufferedStoreSettings {
	return &BufferedStoreSettings{
		MaxPendingEntries: defaultBufferedStoreMaxPendingEntries,
		MaxPendingBytes:   defaultBufferedStoreMaxPendingBytes,
	}
}

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
	if out.MaxPendingEntries == 0 && out.MaxPendingBytes == 0 && out.DrainBatchEntries == 0 {
		return DefaultBufferedStoreSettings()
	}
	if out.MaxPendingEntries == 0 {
		out.MaxPendingEntries = defaultBufferedStoreMaxPendingEntries
	}
	if out.MaxPendingBytes == 0 {
		out.MaxPendingBytes = defaultBufferedStoreMaxPendingBytes
	}
	if out.DrainBatchEntries < 0 {
		out.DrainBatchEntries = 0
	}
	return &out
}
