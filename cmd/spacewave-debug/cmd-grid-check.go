//go:build !js

package main

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const jsGridCheck = `
(function(sel) {
  var el = document.querySelector(sel);
  if (!el) return JSON.stringify({error: 'no element matched'});
  var children = el.children;
  var buckets = {};
  var order = [];
  for (var i = 0; i < children.length; i++) {
    var c = children[i];
    var rect = c.getBoundingClientRect();
    var y = Math.round(rect.top);
    var label = c.tagName.toLowerCase();
    if (c.id) label += '#' + c.id;
    if (c.className && typeof c.className === 'string') {
      var cls = c.className.trim();
      if (cls) label += '.' + cls.split(/\s+/)[0];
    }
    var key = String(y);
    if (!buckets[key]) {
      buckets[key] = {labels: [], heights: [], y: y};
      order.push(key);
    }
    buckets[key].labels.push(label);
    buckets[key].heights.push(Math.round(rect.height * 100) / 100);
  }
  var rows = [];
  for (var ri = 0; ri < order.length; ri++) {
    rows.push(buckets[order[ri]]);
  }
  return JSON.stringify(rows);
})
`

func buildGridCheckCommand() *cli.Command {
	return &cli.Command{
		Name:      "grid-check",
		Usage:     "check height consistency of grid children by row",
		ArgsUsage: "<selector>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return errors.New("usage: grid-check <selector>")
			}
			sel := c.Args().First()
			code := jsGridCheck + "(" + escapeJSString(sel) + ")"

			type gridRow struct {
				labels  []string
				heights []float64
			}
			var rows []gridRow
			if err := args.RunEvalJSON(c.Context, code, func(v *fastjson.Value) {
				for _, rv := range v.GetArray() {
					var labels []string
					for _, lv := range rv.GetArray("labels") {
						labels = append(labels, string(lv.GetStringBytes()))
					}
					var heights []float64
					for _, hv := range rv.GetArray("heights") {
						heights = append(heights, hv.GetFloat64())
					}
					rows = append(rows, gridRow{labels: labels, heights: heights})
				}
			}); err != nil {
				return err
			}
			w := os.Stdout
			if len(rows) == 0 {
				w.WriteString("no children found\n")
				return nil
			}
			mismatches := 0
			for i, row := range rows {
				labels := "[" + strings.Join(row.labels, ", ") + "]"
				heights := formatHeights(row.heights)
				min, max := minMax(row.heights)
				delta := max - min
				status := "OK"
				if delta > 1 {
					status = "MISMATCH (" + strconv.FormatFloat(delta, 'f', 0, 64) + "px delta)"
					mismatches++
				}
				w.WriteString("Row " + strconv.Itoa(i) + ": " + labels + " heights=" + heights + " " + status + "\n")
			}
			if mismatches > 0 {
				w.WriteString(strconv.Itoa(mismatches) + " row" + plural(mismatches) + " with mismatched heights\n")
			}
			return nil
		},
	}
}

func formatHeights(h []float64) string {
	parts := make([]string, len(h))
	for i, v := range h {
		parts[i] = strconv.FormatFloat(v, 'f', 0, 64)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func minMax(vals []float64) (float64, float64) {
	mn := math.Inf(1)
	mx := math.Inf(-1)
	for _, v := range vals {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}
