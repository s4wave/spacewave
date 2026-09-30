//go:build !js

package spacewave_cli

import (
	"bytes"
	"cmp"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/bbolt"
	bbolt_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/aperturerobotics/cli"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

// volumeUsageOpenTimeout bounds the wait for the volume file lock. A running
// daemon holds the lock for its whole lifetime, so waiting longer never helps.
const volumeUsageOpenTimeout = time.Second

// volumeKeyClass accumulates the keys that share one grouping prefix.
type volumeKeyClass struct {
	// bucket is the bbolt bucket holding the keys.
	bucket string
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
		Usage:     "break down a volume file's keys and bytes by key prefix",
		ArgsUsage: "<volume-file>",
		Description: "Reads a bbolt volume file read-only. A running daemon locks its volume, " +
			"so scan a copy instead (cp -c clones it on APFS).",
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
			return runDebugVolumeUsage(c.Args().First(), []byte(prefix), []byte(separator), depth, width, top, c.String("output"))
		},
	}
}

// runDebugVolumeUsage scans a volume file and prints its key classes by size.
func runDebugVolumeUsage(
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

	// Open read-only and fail fast when a daemon holds the lock.
	db, err := bbolt.Open(path, 0o400, &bbolt.Options{ReadOnly: true, Timeout: volumeUsageOpenTimeout})
	if errors.Is(err, bbolt_errors.ErrTimeout) {
		return errors.Errorf("%s is locked by a running daemon; scan a copy instead", path)
	}
	if err != nil {
		return errors.Wrap(err, "open volume")
	}
	defer db.Close()

	// Group every key under the prefix by its class.
	classes := make(map[string]*volumeKeyClass)
	var total volumeKeyClass
	err = db.View(func(tx *bbolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bbolt.Bucket) error {
			cur := b.Cursor()
			for k, v := cur.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = cur.Next() {
				class := volumeKeyClassOf(k[len(prefix):], separator, depth, width)
				id := string(name) + "\x00" + class
				kc := classes[id]
				if kc == nil {
					kc = &volumeKeyClass{bucket: string(name), class: string(prefix) + class}
					classes[id] = kc
				}
				kc.keys++
				kc.keyBytes += uint64(len(k))
				kc.valueBytes += uint64(len(v))
			}
			return nil
		})
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
		ms.WriteObjectField("classes")
		ms.WriteArrayStart()
		var moreClass bool
		for _, kc := range ranked {
			ms.WriteMoreIf(&moreClass)
			ms.WriteObjectStart()
			var moreField bool
			writeJSONStringField(ms, &moreField, "bucket", kc.bucket)
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
	})
	os.Stdout.WriteString("\n")
	rows := [][]string{{"BUCKET", "CLASS", "KEYS", "KEY BYTES", "VALUE BYTES"}}
	for _, kc := range ranked {
		rows = append(rows, []string{
			kc.bucket,
			kc.class,
			strconv.FormatUint(kc.keys, 10),
			humanize.IBytes(kc.keyBytes),
			humanize.IBytes(kc.valueBytes),
		})
	}
	writeTable(os.Stdout, "", rows)
	return nil
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
