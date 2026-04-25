//go:build !js

package main

import (
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// linebreaksEntry is a single element's line-break data from the JS snippet.
type linebreaksEntry struct {
	selector string
	width    int
	lines    []string
}

// parseLinebreaksEntries parses a fastjson array of linebreaks results.
func parseLinebreaksEntries(v *fastjson.Value) []linebreaksEntry {
	var entries []linebreaksEntry
	for _, rv := range v.GetArray() {
		var lines []string
		for _, lv := range rv.GetArray("lines") {
			lines = append(lines, string(lv.GetStringBytes()))
		}
		entries = append(entries, linebreaksEntry{
			selector: string(rv.GetStringBytes("selector")),
			width:    rv.GetInt("width"),
			lines:    lines,
		})
	}
	return entries
}

func buildLinebreaksCommand() *cli.Command {
	return &cli.Command{
		Name:      "linebreaks",
		Usage:     "show exact visual line breaks for text elements",
		ArgsUsage: "<selector>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return errors.New("usage: linebreaks <selector>")
			}
			sel := c.Args().First()
			code := jsDetectLineBreaks + "(" + escapeJSString(sel) + ")"

			var results []linebreaksEntry
			if err := args.RunEvalJSON(c.Context, code, func(v *fastjson.Value) {
				results = parseLinebreaksEntries(v)
			}); err != nil {
				return err
			}
			if len(results) == 0 {
				return errors.Errorf("no elements matched %q", sel)
			}
			w := os.Stdout
			for _, r := range results {
				w.WriteString("[" + r.selector + "] (" + strconv.Itoa(len(r.lines)) + " lines, w:" + strconv.Itoa(r.width) + ")\n")
				for i, line := range r.lines {
					w.WriteString("  " + strconv.Itoa(i+1) + ": " + line + "\n")
				}
			}
			return nil
		},
	}
}
