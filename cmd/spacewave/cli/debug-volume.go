//go:build !js

package spacewave_cli

import (
	"bytes"
	"cmp"
	"context"
	"os"
	"slices"
	"strconv"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// volumeKeyClass accumulates the keys that share one grouping prefix.
type volumeKeyClass struct {
	// class is the grouping prefix of the keys.
	class string
	// keys counts the keys in the class.
	keys uint64
	// keyBytes sums the key lengths.
	keyBytes uint64
	// valueBytes sums the value lengths.
	valueBytes uint64
}

// newDebugVolumeUsageCommand builds the offline volume usage breakdown.
func newDebugVolumeUsageCommand() *cli.Command {
	// Bind the grouping flags to the scan arguments.
	var prefix string
	var separator string
	var depth int
	var width int
	var top int
	return &cli.Command{
		Name:      "volume-usage",
		Usage:     "break down a volume file's keys and bytes by key prefix, and its free space",
		ArgsUsage: "<volume-file>",
		Description: "Scans an s4db volume file in one snapshot. It may run beside the " +
			"daemon that has the volume open. The space section shows how the free " +
			"runs are sized, where the live values end, what holds freed space " +
			"back, and how much of the free space the file system still allocates.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "prefix",
				Usage:       "only scan keys with this prefix",
				Destination: &prefix,
			},
			&cli.StringFlag{
				Name:        "separator",
				Usage:       "key segment separator",
				Value:       "/",
				Destination: &separator,
			},
			&cli.IntFlag{
				Name:        "depth",
				Usage:       "number of key segments after the prefix to group by, or 0 for whole keys",
				Value:       1,
				Destination: &depth,
			},
			&cli.IntFlag{
				Name:        "width",
				Usage:       "maximum bytes after the prefix to group by",
				Value:       48,
				Destination: &width,
			},
			&cli.IntFlag{
				Name:        "top",
				Usage:       "number of classes to print, largest first",
				Value:       40,
				Destination: &top,
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return errors.New("expected one volume file argument")
			}
			return runDebugVolumeUsage(c.Context, c.Args().First(), []byte(prefix), []byte(separator), depth, width, top, c.String("output"))
		},
	}
}

