//go:build !js

package main

import (
	"math"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const jsMeasure = `
(function(sel) {
  var els = document.querySelectorAll(sel);
  var results = [];
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
    var rect = el.getBoundingClientRect();
    var style = getComputedStyle(el);
    var fontSize = parseFloat(style.fontSize);
    var lineHeight = parseFloat(style.lineHeight);
    if (isNaN(lineHeight)) lineHeight = fontSize * 1.2;
    var lines = Math.max(1, Math.round(rect.height / lineHeight));
    var text = el.textContent || '';
    var totalChars = text.replace(/\s+/g, ' ').trim().length;
    var charsPerLine = lines > 0 ? Math.round(totalChars / lines) : totalChars;
    var label = el.tagName.toLowerCase();
    if (el.id) label += '#' + el.id;
    if (el.className && typeof el.className === 'string') {
      label += '.' + el.className.trim().split(/\s+/).join('.');
    }
    results.push({
      selector: label,
      width: rect.width,
      height: rect.height,
      fontSize: style.fontSize,
      lineHeight: style.lineHeight,
      lines: lines,
      charsPerLine: charsPerLine
    });
  }
  return JSON.stringify(results);
})
`

func buildMeasureCommand() *cli.Command {
	return &cli.Command{
		Name:      "measure",
		Usage:     "dump geometry and typography for elements",
		ArgsUsage: "<selector>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return errors.New("usage: measure <selector>")
			}
			sel := c.Args().First()
			code := jsMeasure + "(" + escapeJSString(sel) + ")"

			type measureEntry struct {
				selector     string
				width        float64
				height       float64
				fontSize     string
				lineHeight   string
				lines        int
				charsPerLine int
			}
			var results []measureEntry
			if err := args.RunEvalJSON(c.Context, code, func(v *fastjson.Value) {
				for _, rv := range v.GetArray() {
					results = append(results, measureEntry{
						selector:     string(rv.GetStringBytes("selector")),
						width:        rv.GetFloat64("width"),
						height:       rv.GetFloat64("height"),
						fontSize:     string(rv.GetStringBytes("fontSize")),
						lineHeight:   string(rv.GetStringBytes("lineHeight")),
						lines:        rv.GetInt("lines"),
						charsPerLine: rv.GetInt("charsPerLine"),
					})
				}
			}); err != nil {
				return err
			}
			if len(results) == 0 {
				return errors.Errorf("no elements matched %q", sel)
			}
			w := os.Stdout
			for _, r := range results {
				width := strconv.FormatFloat(math.Round(r.width*100)/100, 'f', 2, 64)
				height := strconv.FormatFloat(math.Round(r.height*100)/100, 'f', 2, 64)
				w.WriteString("[" + r.selector + "] " + width + "x" + height +
					" font:" + r.fontSize + "/" + r.lineHeight +
					" lines:" + strconv.Itoa(r.lines) +
					" ~chars/line:" + strconv.Itoa(r.charsPerLine) + "\n")
			}
			return nil
		},
	}
}
