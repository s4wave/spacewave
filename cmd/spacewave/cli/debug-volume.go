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

	// Report how the file's pages are used before taking the snapshot.
	usage, err := db.Usage(ctx)
	if err != nil {
		return errors.Wrap(err, "read page usage")
	}
	space, err := db.SpaceReport(ctx)
	if err != nil {
		return errors.Wrap(err, "read free space")
	}

	// Read every key from one snapshot. The snapshot holds back the space
	// a running daemon frees, so the scan reads only the index.
	ktx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer ktx.Discard()
	tx := ktx.(*s4db.Tx)

	// Group every key under the prefix by its class.
	classes := make(map[string]*volumeKeyClass)
	var total volumeKeyClass
	err = tx.ScanPrefixLens(ctx, prefix, func(k []byte, n int) error {
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
		kc.valueBytes += uint64(n) //nolint:gosec // Value lengths are never negative.
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
		ms.WriteObjectField("pages")
		ms.WriteObjectStart()
		var morePage bool
		writeJSONInt64Field(ms, &morePage, "freeBytes", usage.FreeBytes)
		writeJSONInt64Field(ms, &morePage, "valueChunkBytes", usage.ValueChunkBytes)
		writeJSONInt64Field(ms, &morePage, "liveValueBytes", usage.ValueBytes)
		writeJSONInt64Field(ms, &morePage, "indexKeys", usage.IndexKeys)
		writeJSONInt64Field(ms, &morePage, "indexBytes", usage.IndexBytes)
		writeJSONInt64Field(ms, &morePage, "indexChunkBytes", usage.IndexChunkBytes)
		writeJSONInt64Field(ms, &morePage, "logBytes", usage.LogBytes)
		writeJSONInt64Field(ms, &morePage, "pendingBytes", usage.PendingBytes)
		ms.WriteObjectEnd()
		ms.WriteMoreIf(&more)
		ms.WriteObjectField("space")
		writeVolumeSpaceJSON(ms, &space)
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
		{"Disk", volumeDiskBytes(space.DiskBytes)},
		{"Free Chunks", ibytes(usage.FreeBytes)},
		{"Free on Disk", volumeDiskBytes(space.FreeDiskBytes)},
		{"Data End", ibytes(space.ValueEndBytes)},
		{"Value Chunks", ibytes(usage.ValueChunkBytes)},
		{"Live Values", ibytes(usage.ValueBytes)},
		{"Index Keys", strconv.FormatInt(usage.IndexKeys, 10)},
		{"Index Chunks", ibytes(usage.IndexChunkBytes)},
		{"Index Pages", ibytes(usage.IndexBytes)},
		{"Log Chunks", ibytes(usage.LogBytes)},
		{"Pending", ibytes(usage.PendingBytes) + " (" + strconv.Itoa(space.PendingValueRuns) +
			" value runs, " + strconv.Itoa(space.PendingPageRuns) + " page runs)"},
		{"Unpunched", ibytes(space.UnpunchedBytes)},
		{"Reserved", ibytes(space.ReservedBytes)},
	})
	os.Stdout.WriteString("\n")
	writeVolumeSpaceTables(&space)
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

