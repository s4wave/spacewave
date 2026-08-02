package sobject

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/pkg/errors"
)

const (
	// JournalFormatVersion is the durable journal payload and framing version.
	JournalFormatVersion uint32 = 1
	journalFrameVersion         = uint16(1)
	journalHeaderSize           = 28
	journalTrailerSize          = 8
	journalMaxPayload           = 4 << 20
)

var (
	journalMagic                   = [4]byte{'S', 'W', 'J', '1'}
	journalTrailerMagic            = [4]byte{'E', 'N', 'D', '!'}
	errJournalSequenceBaseMismatch = errors.New("journal sequence base mismatch")
)

type journalSequenceBaseError struct {
	observed uint64
	expected uint64
}

func (e *journalSequenceBaseError) Error() string {
	return "journal starts at sequence " + strconv.FormatUint(e.observed, 10) + ", want " + strconv.FormatUint(e.expected, 10)
}

func (e *journalSequenceBaseError) Unwrap() error {
	return errJournalSequenceBaseMismatch
}

// JournalStorage is the bounded append/replay storage contract.
type JournalStorage interface {
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Sync() error
	Size() (int64, error)
}

// JournalGenerationStore owns the stable journal identity, compact snapshot
// generations, and the active-generation publication marker. The marker is
// external to the candidate snapshot so a crash before publication leaves the
// prior generation selected.
type JournalGenerationStore interface {
	JournalIdentity() []byte
	ReadJournalGeneration() ([]byte, error)
	WriteJournalGeneration([]byte) error
	ReadJournalCheckpointGeneration(generation uint64) ([]byte, error)
	WriteJournalCheckpointGeneration(generation uint64, data []byte) error
	// WriteJournalGenerationFloor durably advances the monotonic publication
	// floor after the active marker has been atomically published.
	WriteJournalGenerationFloor(uint64) error
}

// journalGenerationFloorStore reads the monotonic publication floor and
// reports malformed or missing sidecars instead of treating them as zero.
type journalGenerationFloorStore interface {
	JournalGenerationFloor() (uint64, error)
}

// FileJournalStorage stores journal frames in a mode-0600 file.
type FileJournalStorage struct {
	file     *os.File
	path     string
	identity []byte
}

// OpenFileJournalStorage opens or creates a journal storage file.
func OpenFileJournalStorage(path string) (*FileJournalStorage, error) {
	_, pathErr := os.Stat(path)
	pathExists := pathErr == nil
	if pathErr != nil && !os.IsNotExist(pathErr) {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, pathErr.Error())
	}
	identity, initialized, identityExists, err := readJournalIdentityMetadata(path)
	if err != nil {
		return nil, err
	}
	if !identityExists {
		if pathExists {
			return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "existing journal has no persisted identity")
		}
		identity = make([]byte, sha256.Size)
		if _, err := rand.Read(identity); err != nil {
			return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "generate journal identity")
		}
		if err := writeJournalIdentityMetadata(path, identity, false); err != nil {
			return nil, errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
		}
	}
	if initialized && !pathExists {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "initialized journal file is missing")
	}
	if !initialized && pathExists {
		stat, statErr := os.Stat(path)
		if statErr != nil {
			return nil, errors.Wrap(ErrJournalCheckpointCorrupt, statErr.Error())
		}
		if stat.Size() != 0 {
			return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "initializing journal is nonempty")
		}
	}
	markerData, markerErr := os.ReadFile(path + ".generation")
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, markerErr.Error())
	}
	if !initialized && len(markerData) > 0 {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "initializing journal has an active generation")
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, errors.Wrap(err, "open shared object journal")
	}
	closeWithError := func(openErr error) (*FileJournalStorage, error) {
		_ = file.Close()
		return nil, openErr
	}
	stat, err := file.Stat()
	if err != nil {
		return closeWithError(errors.Wrap(err, "stat shared object journal"))
	}
	if stat.Mode().Perm()&0o077 != 0 {
		return closeWithError(ErrJournalInsecureFileMode)
	}
	if !initialized {
		if err := ensureJournalGenerationFloor(path, identity, true); err != nil {
			return closeWithError(err)
		}
		floorData, err := os.ReadFile(journalGenerationFloorPath(path))
		if err != nil {
			return closeWithError(errors.Wrap(ErrJournalCheckpointCorrupt, err.Error()))
		}
		floor, err := unmarshalJournalGenerationFloor(floorData, identity)
		if err != nil {
			return closeWithError(err)
		}
		if floor != 0 {
			return closeWithError(errors.Wrap(ErrJournalCheckpointCorrupt, "initializing journal floor is nonzero"))
		}
		if err := file.Sync(); err != nil {
			return closeWithError(errors.Wrap(err, "sync shared object journal"))
		}
		if err := syncJournalDirectory(filepath.Dir(path)); err != nil {
			return closeWithError(errors.Wrap(err, "sync shared object journal directory"))
		}
		if err := writeJournalIdentityMetadata(path, identity, true); err != nil {
			return closeWithError(errors.Wrap(ErrJournalCheckpointCorrupt, err.Error()))
		}
		initialized = true
	} else if err := ensureJournalGenerationFloor(path, identity, false); err != nil {
		return closeWithError(err)
	}
	if !initialized {
		return closeWithError(errors.Wrap(ErrJournalCheckpointCorrupt, "journal identity was not initialized"))
	}
	return &FileJournalStorage{file: file, path: path, identity: identity}, nil
}

const (
	journalIdentityMetadataSize = 48
	journalIdentityInitialized  = byte(1)
)

var journalIdentityMetadataMagic = [4]byte{'S', 'W', 'I', '1'}

func marshalJournalIdentityMetadata(identity []byte, initialized bool) ([]byte, error) {
	if len(identity) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid persisted journal identity")
	}
	data := make([]byte, journalIdentityMetadataSize)
	copy(data[:4], journalIdentityMetadataMagic[:])
	binary.BigEndian.PutUint32(data[4:8], JournalFormatVersion)
	copy(data[8:40], identity)
	if initialized {
		data[40] = journalIdentityInitialized
	}
	binary.BigEndian.PutUint32(data[44:48], crc32.Checksum(data[:44], crc32.MakeTable(crc32.Castagnoli)))
	return data, nil
}

func unmarshalJournalIdentityMetadata(data []byte) ([]byte, bool, error) {
	if len(data) != journalIdentityMetadataSize ||
		!bytes.Equal(data[:4], journalIdentityMetadataMagic[:]) ||
		binary.BigEndian.Uint32(data[4:8]) != JournalFormatVersion ||
		(data[40] != 0 && data[40] != journalIdentityInitialized) ||
		binary.BigEndian.Uint32(data[44:48]) != crc32.Checksum(data[:44], crc32.MakeTable(crc32.Castagnoli)) {
		return nil, false, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid persisted journal identity metadata")
	}
	return slices.Clone(data[8:40]), data[40] == journalIdentityInitialized, nil
}

