//go:build !js

package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

func buildOrphansCommand() *cli.Command {
	var all bool
	return &cli.Command{
		Name:      "orphans",
		Usage:     "detect typographic orphans (short last lines)",
		ArgsUsage: "<selector>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "show all elements, not just orphans",
				Destination: &all,
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return errors.New("usage: orphans <selector>")
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
			count := 0
			for _, r := range results {
				if len(r.lines) < 2 {
					if all {
						w.WriteString("OK     [" + r.selector + "] (single line)\n")
					}
					continue
				}
				last := r.lines[len(r.lines)-1]
				chars := len(strings.TrimSpace(last))
				words := len(strings.Fields(last))
				orphan := chars < 15 || words <= 1
				lineNum := strconv.Itoa(len(r.lines))
				charStr := strconv.Itoa(chars)
				wordStr := strconv.Itoa(words)
				if orphan {
					w.WriteString("ORPHAN [" + r.selector + "] L" + lineNum + ": " + strconv.Quote(last) + " (" + charStr + " chars, " + wordStr + " word" + plural(words) + ")\n")
					count++
				} else if all {
					w.WriteString("OK     [" + r.selector + "] L" + lineNum + ": " + strconv.Quote(last) + " (" + charStr + " chars, " + wordStr + " word" + plural(words) + ")\n")
				}
			}
			if count == 0 {
				w.WriteString("no orphans found\n")
			} else {
				w.WriteString(strconv.Itoa(count) + " orphan" + plural(count) + " found\n")
			}
			return nil
		},
	}
}

// plural returns "s" when n != 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
