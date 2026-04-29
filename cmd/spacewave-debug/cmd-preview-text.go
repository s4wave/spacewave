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

const jsPreviewText = `
(function(sel, text) {
  var el = document.querySelector(sel);
  if (!el) return JSON.stringify({error: 'no element matched'});
  var original = el.textContent;
  el.textContent = text;

  // Wait for layout via double requestAnimationFrame.
  return new Promise(function(resolve) {
    requestAnimationFrame(function() {
      requestAnimationFrame(function() {
        var rect = el.getBoundingClientRect();
        var walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, null);
        var range = document.createRange();
        var lines = [];
        var currentLine = '';
        var lastBottom = null;
        var node;
        while ((node = walker.nextNode())) {
          for (var ci = 0; ci < node.textContent.length; ci++) {
            range.setStart(node, ci);
            range.setEnd(node, ci + 1);
            var charRect = range.getBoundingClientRect();
            if (lastBottom !== null && charRect.bottom > lastBottom + 1) {
              if (currentLine.length > 0) lines.push(currentLine);
              currentLine = '';
            }
            var ch = node.textContent[ci];
            if (ch !== '\n' && ch !== '\r') currentLine += ch;
            lastBottom = charRect.bottom;
          }
        }
        if (currentLine.length > 0) lines.push(currentLine);
        for (var li = 0; li < lines.length; li++) {
          lines[li] = lines[li].replace(/\s+$/, '');
        }

        var label = el.tagName.toLowerCase();
        if (el.id) label += '#' + el.id;
        if (el.className && typeof el.className === 'string') {
          label += '.' + el.className.trim().split(/\s+/).join('.');
        }

        el.textContent = original;
        resolve(JSON.stringify({
          selector: label,
          width: Math.round(rect.width),
          lines: lines
        }));
      });
    });
  });
})
`

func buildPreviewTextCommand() *cli.Command {
	return &cli.Command{
		Name:      "preview-text",
		Usage:     "preview how text would render in an element",
		ArgsUsage: "<selector> <text>",
		Action: func(c *cli.Context) error {
			if c.NArg() < 2 {
				return errors.New("usage: preview-text <selector> <text>")
			}
			sel := c.Args().Get(0)
			text := c.Args().Get(1)
			code := jsPreviewText + "(" + escapeJSString(sel) + ", " + escapeJSString(text) + ")"

			var lines []string
			var evalErr string
			if err := args.RunEvalJSON(c.Context, code, func(v *fastjson.Value) {
				evalErr = string(v.GetStringBytes("error"))
				for _, lv := range v.GetArray("lines") {
					lines = append(lines, string(lv.GetStringBytes()))
				}
			}); err != nil {
				return err
			}
			if evalErr != "" {
				return errors.New(evalErr)
			}
			w := os.Stdout
			w.WriteString("Preview (" + strconv.Itoa(len(lines)) + " lines):\n")
			for i, line := range lines {
				w.WriteString("  " + strconv.Itoa(i+1) + ": " + line + "\n")
			}
			if len(lines) >= 2 {
				last := lines[len(lines)-1]
				chars := len(strings.TrimSpace(last))
				words := len(strings.Fields(last))
				if chars < 15 || words <= 1 {
					w.WriteString("ORPHAN: L" + strconv.Itoa(len(lines)) +
						" is " + strconv.Itoa(words) + " word" + plural(words) +
						" (" + strconv.Itoa(chars) + " chars)\n")
				}
			}
			return nil
		},
	}
}