func readJournalIdentityMetadata(path string) ([]byte, bool, bool, error) {
	identityPath := path + ".identity"
	data, err := os.ReadFile(identityPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, false, nil
		}
		return nil, false, false, errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	stat, err := os.Stat(identityPath)
	if err != nil {
		return nil, false, false, errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if stat.Mode().Perm()&0o077 != 0 {
		return nil, false, false, errors.Wrap(ErrJournalCheckpointCorrupt, "insecure persisted journal identity")
	}
	identity, initialized, err := unmarshalJournalIdentityMetadata(data)
	if err != nil {
		return nil, false, false, err
	}
	return identity, initialized, true, nil
}

func writeJournalIdentityMetadata(path string, identity []byte, initialized bool) error {
	data, err := marshalJournalIdentityMetadata(identity, initialized)
	if err != nil {
		return err
	}
	return writeJournalSidecar(path+".identity", data, filepath.Dir(path))
}

const journalGenerationFloorSize = 52

var journalGenerationFloorMagic = [4]byte{'S', 'W', 'F', '1'}

func marshalJournalGenerationFloor(identity []byte, generation uint64) ([]byte, error) {
	if len(identity) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal floor identity")
	}
	data := make([]byte, journalGenerationFloorSize)
	copy(data[:4], journalGenerationFloorMagic[:])
	binary.BigEndian.PutUint32(data[4:8], JournalFormatVersion)
	copy(data[8:40], identity)
	binary.BigEndian.PutUint64(data[40:48], generation)
	binary.BigEndian.PutUint32(data[48:52], crc32.Checksum(data[:48], crc32.MakeTable(crc32.Castagnoli)))
	return data, nil
}

func unmarshalJournalGenerationFloor(data, identity []byte) (uint64, error) {
	if len(data) != journalGenerationFloorSize || len(identity) != sha256.Size ||
		!bytes.Equal(data[:4], journalGenerationFloorMagic[:]) ||
		binary.BigEndian.Uint32(data[4:8]) != JournalFormatVersion ||
		!bytes.Equal(data[8:40], identity) ||
		crc32.Checksum(data[:48], crc32.MakeTable(crc32.Castagnoli)) != binary.BigEndian.Uint32(data[48:52]) {
		return 0, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation floor")
	}
	return binary.BigEndian.Uint64(data[40:48]), nil
}

func ensureJournalGenerationFloor(path string, identity []byte, create bool) error {
	floorPath := journalGenerationFloorPath(path)
	data, err := os.ReadFile(floorPath)
	if err == nil {
		stat, statErr := os.Stat(floorPath)
		if statErr != nil {
			return errors.Wrap(ErrJournalCheckpointCorrupt, statErr.Error())
		}
		if stat.Mode().Perm()&0o077 != 0 {
			return errors.Wrap(ErrJournalCheckpointCorrupt, "insecure journal generation floor")
		}
		_, err = unmarshalJournalGenerationFloor(data, identity)
		return err
	}
	if !os.IsNotExist(err) {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if !create {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "existing journal has no persisted generation floor")
	}
	data, err = marshalJournalGenerationFloor(identity, 0)
	if err != nil {
		return err
	}
	if err := writeJournalSidecar(floorPath, data, filepath.Dir(path)); err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	return nil
}

func syncJournalDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// ReadAt reads journal bytes at an absolute offset.
func (s *FileJournalStorage) ReadAt(p []byte, offset int64) (int, error) {
	return s.file.ReadAt(p, offset)
}

// WriteAt writes journal bytes at an absolute offset.
func (s *FileJournalStorage) WriteAt(p []byte, offset int64) (int, error) {
	return s.file.WriteAt(p, offset)
}

// Truncate removes bytes after the validated journal prefix.
func (s *FileJournalStorage) Truncate(size int64) error {
	return s.file.Truncate(size)
}

// Sync flushes journal bytes and metadata.
func (s *FileJournalStorage) Sync() error {
	return s.file.Sync()
}

// Size reports the current journal byte length.
func (s *FileJournalStorage) Size() (int64, error) {
	stat, err := s.file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *FileJournalStorage) JournalIdentity() []byte {
	return slices.Clone(s.identity)
}

func (s *FileJournalStorage) ReadJournalGeneration() ([]byte, error) {
	data, err := os.ReadFile(s.path + ".generation")
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func (s *FileJournalStorage) WriteJournalGeneration(data []byte) error {
	return writeJournalSidecar(s.path+".generation", data, filepath.Dir(s.path))
}

// WriteJournalGenerationFloor durably advances the rollback-prevention floor.
// It is intentionally separate from marker publication so marker is the
// recoverable publication event.
func (s *FileJournalStorage) WriteJournalGenerationFloor(generation uint64) error {
	if generation == 0 {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation floor")
	}
	current, err := s.JournalGenerationFloor()
	if err != nil {
		return err
	}
	if generation <= current {
		return nil
	}
	encoded, err := marshalJournalGenerationFloor(s.identity, generation)
	if err != nil {
		return err
	}
	return writeJournalSidecar(journalGenerationFloorPath(s.path), encoded, filepath.Dir(s.path))
}

func (s *FileJournalStorage) JournalGenerationFloor() (uint64, error) {
	data, err := os.ReadFile(journalGenerationFloorPath(s.path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation floor is missing")
		}
		return 0, errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	return unmarshalJournalGenerationFloor(data, s.identity)
}

func (s *FileJournalStorage) ReadJournalCheckpointGeneration(generation uint64) ([]byte, error) {
	data, err := os.ReadFile(journalCheckpointPath(s.path, generation))
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

func (s *FileJournalStorage) WriteJournalCheckpointGeneration(generation uint64, data []byte) error {
	return writeJournalSidecar(journalCheckpointPath(s.path, generation), data, filepath.Dir(s.path))
}

func journalCheckpointPath(path string, generation uint64) string {
	return path + ".checkpoint." + strconv.FormatUint(generation, 10)
}

func journalGenerationFloorPath(path string) string {
	return path + ".generation.floor"
}

func writeJournalSidecar(path string, data []byte, directory string) error {
	tempPath := path + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncJournalDirectory(directory)
}

// Close closes the journal file.
func (s *FileJournalStorage) Close() error {
	return s.file.Close()
}

// memoryJournalStorage is an in-memory JournalStorage for tests in this package.
type memoryJournalStorage struct {
	mu sync.Mutex

	data                   []byte
	durable                []byte
	identity               []byte
	generations            map[uint64][]byte
	marker                 []byte
	generationFloor        uint64
	writeLimit             int
	writeErr               error
	syncErr                error
	checkpointErr          error
	generationErr          error
	generationFloorErr     error
	generationFloorReadErr error
	truncateErr            error
}

// NewMemoryJournalStorage constructs an empty in-memory journal.
func newMemoryJournalStorage() *memoryJournalStorage {
	return &memoryJournalStorage{identity: journalDefaultIdentity(), generations: make(map[uint64][]byte)}
}

func newMemoryJournalStorageWithIdentity(identity []byte) *memoryJournalStorage {
	if len(identity) != sha256.Size {
		identity = journalDefaultIdentity()
	}
	return &memoryJournalStorage{identity: slices.Clone(identity), generations: make(map[uint64][]byte)}
}

func (s *memoryJournalStorage) JournalIdentity() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.identity)
}

func (s *memoryJournalStorage) ReadJournalGeneration() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.marker), nil
}

func (s *memoryJournalStorage) WriteJournalGeneration(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generationErr != nil {
		return s.generationErr
	}
	s.marker = slices.Clone(data)
	return nil
}