// runDebugVolumeUsage scans a volume file and prints its key classes by size.
func runDebugVolumeUsage(
	ctx context.Context,
	path string,
	prefix []byte,
	separator []byte,
	depth, width, top int,
	outputFormat string,
) error {
	// Validate the grouping before a scan that can take minutes.
	if depth < 0 || width < 0 || top < 0 {
		return errors.New("depth, width and top must not be negative")
	}
	if len(separator) == 0 {
		return errors.New("separator must not be empty")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	fileBytes := uint64(fi.Size()) //nolint:gosec // Stat sizes are never negative.

	// Open the file beside any daemon that has it open.
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		return errors.Wrap(err, "open volume")
	}
	defer db.Close()

	// Report how the file's chunks are used before taking the snapshot.
	stats, err := db.Stats(ctx)
	if err != nil {
		return errors.Wrap(err, "read stats")
	}

	// Read every key and value from one snapshot. The snapshot holds back
	// the space a running daemon frees.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Group every key under the prefix by its class.
	classes := make(map[string]*volumeKeyClass)
	var total volumeKeyClass
	err = tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
		// Find or add the key's class.
		class := volumeKeyClassOf(k[len(prefix):], separator, depth, width)
		kc := classes[class]
		if kc == nil {
			kc = &volumeKeyClass{class: string(prefix) + class}
			classes[class] = kc
		}

		// Count the key and its bytes.
		kc.keys++
		kc.keyBytes += uint64(len(k))
		kc.valueBytes += uint64(len(v))
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "scan volume")
	}

	// Rank classes by the bytes they hold.
	ranked := make([]*volumeKeyClass, 0, len(classes))
	for _, kc := range classes {
		ranked = append(ranked, kc)
		total.keys += kc.keys
		total.keyBytes += kc.keyBytes
		total.valueBytes += kc.valueBytes
	}
	slices.SortFunc(ranked, func(a, b *volumeKeyClass) int {
		return cmp.Compare(b.keyBytes+b.valueBytes, a.keyBytes+a.valueBytes)
	})
	if top > 0 && len(ranked) > top {
		ranked = ranked[:top]
	}

	// Emit structured output for scripts.
	if outputFormat == "json" || outputFormat == "yaml" {
		buf, ms := newMarshalBuf()
		ms.WriteObjectStart()
		var more bool
		writeJSONStringField(ms, &more, "path", path)
		writeJSONUint64Field(ms, &more, "fileBytes", fileBytes)
		writeJSONUint64Field(ms, &more, "keys", total.keys)
		writeJSONUint64Field(ms, &more, "keyBytes", total.keyBytes)
		writeJSONUint64Field(ms, &more, "valueBytes", total.valueBytes)
		ms.WriteMoreIf(&more)
		ms.WriteObjectField("stats")
		writeVolumeStatsJSON(ms, &stats)
		ms.WriteMoreIf(&more)
		ms.WriteObjectField("classes")
		ms.WriteArrayStart()
		var moreClass bool
		for _, kc := range ranked {
			ms.WriteMoreIf(&moreClass)
			ms.WriteObjectStart()
			var moreField bool
			writeJSONStringField(ms, &moreField, "class", kc.class)
			writeJSONUint64Field(ms, &moreField, "keys", kc.keys)
			writeJSONUint64Field(ms, &moreField, "keyBytes", kc.keyBytes)
			writeJSONUint64Field(ms, &moreField, "valueBytes", kc.valueBytes)
			ms.WriteObjectEnd()
		}
		ms.WriteArrayEnd()
		ms.WriteObjectEnd()
		return formatOutput(buf.Bytes(), outputFormat)
	}

	// Print totals and the ranked classes for people.
	writeFields(os.Stdout, [][2]string{
		{"Volume", path},
		{"File", humanize.IBytes(fileBytes)},
		{"Keys", strconv.FormatUint(total.keys, 10)},
		{"Key Bytes", humanize.IBytes(total.keyBytes)},
		{"Value Bytes", humanize.IBytes(total.valueBytes)},
		{"Index Keys", strconv.FormatInt(stats.KeyCount, 10)},
		{"Live Values", ibytes(stats.LiveBytes)},
		{"Dead Values", ibytes(stats.DeadBytes)},
		{"Free", ibytes(stats.FreeBytes)},
		{"Pending", ibytes(stats.PendingBytes)},
		{"Memtable", ibytes(stats.MemtableBytes)},
		{"Collect Reserve", ibytes(stats.CollectBytes)},
	})
	os.Stdout.WriteString("\n")
	writeVolumeStatsTables(&stats)
	rows := [][]string{{"CLASS", "KEYS", "KEY BYTES", "VALUE BYTES"}}
	for _, kc := range ranked {
		rows = append(rows, []string{
			kc.class,
			strconv.FormatUint(kc.keys, 10),
			humanize.IBytes(kc.keyBytes),
			humanize.IBytes(kc.valueBytes),
		})
	}
	writeTable(os.Stdout, "", rows)
	return nil
}

// writeVolumeStatsJSON writes the file's chunk use, runs and reader slots as
// one JSON object.
func writeVolumeStatsJSON(ms *protojson.MarshalState, stats *s4db.Stats) {
	// Write the sizes.
	ms.WriteObjectStart()
	var more bool
	for _, f := range []struct {
		name string
		n    int64
	}{
		{"fileBytes", stats.FileBytes}, {"liveBytes", stats.LiveBytes},
		{"deadBytes", stats.DeadBytes}, {"freeBytes", stats.FreeBytes},
		{"pendingBytes", stats.PendingBytes}, {"keyCount", stats.KeyCount},
		{"memtableBytes", stats.MemtableBytes}, {"collectBytes", stats.CollectBytes},
	} {
		writeJSONInt64Field(ms, &more, f.name, f.n)
	}

	// Write the chunk counts by kind.
	ms.WriteMoreIf(&more)
	ms.WriteObjectField("chunks")
	ms.WriteObjectStart()
	var moreChunk bool
	for _, row := range volumeChunkRows(&stats.Chunks) {
		writeJSONInt64Field(ms, &moreChunk, row.name, row.count)
	}
	ms.WriteObjectEnd()

	// Write the runs per level.
	ms.WriteMoreIf(&more)
	ms.WriteObjectField("runs")
	ms.WriteArrayStart()
	for i, n := range stats.Runs {
		if i != 0 {
			ms.WriteMore()
		}
		ms.WriteInt64(int64(n))
	}
	ms.WriteArrayEnd()

	// Write the reader slots.
	ms.WriteMoreIf(&more)
	ms.WriteObjectField("slots")
	ms.WriteArrayStart()
	var moreSlot bool
	for _, sl := range stats.Slots {
		ms.WriteMoreIf(&moreSlot)
		ms.WriteObjectStart()
		var moreField bool
		writeJSONInt64Field(ms, &moreField, "slot", int64(sl.Slot))
		writeJSONUint64Field(ms, &moreField, "seq", sl.Seq)
		writeJSONUint64Field(ms, &moreField, "version", sl.Version)
		writeJSONBoolField(ms, &moreField, "own", sl.Own)
		ms.WriteObjectEnd()
	}
	ms.WriteArrayEnd()
	ms.WriteObjectEnd()
}