// writeVolumeSpaceJSON writes the free space report as one JSON object.
func writeVolumeSpaceJSON(ms *protojson.MarshalState, space *s4db.SpaceReport) {
	// Write where the space lies.
	ms.WriteObjectStart()
	var more bool
	writeJSONInt64Field(ms, &more, "diskBytes", space.DiskBytes)
	writeJSONInt64Field(ms, &more, "freeDiskBytes", space.FreeDiskBytes)
	writeJSONInt64Field(ms, &more, "valueEndBytes", space.ValueEndBytes)

	// Write what holds freed space back.
	writeJSONInt64Field(ms, &more, "pendingValueRuns", int64(space.PendingValueRuns))
	writeJSONInt64Field(ms, &more, "pendingPageRuns", int64(space.PendingPageRuns))
	writeJSONInt64Field(ms, &more, "unpunchedBytes", space.UnpunchedBytes)
	writeJSONInt64Field(ms, &more, "reservedBytes", space.ReservedBytes)

	// Write the chunk counts.
	ms.WriteMoreIf(&more)
	ms.WriteObjectField("chunks")
	ms.WriteObjectStart()
	var moreChunk bool
	c := space.Chunks
	for _, f := range []struct {
		name string
		n    int64
	}{{"free", c.Free}, {"log", c.Log}, {"index", c.Index}, {"value", c.Value}} {
		writeJSONInt64Field(ms, &moreChunk, f.name, f.n)
	}
	ms.WriteObjectEnd()

	// Write how full the value and index chunks are.
	for _, f := range []struct {
		name string
		fill *[11]int64
	}{{"valueFill", &space.ValueFill}, {"indexFill", &space.IndexFill}} {
		ms.WriteMoreIf(&more)
		ms.WriteObjectField(f.name)
		ms.WriteArrayStart()
		var moreFill bool
		for _, n := range f.fill {
			ms.WriteMoreIf(&moreFill)
			ms.WriteInt64(n)
		}
		ms.WriteArrayEnd()
	}

	// Write the reader slots.
	ms.WriteMoreIf(&more)
	ms.WriteObjectField("slots")
	ms.WriteArrayStart()
	var moreSlot bool
	for _, sl := range space.Slots {
		ms.WriteMoreIf(&moreSlot)
		ms.WriteObjectStart()
		var moreField bool
		writeJSONInt64Field(ms, &moreField, "slot", int64(sl.Slot))
		writeJSONUint64Field(ms, &moreField, "seq", sl.Seq)
		writeJSONUint64Field(ms, &moreField, "ckpt", sl.Ckpt)
		writeJSONBoolField(ms, &moreField, "live", sl.Live)
		writeJSONBoolField(ms, &moreField, "own", sl.Own)
		ms.WriteObjectEnd()
	}
	ms.WriteArrayEnd()
	ms.WriteObjectEnd()
}

// writeVolumeSpaceTables prints the chunks by kind and fill, and the reader
// slots.
func writeVolumeSpaceTables(space *s4db.SpaceReport) {
	// Print the chunks of each kind, then the value and index chunks by the
	// tenth of them in use.
	c := space.Chunks
	writeFields(os.Stdout, [][2]string{
		{"Chunks", strconv.FormatInt(c.Free+c.Log+c.Index+c.Value, 10) + " (" +
			strconv.FormatInt(c.Value, 10) + " value, " + strconv.FormatInt(c.Index, 10) + " index, " +
			strconv.FormatInt(c.Log, 10) + " log, " + strconv.FormatInt(c.Free, 10) + " free)"},
	})
	os.Stdout.WriteString("\n")
	rows := [][]string{{"FILL", "VALUE CHUNKS", "INDEX CHUNKS"}}
	for i := range space.ValueFill {
		fill := strconv.Itoa(10*i) + "% to " + strconv.Itoa(10*i+10) + "%"
		if i == len(space.ValueFill)-1 {
			fill = "full"
		}
		rows = append(rows, []string{
			fill,
			strconv.FormatInt(space.ValueFill[i], 10),
			strconv.FormatInt(space.IndexFill[i], 10),
		})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\n")

	// Print the slots whose pins hold freed space back.
	rows = [][]string{{"SLOT", "SEQ", "CKPT", "HELD"}}
	for _, sl := range space.Slots {
		held := "no"
		if sl.Own {
			held = "this process"
		} else if sl.Live {
			held = "yes"
		}
		rows = append(rows, []string{
			strconv.Itoa(sl.Slot),
			strconv.FormatUint(sl.Seq, 10),
			strconv.FormatUint(sl.Ckpt, 10),
			held,
		})
	}
	writeTable(os.Stdout, "", rows)
	os.Stdout.WriteString("\n")
}

// volumeDiskBytes formats an allocated byte count the file system may not
// report.
func volumeDiskBytes(n int64) string {
	if n < 0 {
		return "unknown"
	}
	return ibytes(n)
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