func (s *memoryJournalStorage) WriteJournalGenerationFloor(generation uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generationFloorErr != nil {
		return s.generationFloorErr
	}
	if generation == 0 {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation floor")
	}
	if generation > s.generationFloor {
		s.generationFloor = generation
	}
	return nil
}

func (s *memoryJournalStorage) JournalGenerationFloor() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generationFloorReadErr != nil {
		return 0, s.generationFloorReadErr
	}
	return s.generationFloor, nil
}

func (s *memoryJournalStorage) ReadJournalCheckpointGeneration(generation uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.generations[generation]), nil
}

func (s *memoryJournalStorage) WriteJournalCheckpointGeneration(generation uint64, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.checkpointErr != nil {
		return s.checkpointErr
	}
	s.generations[generation] = slices.Clone(data)
	return nil
}

// ReadAt reads journal bytes at an absolute offset.
func (s *memoryJournalStorage) ReadAt(p []byte, offset int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if offset < 0 {
		return 0, errors.New("negative journal offset")
	}
	if offset >= int64(len(s.data)) {
		return 0, io.EOF
	}
	n := copy(p, s.data[offset:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt writes journal bytes at an absolute offset.
func (s *memoryJournalStorage) WriteAt(p []byte, offset int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if offset < 0 {
		return 0, errors.New("negative journal offset")
	}
	if s.writeErr != nil {
		if s.writeLimit >= 0 && s.writeLimit < len(p) {
			n := s.writeLimit
			s.ensureSize(offset + int64(n))
			copy(s.data[offset:], p[:n])
			return n, s.writeErr
		}
		return 0, s.writeErr
	}
	s.ensureSize(offset + int64(len(p)))
	copy(s.data[offset:], p)
	return len(p), nil
}
func (s *memoryJournalStorage) setGenerationFloorFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generationFloorErr = err
}

func (s *memoryJournalStorage) setGenerationFloorReadFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generationFloorReadErr = err
}

func (s *memoryJournalStorage) setTruncateFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.truncateErr = err
}

// Truncate removes bytes after the validated journal prefix.
func (s *memoryJournalStorage) Truncate(size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if size < 0 {
		return errors.New("negative journal size")
	}
	if s.truncateErr != nil {
		return s.truncateErr
	}
	if size < int64(len(s.data)) {
		s.data = s.data[:size]
		return nil
	}
	s.ensureSize(size)
	return nil
}

// Sync applies the configured sync result and advances the durable prefix.
func (s *memoryJournalStorage) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncErr != nil {
		s.data = slices.Clone(s.durable)
		return s.syncErr
	}
	s.durable = slices.Clone(s.data)
	return nil
}

// Size reports the current journal byte length.
func (s *memoryJournalStorage) Size() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.data)), nil
}

// Bytes returns a copy of the current journal bytes.
func (s *memoryJournalStorage) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.data)
}

// SetWriteFailure makes future writes fail, optionally after a partial prefix.
func (s *memoryJournalStorage) setWriteFailure(limit int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeLimit = limit
	s.writeErr = err
}

// SetSyncFailure makes future syncs fail.
func (s *memoryJournalStorage) setSyncFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncErr = err
}

func (s *memoryJournalStorage) setCheckpointFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkpointErr = err
}

func (s *memoryJournalStorage) setGenerationFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generationErr = err
}

func (s *memoryJournalStorage) ensureSize(size int64) {
	if size <= int64(len(s.data)) {
		return
	}
	s.data = append(s.data, make([]byte, size-int64(len(s.data)))...)
}

// journalPendingActivation records a marker/floor publication that is valid
// enough to verify but has not yet earned retirement of the observed segment.
type journalPendingActivation struct {
	marker journalGenerationMarker
	floor  uint64
}

// journalWriter appends validated frames and becomes permanently poisoned after a write failure.
type journalWriter struct {
	mu sync.Mutex

	storage    JournalStorage
	crypto     *JournalCrypto
	offset     int64
	sequence   uint64
	records    []*SOJournalRecord
	reducer    *JournalReducer
	generation uint64
	identity   []byte
	poisoned   error
	pending    *journalPendingActivation
}

const journalGenerationMarkerSize = 144

var journalGenerationMagic = [4]byte{'S', 'W', 'G', '1'}

type journalGenerationMarker struct {
	Identity       []byte
	Generation     uint64
	NextSequence   uint64
	SnapshotLength uint32
	SnapshotDigest []byte
	RetiredLength  uint64
	RetiredDigest  []byte
}

func marshalJournalGenerationMarker(marker journalGenerationMarker) ([]byte, error) {
	if len(marker.Identity) != sha256.Size || marker.Generation == 0 || marker.NextSequence == 0 ||
		marker.SnapshotLength == 0 || len(marker.SnapshotDigest) != sha256.Size ||
		len(marker.RetiredDigest) != sha256.Size {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation marker")
	}
	data := make([]byte, journalGenerationMarkerSize)
	copy(data[:4], journalGenerationMagic[:])
	binary.BigEndian.PutUint32(data[4:8], JournalFormatVersion)
	copy(data[8:40], marker.Identity)
	binary.BigEndian.PutUint64(data[40:48], marker.Generation)
	binary.BigEndian.PutUint64(data[48:56], marker.NextSequence)
	binary.BigEndian.PutUint32(data[56:60], marker.SnapshotLength)
	copy(data[60:92], marker.SnapshotDigest)
	binary.BigEndian.PutUint64(data[92:100], marker.RetiredLength)
	copy(data[100:132], marker.RetiredDigest)
	crc := crc32.Checksum(data[:140], crc32.MakeTable(crc32.Castagnoli))
	binary.BigEndian.PutUint32(data[140:144], crc)
	return data, nil
}

func unmarshalJournalGenerationMarker(data, identity []byte) (journalGenerationMarker, error) {
	var marker journalGenerationMarker
	if len(data) != journalGenerationMarkerSize || !bytes.Equal(data[:4], journalGenerationMagic[:]) ||
		binary.BigEndian.Uint32(data[4:8]) != JournalFormatVersion ||
		len(identity) != sha256.Size || !bytes.Equal(data[8:40], identity) ||
		crc32.Checksum(data[:140], crc32.MakeTable(crc32.Castagnoli)) != binary.BigEndian.Uint32(data[140:144]) {
		return marker, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation marker")
	}
	marker.Identity = slices.Clone(data[8:40])
	marker.Generation = binary.BigEndian.Uint64(data[40:48])
	marker.NextSequence = binary.BigEndian.Uint64(data[48:56])
	marker.SnapshotLength = binary.BigEndian.Uint32(data[56:60])

	marker.SnapshotDigest = slices.Clone(data[60:92])

	marker.RetiredLength = binary.BigEndian.Uint64(data[92:100])
	marker.RetiredDigest = slices.Clone(data[100:132])
	if marker.Generation == 0 || marker.NextSequence == 0 || marker.SnapshotLength == 0 {
		return journalGenerationMarker{}, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid journal generation marker values")
	}
	return marker, nil
}

func cloneJournalGenerationMarker(marker journalGenerationMarker) journalGenerationMarker {
	marker.Identity = slices.Clone(marker.Identity)
	marker.SnapshotDigest = slices.Clone(marker.SnapshotDigest)
	marker.RetiredDigest = slices.Clone(marker.RetiredDigest)
	return marker
}

