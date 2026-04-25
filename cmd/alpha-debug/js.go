//go:build !js

package main

import (
	debug_cli "github.com/s4wave/spacewave/cmd/alpha-debug/cli"
)

// escapeJSString returns s as a safe JavaScript string literal (with quotes).
var escapeJSString = debug_cli.EscapeJSString

// jsDetectLineBreaks is a JavaScript snippet that detects line breaks in text
// nodes using the Range API. It accepts a CSS selector variable `sel` and
// returns JSON: [{selector, width, lines}] where lines is an array of strings.
//
// Usage: wrap in a function that sets `sel` before this code runs.
const jsDetectLineBreaks = `
(function(sel) {
  var els = document.querySelectorAll(sel);
  var results = [];
  for (var i = 0; i < els.length; i++) {
    var el = els[i];
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
          if (currentLine.length > 0) {
            lines.push(currentLine);
          }
          currentLine = '';
        }
        var ch = node.textContent[ci];
        if (ch !== '\n' && ch !== '\r') {
          currentLine += ch;
        }
        lastBottom = charRect.bottom;
      }
    }
    if (currentLine.length > 0) {
      lines.push(currentLine);
    }
    // Trim trailing whitespace from each line.
    for (var li = 0; li < lines.length; li++) {
      lines[li] = lines[li].replace(/\s+$/, '');
    }
    // Build a short selector label.
    var label = el.tagName.toLowerCase();
    if (el.id) label += '#' + el.id;
    if (el.className && typeof el.className === 'string') {
      label += '.' + el.className.trim().split(/\s+/).join('.');
    }
    results.push({
      selector: label,
      width: Math.round(rect.width),
      lines: lines
    });
  }
  return JSON.stringify(results);
})
`
