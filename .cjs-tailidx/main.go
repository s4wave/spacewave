// tailidx: read-only. For each pack file named on stdin, read only its index
// tail (packfile.ReadIndex over an io.ReaderAt) and print "id\tkeysuffix16hex".
package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/s4wave/spacewave/db/packfile"
)

func main() {
	// Stream the requested pack indexes to stdout.
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		// Skip empty input lines before opening a pack.
		p := strings.TrimSpace(in.Text())
		if p == "" {
			continue
		}

		// Open the input pack and determine its readable extent.
		f, err := os.Open(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", p, err)
			continue
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			continue
		}

		// Require a nonnegative extent before converting it to the index size.
		size := st.Size()
		if size < 0 {
			f.Close()
			continue
		}

		// Decode the tail and close the pack before emitting its keys.
		ents, err := packfile.ReadIndex(f, uint64(size))
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "index %s: %v\n", p, err)
			continue
		}

		// Emit the compact key suffixes under their pack identifier.
		id := filepath.Base(p)
		for _, e := range ents {
			k := e.GetKey()
			if len(k) > 16 {
				k = k[len(k)-16:]
			}
			fmt.Fprintf(out, "%s\t%s\n", id, hex.EncodeToString(k))
		}
	}
}
