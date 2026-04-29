---
name: debug-ui-layout
description: Optimize text copy to fit rendered UI layout using spacewave-debug eval to measure exact line breaks and eliminate orphans.
---

# Debug UI Layout Optimization

Use the spacewave-debug eval bridge to iteratively measure how text copy renders at actual viewport widths, then adjust wording to produce clean line breaks without orphans.

## Process

### 1. Identify the section

Find the section by its DOM id or heading text:

```bash
go run ./cmd/spacewave-debug/ eval "document.getElementById('section-id')?.querySelector('h2')?.textContent"
```

### 2. Measure overall dimensions

Write an eval script to `.tmp/eval-{section}.js` that collects width, height, line count, and line-height for every text element in the section:

```js
var section = document.getElementById("section-id")
if (!section) return "no section"
var ps = section.querySelectorAll("p")
var res = []
for (var i = 0; i < ps.length; i++) {
  var p = ps[i]
  var rect = p.getBoundingClientRect()
  var style = getComputedStyle(p)
  var lines = Math.round(rect.height / parseFloat(style.lineHeight))
  res.push("p" + i + " w:" + Math.round(rect.width) + " h:" + Math.round(rect.height) + " lines:" + lines + " lh:" + style.lineHeight + " | " + p.textContent.substring(0, 90))
}
return res.join("\n")
```

Run with:

```bash
go run ./cmd/spacewave-debug/ eval --file .tmp/eval-{section}.js
```

### 3. Measure exact line breaks

Use the DOM Range API to determine where each line wraps at the rendered width:

```js
var section = document.getElementById("section-id")
if (!section) return "no section"
var cards = section.querySelectorAll("p")
var res = []
for (var idx = 0; idx < cards.length; idx++) {
  var p = cards[idx]
  var text = p.textContent
  var range = document.createRange()
  var node = p.firstChild
  if (!node) continue
  var lines = []
  var lastTop = -1
  var lineStart = 0
  for (var i = 0; i < text.length; i++) {
    range.setStart(node, i)
    range.setEnd(node, i + 1)
    var r = range.getBoundingClientRect()
    if (lastTop !== -1 && r.top > lastTop + 2) {
      lines.push(text.substring(lineStart, i))
      lineStart = i
    }
    lastTop = r.top
  }
  lines.push(text.substring(lineStart))
  res.push("Card " + idx + " (" + lines.length + " lines):")
  for (var j = 0; j < lines.length; j++) {
    res.push("  L" + (j + 1) + ": [" + lines[j] + "]")
  }
}
return res.join("\n")
```

### 4. Identify problems

Common layout issues to fix:

- **Orphans**: Single word (or very short fragment) on the last line
- **Uneven line counts**: Cards in the same grid row should have matching line counts
- **Widows**: First line of a paragraph has only 1-2 words
- **Bad breaks**: Sentence breaks that split meaning awkwardly (e.g., "full-" / "stack")

### 5. Edit copy and re-verify

Adjust wording to fix issues. Strategies:

- **Shorten** to eliminate an extra line (remove filler words, use shorter synonyms)
- **Lengthen** to fill a short last line (add a word or two to push content onto L3)
- **Restructure** to shift where breaks fall (reorder clauses, swap em-dash for comma)
- **Synonym swap** to change character count at break points ("devices" -> "hardware", "everywhere" -> "across everything")

After each edit:

1. Run `bun fecheck` to bump rev and typecheck
2. Re-run the eval script to verify the new line breaks
3. Repeat until all elements have clean breaks

### 6. Quality checklist

- All cards in the same grid row have equal line counts
- No single-word orphans on final lines (aim for 2+ words, 10+ chars)
- Title and subtitle fit on single lines
- Copy still reads naturally (don't sacrifice meaning for layout)
- Verify with `bun fecheck` that types still pass

## Tips

- Card width depends on container queries (`@lg`, `@2xl`) and viewport — always measure, never guess
- At ~226px card width, each line holds ~42-44 characters
- At ~289px card width, each line holds ~52-55 characters
- The `--file` flag avoids shell quoting issues with complex eval scripts
- Single expressions auto-return; multi-statement scripts need explicit `return`
- Keep eval scripts in `.tmp/` (gitignored) for reuse during the session