// writeVolumeStatsTables prints the chunk counts by kind, the runs per level
// and the reader slots.
func writeVolumeStatsTables(stats *s4db.Stats) {
	// Print the chunks by kind.
	rows := [][]string{{"CHUNK KIND", "CHUNKS"}}
	for _, row := range volumeChunkRows(&stats.Chunks) {
		rows = append(rows, []string{row.name, strconv.FormatInt(row.count, 10)})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\n")

	// Print the runs per level.
	rows = [][]string{{"LEVEL", "RUNS"}}
	for i, n := range stats.Runs {
		rows = append(rows, []string{strconv.Itoa(i), strconv.Itoa(n)})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\n")

	// Print the slots whose pins hold freed space back.
	rows = [][]string{{"SLOT", "SEQ", "VERSION", "HELD"}}
	for _, sl := range stats.Slots {
		held := "yes"
		if sl.Own {
			held = "this process"
		}
		rows = append(rows, []string{
			strconv.Itoa(sl.Slot),
			strconv.FormatUint(sl.Seq, 10),
			strconv.FormatUint(sl.Version, 10),
			held,
		})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\n")
}

// volumeChunkRow is one chunk kind's count.
type volumeChunkRow struct {
	name  string
	count int64
}

// volumeChunkRows lists the chunk counts by kind in file order of use.
func volumeChunkRows(c *s4db.ChunkCounts) []volumeChunkRow {
	return []volumeChunkRow{
		{"log", c.Log}, {"value", c.Value}, {"run", c.Run}, {"manifest", c.Manifest},
		{"reserved", c.Reserved}, {"pending", c.Pending}, {"free", c.Free},
	}
}

// ibytes formats a signed byte count in binary units.
func ibytes(n int64) string {
	if n < 0 {
		return "-" + humanize.IBytes(uint64(-n))
	}
	return humanize.IBytes(uint64(n))
}

// volumeKeyClassOf returns the leading depth segments of key, including their
// separators, capped to width bytes. Keys with non-printable bytes are quoted.
func volumeKeyClassOf(key, separator []byte, depth, width int) string {
	// Cut after the depth-th separator.
	end := 0
	for range depth {
		i := bytes.Index(key[end:], separator)
		if i < 0 {
			end = len(key)
			break
		}
		end += i + len(separator)
	}
	if depth == 0 {
		end = len(key)
	}
	class := key[:min(end, width)]

	// Keep printable classes readable and escape binary ones.
	for _, c := range class {
		if c < 0x20 || c > 0x7e {
			quoted := strconv.QuoteToASCII(string(class))
			return quoted[1 : len(quoted)-1]
		}
	}
	return string(class)
}

// openStoppedVolume opens the volume file at path without creating or
// rewriting its peer key, and refuses it while another process has it open.
// A daemon started meanwhile shares the file, so stop it for the whole run.
// The caller closes the volume.
func openStoppedVolume(ctx context.Context, le *logrus.Entry, path string) (*volume_s4db.Volume, error) {
	// Refuse a missing file, which the open would create.
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:          path,
		NoGenerateKey: true,
		NoWriteKey:    true,
	})
	if err != nil {
		return nil, errors.Wrap(err, "open volume")
	}

	// Refuse a volume a daemon has open.
	shared, err := volume_s4db.GetDB(vol).Shared()
	if err == nil && shared {
		err = errors.Errorf("%s is open in another process; stop the daemon first", path)
	}
	if err != nil {
		vol.Close()
		return nil, err
	}
	return vol, nil
}
