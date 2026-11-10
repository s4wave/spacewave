//go:build !js && !wasip1

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// The files beside the database: the seed's progress, the end of each
// acknowledged commit's results, and each commit's latency. Each line is
// written once the commit it records returned, so a kill leaves at most a
// commit that landed without its line, never a line without its commit.
func seededPath(path string) string { return path + ".seeded" }
func ackedPath(path string) string  { return path + ".acked" }
func latencyPath(path string) string {
	return path + ".latency"
}

// appendLine appends line and a newline to the file at path.
func appendLine(path, line string) error {
	// Write the line and report the close's error too.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(line + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// lastInt returns the first field of the last line of the file at path as an
// integer, or zero when the file is absent or empty.
func lastInt(path string) (int, error) {
	// Open the file; an absent one has no lines.
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Keep the last line that is not blank and parse its first field.
	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			last = line
		}
	}
	if err := sc.Err(); err != nil || last == "" {
		return 0, err
	}
	n, err := strconv.Atoi(strings.Fields(last)[0])
	return n, errors.Wrapf(err, "last line of %s", path)
}