func marshalCompactJournalCheckpoint(identity []byte, generation, nextSequence uint64, reducer *JournalReducer) ([]byte, error) {
	if len(identity) != sha256.Size || generation == 0 || nextSequence == 0 || reducer == nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "invalid compact checkpoint input")
	}
	checkpoint := &SOJournalCheckpoint{
		JournalIdentity: slices.Clone(identity),
		Generation:      generation,
		NextSequence:    nextSequence,
	}
	for _, snapshot := range reducer.Snapshot() {
		compactSnapshot := cloneJournalAttempt(snapshot)
		compactSnapshot.LookupHistory = nil
		if err := validateCheckpointAttempt(compactSnapshot); err != nil {
			return nil, err
		}
		checkpoint.Attempts = append(checkpoint.Attempts, &SOJournalCheckpointAttempt{
			Key:                    compactSnapshot.Key.CloneVT(),
			Lineage:                compactSnapshot.Lineage.CloneVT(),
			Version:                compactSnapshot.Version.CloneVT(),
			State:                  compactSnapshot.State,
			Readiness:              compactSnapshot.Readiness,
			Intent:                 compactSnapshot.Intent.CloneVT(),
			Envelope:               compactSnapshot.Envelope.CloneVT(),
			EnvelopeDigest:         slices.Clone(compactSnapshot.EnvelopeDigest),
			Receipt:                compactSnapshot.Receipt.CloneVT(),
			Acknowledgement:        compactSnapshot.Acknowledgement.CloneVT(),
			Projection:             compactSnapshot.Projection.CloneVT(),
			Lookup:                 compactSnapshot.Lookup.CloneVT(),
			IntentSequence:         compactSnapshot.IntentSequence,
			EnvelopeSequence:       compactSnapshot.EnvelopeSequence,
			CheckpointEligible:     compactSnapshot.CheckpointEligible,
			SendAttempted:          compactSnapshot.SendAttempted,
			ResendAuthorized:       compactSnapshot.ResendAuthorized,
			LineageRecoveryBlocked: compactSnapshot.LineageRecoveryBlocked,
		})
	}
	data, err := checkpoint.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "marshal compact checkpoint")
	}
	return data, nil
}

func unmarshalCompactJournalCheckpoint(data, identity []byte, generation, nextSequence uint64) (*JournalReducer, error) {
	checkpoint := new(SOJournalCheckpoint)
	if err := checkpoint.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "decode compact checkpoint")
	}
	if !bytes.Equal(checkpoint.GetJournalIdentity(), identity) || checkpoint.GetGeneration() != generation || checkpoint.GetNextSequence() != nextSequence {
		return nil, errors.Wrap(ErrJournalCheckpointCorrupt, "compact checkpoint metadata mismatch")
	}
	reducer := NewJournalReducer()
	snapshots := make([]*JournalAttemptSnapshot, 0, len(checkpoint.GetAttempts()))
	for _, compact := range checkpoint.GetAttempts() {
		snapshot := &JournalAttemptSnapshot{
			Key:                    compact.GetKey().CloneVT(),
			Lineage:                compact.GetLineage().CloneVT(),
			Version:                compact.GetVersion().CloneVT(),
			State:                  compact.GetState(),
			Readiness:              compact.GetReadiness(),
			Intent:                 compact.GetIntent().CloneVT(),
			Envelope:               compact.GetEnvelope().CloneVT(),
			EnvelopeDigest:         slices.Clone(compact.GetEnvelopeDigest()),
			Receipt:                compact.GetReceipt().CloneVT(),
			Acknowledgement:        compact.GetAcknowledgement().CloneVT(),
			Projection:             compact.GetProjection().CloneVT(),
			Lookup:                 compact.GetLookup().CloneVT(),
			IntentSequence:         compact.GetIntentSequence(),
			EnvelopeSequence:       compact.GetEnvelopeSequence(),
			CheckpointEligible:     compact.GetCheckpointEligible(),
			SendAttempted:          compact.GetSendAttempted(),
			ResendAuthorized:       compact.GetResendAuthorized(),
			LineageRecoveryBlocked: compact.GetLineageRecoveryBlocked(),
		}
		if snapshot.Lookup != nil {
			snapshot.LookupHistory = []*SOJournalLookup{snapshot.Lookup.CloneVT()}
		}
		if err := validateCheckpointAttempt(snapshot); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := reducer.hydrate(snapshots); err != nil {
		return nil, err
	}
	return reducer, nil
}

func validateCheckpointAttempt(attempt *JournalAttemptSnapshot) error {
	fail := func(message string) error {
		return errors.Wrap(ErrJournalCheckpointCorrupt, message)
	}
	if attempt == nil || attempt.Key == nil || attempt.Lineage == nil || attempt.Version == nil {
		return fail("checkpoint attempt identity missing")
	}
	if err := validateJournalLineage(attempt.Key, attempt.Lineage); err != nil {
		return fail(err.Error())
	}
	if !validJournalAttemptState(attempt.State) || !validJournalReadiness(attempt.Readiness) {
		return fail("checkpoint attempt lifecycle is invalid")
	}
	if len(attempt.LookupHistory) > 1 {
		return fail("checkpoint lookup history is unbounded")
	}
	if attempt.Lookup == nil && len(attempt.LookupHistory) != 0 {
		return fail("checkpoint lookup history has no current lookup")
	}
	if attempt.Lookup != nil {
		if !validJournalLookup(attempt.Lookup, attempt.Key, attempt.Lineage, attempt.Version) {
			return fail("checkpoint lookup evidence invalid")
		}
		if len(attempt.LookupHistory) == 1 && !attempt.LookupHistory[0].EqualVT(attempt.Lookup) {
			return fail("checkpoint lookup history diverges")
		}
	}
	if attempt.Intent == nil || !validEncryptedPayload(attempt.Intent) || attempt.IntentSequence == 0 {
		return fail("checkpoint intent evidence missing")
	}
	if attempt.Envelope == nil {
		if attempt.EnvelopeSequence != 0 || len(attempt.EnvelopeDigest) != 0 {
			return fail("checkpoint envelope metadata without envelope")
		}
	} else if !validEncryptedPayload(attempt.Envelope) || attempt.EnvelopeSequence == 0 ||
		attempt.EnvelopeSequence <= attempt.IntentSequence || len(attempt.EnvelopeDigest) != sha256.Size {
		return fail("checkpoint envelope evidence invalid")
	}
	if attempt.Receipt != nil {
		if attempt.Envelope == nil || !validJournalReceipt(attempt.Receipt, attempt.Key, attempt.Lineage, attempt.Version, attempt.EnvelopeDigest) {
			return fail("checkpoint receipt evidence invalid")
		}
	}
	if attempt.Acknowledgement != nil {
		if attempt.Receipt == nil || !validJournalAcknowledgement(attempt.Acknowledgement, attempt.Key) ||
			!bytes.Equal(attempt.Acknowledgement.GetReceiptDigest(), attempt.Receipt.GetTerminalReceiptDigest()) {
			return fail("checkpoint acknowledgement evidence invalid")
		}
	}
	if attempt.Projection != nil {
		if attempt.Receipt == nil || !validJournalProjection(attempt.Projection, attempt.Key) ||
			!bytes.Equal(attempt.Projection.GetReceiptDigest(), attempt.Receipt.GetTerminalReceiptDigest()) ||
			attempt.Projection.GetAuthoritativeRootSeqno() != attempt.Receipt.GetAuthoritativeRootSeqno() ||
			!bytes.Equal(attempt.Projection.GetAuthoritativeRootDigest(), attempt.Receipt.GetAuthoritativeRootDigest()) {
			return fail("checkpoint projection evidence invalid")
		}
	}
	if attempt.Receipt == nil && (attempt.Acknowledgement != nil || attempt.Projection != nil) {
		return fail("checkpoint acknowledgement or projection without receipt")
	}
	if attempt.Lookup != nil && (attempt.Lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_ACCEPTED ||
		attempt.Lookup.GetState() == SOReceiptState_SO_RECEIPT_STATE_REJECTED) {
		if attempt.Receipt == nil || !attempt.Receipt.EqualVT(attempt.Lookup.GetReceipt()) {
			return fail("checkpoint lookup receipt diverges")
		}
	}
	if attempt.ResendAuthorized && (attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT ||
		!attempt.SendAttempted || attempt.Lookup == nil ||
		attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_NO_RECORD) {
		return fail("checkpoint resend authorization is incoherent")
	}
	if attempt.LineageRecoveryBlocked && attempt.State != SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH {
		return fail("checkpoint lineage recovery block is incoherent")
	}
	switch attempt.State {
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_INTENT_DURABLE:
		if attempt.Envelope != nil || attempt.Receipt != nil || attempt.Acknowledgement != nil || attempt.Projection != nil ||
			attempt.Lookup != nil || attempt.SendAttempted || attempt.ResendAuthorized || attempt.LineageRecoveryBlocked {
			return fail("checkpoint intent state is incoherent")
		}
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_ENVELOPE_DURABLE:
		if attempt.Readiness != SOJournalReadiness_SO_JOURNAL_READINESS_READY || attempt.Envelope == nil ||
			attempt.Receipt != nil || attempt.Acknowledgement != nil || attempt.Projection != nil ||
			attempt.Lookup != nil || attempt.SendAttempted || attempt.ResendAuthorized || attempt.LineageRecoveryBlocked {
			return fail("checkpoint envelope state is incoherent")
		}
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_SENT:
		if attempt.Readiness != SOJournalReadiness_SO_JOURNAL_READINESS_READY || attempt.Envelope == nil ||
			!attempt.SendAttempted || attempt.Receipt != nil || attempt.Acknowledgement != nil || attempt.Projection != nil ||
			(attempt.Lookup != nil && attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_PENDING &&
				attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_NO_RECORD) {
			return fail("checkpoint sent state is incoherent")
		}
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECEIPT_DURABLE:
		if attempt.Readiness != SOJournalReadiness_SO_JOURNAL_READINESS_READY || attempt.Envelope == nil ||
			!attempt.SendAttempted || attempt.Receipt == nil || attempt.ResendAuthorized ||
			(attempt.Lookup != nil && attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_ACCEPTED &&
				attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_REJECTED &&
				attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_PENDING &&
				attempt.Lookup.GetState() != SOReceiptState_SO_RECEIPT_STATE_NO_RECORD) {
			return fail("checkpoint receipt state is incoherent")
		}
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_STALE_TRANSFORM_EPOCH:
		if attempt.Receipt != nil || attempt.Acknowledgement != nil || attempt.Projection != nil || attempt.ResendAuthorized ||
			(attempt.SendAttempted && !authoritativeLookupComplete(attempt.Lookup)) {
			return fail("checkpoint stale state is incoherent")
		}
	case SOJournalAttemptState_SO_JOURNAL_ATTEMPT_STATE_RECOVERY_BLOCKED:
		if attempt.Receipt != nil || attempt.Acknowledgement != nil || attempt.Projection != nil || attempt.ResendAuthorized ||
			(attempt.SendAttempted && !authoritativeLookupComplete(attempt.Lookup)) {
			return fail("checkpoint recovery state is incoherent")
		}
	default:
		return fail("checkpoint attempt state is unknown")
	}
	if attempt.CheckpointEligible != (attempt.Receipt != nil && attempt.Projection != nil) {
		return fail("checkpoint eligibility mismatch")
	}
	return nil
}

// openJournalWriter opens an existing journal and validates the published
// compact generation before reading its append-only tail.
func openJournalWriter(storage JournalStorage, cryptos ...*JournalCrypto) (*journalWriter, []*SOJournalRecord, error) {
	if journalIsNil(storage) {
		return nil, nil, ErrJournalStorageRequired
	}
	var crypto *JournalCrypto
	if len(cryptos) > 0 {
		crypto = cryptos[0]
	}
	generationStore, generationSupported := storage.(JournalGenerationStore)
	if !generationSupported {
		return nil, nil, ErrJournalStorageRequired
	}
	identity := generationStore.JournalIdentity()
	if len(identity) != sha256.Size {
		return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal identity is unavailable")
	}
	var marker journalGenerationMarker
	markerData, markerErr := generationStore.ReadJournalGeneration()
	if markerErr != nil {
		return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, markerErr.Error())
	}
	generationPresent := len(markerData) > 0
	floorStore, floorSupported := generationStore.(journalGenerationFloorStore)
	if !floorSupported {
		return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation floor is unavailable")
	}
	floor, floorErr := floorStore.JournalGenerationFloor()
	if floorErr != nil {
		return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, floorErr.Error())
	}
	if !generationPresent {
		if floor != 0 {
			return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker is missing below publication floor")
		}
	} else {
		marker, markerErr = unmarshalJournalGenerationMarker(markerData, identity)
		if markerErr != nil {
			return nil, nil, markerErr
		}
		if marker.Generation < floor {
			return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker rolled back")
		}
		if marker.Generation > floor+1 {
			return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker skips publication floor")
		}
		if crypto == nil {
			return nil, nil, errors.Wrap(ErrJournalKeyUnavailable, "journal crypto is required for checkpoint replay")
		}
		encrypted, readErr := generationStore.ReadJournalCheckpointGeneration(marker.Generation)
		if readErr != nil || len(encrypted) == 0 {
			if readErr == nil {
				readErr = errors.New("published checkpoint generation is missing")
			}
			return nil, nil, errors.Wrap(ErrJournalCheckpointCorrupt, readErr.Error())
		}
		plaintext, openErr := crypto.OpenCheckpointGeneration(encrypted, identity, marker.Generation, marker.NextSequence, int(marker.SnapshotLength), marker.SnapshotDigest)
		if openErr != nil {
			return nil, nil, openErr
		}
		reducer, unmarshalErr := unmarshalCompactJournalCheckpoint(plaintext, identity, marker.Generation, marker.NextSequence)
		clear(plaintext)
		if unmarshalErr != nil {
			return nil, nil, unmarshalErr
		}
		if err := authenticateJournalSnapshots(reducer.Snapshot(), crypto, identity); err != nil {
			return nil, nil, err
		}
		pending := marker.Generation == floor+1
		tailRecords, offset, scanErr := scanJournalFrom(storage, marker.NextSequence)
		if scanErr != nil {
			if errors.Is(scanErr, errJournalSequenceBaseMismatch) {
				pending = true
				tailRecords, offset = nil, 0
			} else {
				return nil, nil, scanErr
			}
		}
		if pending {
			retired, readErr := readJournalStorageBytes(storage)
			if readErr != nil {
				return nil, nil, errors.Wrap(ErrJournalCorrupt, readErr.Error())
			}
			if uint64(len(retired)) != marker.RetiredLength {
				return nil, nil, errors.Wrap(ErrJournalCorrupt, "observed segment does not match retired length")
			}
			retiredDigest := sha256.Sum256(retired)
			if !bytes.Equal(retiredDigest[:], marker.RetiredDigest) {
				return nil, nil, errors.Wrap(ErrJournalCorrupt, "observed segment does not match retired digest")
			}
		}
		if err := authenticateJournalRecords(tailRecords, crypto, identity); err != nil {
			return nil, nil, err
		}
		for _, record := range tailRecords {
			if err := reducer.Apply(record); err != nil {
				return nil, nil, errors.Wrap(ErrJournalCorrupt, err.Error())
			}
		}
		if !pending {
			if size, sizeErr := storage.Size(); sizeErr != nil {
				return nil, nil, errors.Wrap(sizeErr, "size shared object journal")
			} else if size != offset {
				if err := storage.Truncate(offset); err != nil {
					return nil, nil, errors.Wrap(err, "truncate shared object journal tail")
				}
				if err := storage.Sync(); err != nil {
					return nil, nil, errors.Wrap(err, "sync shared object journal tail")
				}
			}
		}
		writer := &journalWriter{
			storage: storage, crypto: crypto, offset: offset,
			sequence: marker.NextSequence + uint64(len(tailRecords)),
			records:  tailRecords, reducer: reducer,
			generation: marker.Generation, identity: slices.Clone(identity),
		}
		if pending {
			writer.pending = &journalPendingActivation{
				marker: cloneJournalGenerationMarker(marker),
				floor:  floor,
			}
		}
		return writer, slices.Clone(tailRecords), nil
	}
	records, offset, err := scanJournalFrom(storage, 1)
	if err != nil {
		return nil, nil, err
	}
	if err := authenticateJournalRecords(records, crypto, identity); err != nil {
		return nil, nil, err
	}
	reducer, err := ReduceJournal(records)
	if err != nil {
		return nil, nil, errors.Wrap(ErrJournalCorrupt, err.Error())
	}
	if size, sizeErr := storage.Size(); sizeErr != nil {
		return nil, nil, errors.Wrap(sizeErr, "size shared object journal")
	} else if size != offset {
		if err := storage.Truncate(offset); err != nil {
			return nil, nil, errors.Wrap(err, "truncate shared object journal tail")
		}
		if err := storage.Sync(); err != nil {
			return nil, nil, errors.Wrap(err, "sync shared object journal tail")
		}
	}
	return &journalWriter{
		storage: storage, crypto: crypto, offset: offset,
		sequence: uint64(len(records)) + 1, records: records,
		reducer: reducer, identity: slices.Clone(identity),
	}, slices.Clone(records), nil
}

// activatePending publishes a validated generation and retires its exact
// superseded segment. Verification callers invoke this once under the writer
// lock only after compact and tail authority checks have passed.
func (w *journalWriter) activatePending() error {
	if w == nil {
		return ErrJournalStorageRequired
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending == nil {
		return nil
	}
	if w.poisoned != nil {
		return w.poisoned
	}
	store, ok := w.storage.(JournalGenerationStore)
	if !ok {
		return ErrJournalStorageRequired
	}
	floorStore, ok := w.storage.(journalGenerationFloorStore)
	if !ok {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation floor is unavailable")
	}
	floor, err := floorStore.JournalGenerationFloor()
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if floor != w.pending.floor {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation floor changed during activation")
	}
	markerData, err := store.ReadJournalGeneration()
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	marker, err := unmarshalJournalGenerationMarker(markerData, w.identity)
	if err != nil {
		return err
	}
	expected := w.pending.marker
	if marker.Generation != expected.Generation || marker.NextSequence != expected.NextSequence ||
		marker.SnapshotLength != expected.SnapshotLength ||
		!bytes.Equal(marker.SnapshotDigest, expected.SnapshotDigest) ||
		marker.RetiredLength != expected.RetiredLength ||
		!bytes.Equal(marker.RetiredDigest, expected.RetiredDigest) {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker changed during activation")
	}
	if marker.Generation != floor && marker.Generation != floor+1 {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker is outside activation window")
	}
	retired, err := readJournalStorageBytes(w.storage)
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if uint64(len(retired)) != marker.RetiredLength {
		return errors.Wrap(ErrJournalCorrupt, "observed segment does not match retired length")
	}
	retiredDigest := sha256.Sum256(retired)
	if !bytes.Equal(retiredDigest[:], marker.RetiredDigest) {
		return errors.Wrap(ErrJournalCorrupt, "observed segment does not match retired digest")
	}
	if marker.Generation == floor+1 {
		if err := store.WriteJournalGenerationFloor(marker.Generation); err != nil {
			return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
		}
	}
	if err := w.storage.Truncate(0); err != nil {
		w.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if err := w.storage.Sync(); err != nil {
		w.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	w.offset = 0
	w.sequence = marker.NextSequence
	w.records = nil
	w.generation = marker.Generation
	w.pending = nil
	return nil
}

// Append validates and durably appends one journal record.
func (w *journalWriter) Append(record *SOJournalRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.poisoned != nil {
		return w.poisoned
	}
	if w.pending != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation activation is pending")
	}
	if record == nil {
		return errors.Wrap(ErrJournalCorrupt, "nil journal record")
	}
	clone := record.CloneVT()
	clone.FormatVersion = JournalFormatVersion
	if clone.Sequence == 0 {
		clone.Sequence = w.sequence
	} else if clone.Sequence != w.sequence {
		return errors.Wrap(ErrJournalCorrupt, "journal sequence is not writer-owned")
	}
	if err := validateJournalRecord(clone); err != nil {
		return err
	}
	if clone.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT || clone.GetKind() == SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_SIGNED_ENVELOPE {
		if w.crypto == nil {
			return errors.Wrap(ErrJournalKeyUnavailable, "journal crypto is required for staged append")
		}
		if err := authenticateJournalRecords([]*SOJournalRecord{clone}, w.crypto, w.identity); err != nil {
			return err
		}
	}
	payload, err := clone.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal shared object journal record")
	}
	frame, err := marshalJournalFrame(clone.Kind, clone.Sequence, payload)
	if err != nil {
		return err
	}
	if err := w.reducer.validate(clone); err != nil {
		return err
	}
	n, writeErr := w.storage.WriteAt(frame, w.offset)
	if writeErr == nil && n != len(frame) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		w.poisoned = errors.Wrap(ErrJournalWriterPoisoned, writeErr.Error())
		return writeErr
	}
	if err := w.storage.Sync(); err != nil {
		w.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return err
	}
	if err := w.reducer.Apply(clone); err != nil {
		w.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return err
	}
	w.offset += int64(len(frame))
	w.sequence++
	w.records = append(w.records, clone)
	return nil
}

// Replay returns a deep copy of the validated durable journal prefix.
func (w *journalWriter) Replay() []*SOJournalRecord {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	records := make([]*SOJournalRecord, len(w.records))
	for index, record := range w.records {
		records[index] = record.CloneVT()
	}
	return records
}

// journal owns one writer; production code reaches it through JournalPipeline.
type journal struct {
	writer *journalWriter
}

func openJournal(storage JournalStorage, cryptos ...*JournalCrypto) (*journal, error) {
	writer, _, err := openJournalWriter(storage, cryptos...)
	if err != nil {
		return nil, err
	}
	return &journal{writer: writer}, nil
}

func (j *journal) replay() []*SOJournalRecord {
	if j == nil || j.writer == nil {
		return nil
	}
	return j.writer.Replay()
}

func (j *journal) append(record *SOJournalRecord) error {
	if j == nil || j.writer == nil {
		return ErrJournalStorageRequired
	}
	return j.writer.Append(record)
}

func (j *journal) nextSequence() uint64 {
	if j == nil || j.writer == nil {
		return 0
	}
	j.writer.mu.Lock()
	defer j.writer.mu.Unlock()
	return j.writer.sequence
}

func (j *journal) checkpoint() error {
	if j == nil || j.writer == nil {
		return ErrJournalStorageRequired
	}
	if j.writer.crypto == nil {
		return ErrJournalKeyUnavailable
	}
	store, ok := j.writer.storage.(JournalGenerationStore)
	if !ok {
		return ErrJournalStorageRequired
	}
	j.writer.mu.Lock()
	defer j.writer.mu.Unlock()
	if j.writer.poisoned != nil {
		return j.writer.poisoned
	}
	activeGeneration := j.writer.generation
	floorStore, floorSupported := store.(journalGenerationFloorStore)
	if !floorSupported {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation floor is unavailable")
	}
	floor, floorErr := floorStore.JournalGenerationFloor()
	if floorErr != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, floorErr.Error())
	}
	if floor > activeGeneration {
		activeGeneration = floor
	}
	markerData, err := store.ReadJournalGeneration()
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if len(markerData) > 0 {
		marker, err := unmarshalJournalGenerationMarker(markerData, j.writer.identity)
		if err != nil {
			return err
		}
		if marker.Generation < floor {
			return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker rolled back")
		}
		if marker.Generation > floor+1 {
			return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker skips publication floor")
		}
		if marker.Generation > activeGeneration {
			activeGeneration = marker.Generation
		}
	} else if floor != 0 {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "journal generation marker is missing below publication floor")
	}

	retired, err := readJournalStorageBytes(j.writer.storage)
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	retiredDigest := sha256.Sum256(retired)
	generation := activeGeneration + 1
	nextSequence := j.writer.sequence
	data, err := marshalCompactJournalCheckpoint(j.writer.identity, generation, nextSequence, j.writer.reducer)
	if err != nil {
		return err
	}
	defer clear(data)
	digest := sha256.Sum256(data)
	encrypted, err := j.writer.crypto.SealCheckpointGeneration(j.writer.identity, generation, nextSequence, data)
	if err != nil {
		return err
	}
	defer clear(encrypted)
	if err := store.WriteJournalCheckpointGeneration(generation, encrypted); err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	markerData, err = marshalJournalGenerationMarker(journalGenerationMarker{
		Identity: j.writer.identity, Generation: generation,
		NextSequence: nextSequence, SnapshotLength: uint32(len(data)),
		SnapshotDigest: digest[:], RetiredLength: uint64(len(retired)),
		RetiredDigest: retiredDigest[:],
	})
	if err != nil {
		return err
	}
	if err := store.WriteJournalGeneration(markerData); err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if err := store.WriteJournalGenerationFloor(generation); err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	currentRetired, err := readJournalStorageBytes(j.writer.storage)
	if err != nil {
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	currentDigest := sha256.Sum256(currentRetired)
	if uint64(len(currentRetired)) != uint64(len(retired)) || !bytes.Equal(currentDigest[:], retiredDigest[:]) {
		return errors.Wrap(ErrJournalCheckpointCorrupt, "retired journal segment changed before publication")
	}
	if err := j.writer.storage.Truncate(0); err != nil {
		j.writer.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	if err := j.writer.storage.Sync(); err != nil {
		j.writer.poisoned = errors.Wrap(ErrJournalWriterPoisoned, err.Error())
		return errors.Wrap(ErrJournalCheckpointCorrupt, err.Error())
	}
	j.writer.offset = 0
	j.writer.generation = generation
	j.writer.records = nil
	return nil
}

func readJournalStorageBytes(storage JournalStorage) ([]byte, error) {
	size, err := storage.Size()
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(&journalReaderAt{storage: storage}, data); err != nil {
		return nil, err
	}
	return data, nil
}

type journalReaderAt struct {
	storage JournalStorage
	offset  int64
}

func (r *journalReaderAt) Read(data []byte) (int, error) {
	n, err := r.storage.ReadAt(data, r.offset)
	r.offset += int64(n)
	return n, err
}

func marshalJournalFrame(kind SOJournalRecordKind, sequence uint64, payload []byte) ([]byte, error) {
	if !validJournalRecordKind(kind) || len(payload) > journalMaxPayload {
		return nil, errors.Wrap(ErrJournalCorrupt, "invalid journal frame bounds")
	}
	frame := make([]byte, journalHeaderSize+len(payload)+journalTrailerSize)
	copy(frame[:4], journalMagic[:])
	binary.BigEndian.PutUint16(frame[4:6], journalFrameVersion)
	binary.BigEndian.PutUint16(frame[6:8], uint16(kind))
	binary.BigEndian.PutUint64(frame[8:16], sequence)
	binary.BigEndian.PutUint32(frame[16:20], uint32(len(payload)))
	headerCRC := crc32.Checksum(frame[:20], crc32.MakeTable(crc32.Castagnoli))
	binary.BigEndian.PutUint32(frame[20:24], headerCRC)
	copy(frame[journalHeaderSize:], payload)
	trailerOffset := journalHeaderSize + len(payload)
	copy(frame[trailerOffset:trailerOffset+4], journalTrailerMagic[:])
	binary.BigEndian.PutUint32(frame[trailerOffset+4:], uint32(len(payload)))
	frameCRC := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = frameCRC.Write(frame[:24])
	_, _ = frameCRC.Write(payload)
	binary.BigEndian.PutUint32(frame[24:28], frameCRC.Sum32())
	return frame, nil
}

func scanJournalFrom(storage JournalStorage, initialSequence uint64) ([]*SOJournalRecord, int64, error) {
	size, err := storage.Size()
	if err != nil {
		return nil, 0, errors.Wrap(err, "size shared object journal")
	}
	var records []*SOJournalRecord
	var offset int64
	expectedSequence := initialSequence
	for offset < size {
		remaining := size - offset
		if remaining < journalHeaderSize {
			header := make([]byte, remaining)
			if _, err := readAtMost(storage, header, offset); err != nil {
				return nil, 0, errors.Wrap(err, "read journal tail")
			}
			if !validJournalHeaderPrefixForSequence(header, expectedSequence) {
				return nil, 0, errors.Wrap(ErrJournalCorrupt, "impossible journal tail prefix")
			}
			return records, offset, nil
		}
		header := make([]byte, journalHeaderSize)
		if _, err := readAtMost(storage, header, offset); err != nil {
			return nil, 0, errors.Wrap(err, "read journal frame header")
		}
		kind, sequence, payloadLength, err := parseJournalHeader(header)
		if err != nil {
			return nil, 0, err
		}
		if sequence != expectedSequence {
			if len(records) == 0 && initialSequence != 1 {
				return nil, 0, &journalSequenceBaseError{observed: sequence, expected: expectedSequence}
			}
			return nil, 0, errors.Wrap(ErrJournalCorrupt, "journal frame sequence is not contiguous")
		}
		frameLength := int64(journalHeaderSize) + int64(payloadLength) + journalTrailerSize
		if frameLength > remaining {
			if remaining >= int64(journalHeaderSize+journalTrailerSize) {
				trailer := make([]byte, journalTrailerSize)
				if _, readErr := readAtMost(storage, trailer, offset+remaining-journalTrailerSize); readErr != nil {
					return nil, 0, errors.Wrap(readErr, "read journal frame trailer")
				}
				actualPayloadLength := remaining - int64(journalHeaderSize+journalTrailerSize)
				if bytes.Equal(trailer[:4], journalTrailerMagic[:]) && binary.BigEndian.Uint32(trailer[4:]) == uint32(actualPayloadLength) {
					return nil, 0, errors.Wrap(ErrJournalCorrupt, "journal frame header length disagrees with committed trailer")
				}
			}
			return records, offset, nil
		}
		payload := make([]byte, payloadLength)
		if _, err := readAtMost(storage, payload, offset+journalHeaderSize); err != nil {
			return nil, 0, errors.Wrap(err, "read journal frame payload")
		}
		trailer := make([]byte, journalTrailerSize)
		if _, err := readAtMost(storage, trailer, offset+int64(journalHeaderSize)+int64(payloadLength)); err != nil {
			return nil, 0, errors.Wrap(err, "read journal frame trailer")
		}
		if !bytes.Equal(trailer[:4], journalTrailerMagic[:]) || binary.BigEndian.Uint32(trailer[4:]) != payloadLength {
			return nil, 0, errors.Wrap(ErrJournalCorrupt, "journal frame trailer mismatch")
		}
		if err := verifyJournalCRC(header, payload); err != nil {
			return nil, 0, err
		}
		record := new(SOJournalRecord)
		if err := record.UnmarshalVT(payload); err != nil {
			return nil, 0, errors.Wrap(ErrJournalCorrupt, "decode journal payload")
		}
		if record.GetFormatVersion() != JournalFormatVersion || record.GetSequence() != sequence || record.GetKind() != kind {
			return nil, 0, errors.Wrap(ErrJournalCorrupt, "journal payload prefix disagrees with frame")
		}
		if err := validateJournalRecord(record); err != nil {
			return nil, 0, errors.Wrap(ErrJournalCorrupt, err.Error())
		}
		expectedSequence++
		records = append(records, record)
		offset += frameLength
	}
	return records, offset, nil
}

func parseJournalHeader(header []byte) (SOJournalRecordKind, uint64, uint32, error) {
	if !bytes.Equal(header[:4], journalMagic[:]) {
		return 0, 0, 0, errors.Wrap(ErrJournalCorrupt, "invalid journal frame magic")
	}
	if binary.BigEndian.Uint16(header[4:6]) != journalFrameVersion {
		return 0, 0, 0, errors.Wrap(ErrJournalCorrupt, "unsupported journal frame version")
	}
	kind := SOJournalRecordKind(binary.BigEndian.Uint16(header[6:8]))
	if !validJournalRecordKind(kind) {
		return 0, 0, 0, errors.Wrap(ErrJournalCorrupt, "invalid journal frame kind")
	}
	payloadLength := binary.BigEndian.Uint32(header[16:20])
	if crc32.Checksum(header[:20], crc32.MakeTable(crc32.Castagnoli)) != binary.BigEndian.Uint32(header[20:24]) {
		return 0, 0, 0, errors.Wrap(ErrJournalCorrupt, "journal frame header checksum mismatch")
	}
	if payloadLength > journalMaxPayload {
		return 0, 0, 0, errors.Wrap(ErrJournalCorrupt, "journal frame exceeds bounded payload")
	}
	return kind, binary.BigEndian.Uint64(header[8:16]), payloadLength, nil
}

func verifyJournalCRC(header, payload []byte) error {
	crc := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = crc.Write(header[:24])
	_, _ = crc.Write(payload)
	if crc.Sum32() != binary.BigEndian.Uint32(header[24:28]) {
		return errors.Wrap(ErrJournalCorrupt, "journal frame checksum mismatch")
	}
	return nil
}

func validJournalHeaderPrefixForSequence(header []byte, expectedSequence uint64) bool {
	if len(header) == 0 || len(header) >= journalHeaderSize || !bytes.Equal(header[:min(len(header), 4)], journalMagic[:min(len(header), 4)]) {
		return false
	}
	if len(header) >= 5 && header[4] != byte(journalFrameVersion>>8) {
		return false
	}
	if len(header) >= 6 && binary.BigEndian.Uint16(header[4:6]) != journalFrameVersion {
		return false
	}
	if len(header) >= 7 {
		high := header[6]
		possible := false
		for kind := SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT; kind <= SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED; kind++ {
			if byte(uint16(kind)>>8) == high {
				possible = true
				break
			}
		}
		if !possible {
			return false
		}
	}
	if len(header) >= 8 && !validJournalRecordKind(SOJournalRecordKind(binary.BigEndian.Uint16(header[6:8]))) {
		return false
	}
	if expectedSequence != 0 && len(header) > 8 {
		var expected [8]byte
		binary.BigEndian.PutUint64(expected[:], expectedSequence)
		for index := 8; index < len(header) && index < 16; index++ {
			if header[index] != expected[index-8] {
				return false
			}
		}
	}
	if len(header) >= 17 {
		observed := min(len(header)-16, 4)
		var minimum uint32
		for index := range observed {
			minimum = (minimum << 8) | uint32(header[16+index])
		}
		for index := observed; index < 4; index++ {
			minimum <<= 8
		}
		if minimum > journalMaxPayload {
			return false
		}
	}
	if len(header) >= 20 && binary.BigEndian.Uint32(header[16:20]) > journalMaxPayload {
		return false
	}
	if len(header) >= 21 {
		expectedCRC := crc32.Checksum(header[:20], crc32.MakeTable(crc32.Castagnoli))
		observed := len(header) - 20
		for index := 0; index < observed && index < 4; index++ {
			shift := uint(8 * (3 - index))
			if header[20+index] != byte(expectedCRC>>shift) {
				return false
			}
		}
	}
	return true
}

func validJournalRecordKind(kind SOJournalRecordKind) bool {
	return kind >= SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_INTENT && kind <= SOJournalRecordKind_SO_JOURNAL_RECORD_KIND_RESEND_AUTHORIZED
}

func readAtMost(storage JournalStorage, buf []byte, offset int64) (int, error) {
	n, err := storage.ReadAt(buf, offset)
	if err != nil && (!errors.Is(err, io.EOF) || n != len(buf)) {
		return n, err
	}
	if n != len(buf) {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}
