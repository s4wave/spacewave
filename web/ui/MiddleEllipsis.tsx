import type { CSSProperties } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

// MID_HEAD is the least of its head a shortened value shows, before the
// ellipsis.
const MID_HEAD = 4

// MID_TAIL is how much of a value without a path separator is its tail.
const MID_TAIL = 6

// MiddleEllipsisProps configures a MiddleEllipsis.
export interface MiddleEllipsisProps {
  // value is the whole identifier, path or URL.
  value: string
  // className is appended to the root span's class list.
  className?: string
}

// HIDDEN_SCHEME is the one scheme a MiddleEllipsis does not draw, so a web
// URL's head is its host. Every other scheme, http:// included, stays drawn,
// so an insecure or non-web remote never draws the same as a secure one.
const HIDDEN_SCHEME = 'https://'

// hiddenScheme returns the part of value's start a MiddleEllipsis keeps in
// its text but does not draw: "https://" or "".
export function hiddenScheme(value: string): string {
  return value.startsWith(HIDDEN_SCHEME) ? HIDDEN_SCHEME : ''
}

// splitTail returns where value's tail starts: at its last path separator
// before any trailing ones, so a path or URL keeps its last real segment, or
// MID_TAIL characters from its end, so an ID keeps its last six characters.
export function splitTail(value: string): number {
  // Find the last separator before any trailing ones.
  let end = value.length
  while (end > 1 && value[end - 1] === '/') end--
  const slash = value.lastIndexOf('/', end - 1)
  if (slash > 0 && slash < end - 1) return slash
  return Math.max(0, value.length - MID_TAIL)
}

// MiddleEllipsis shows value on one line in a mono face, shortened in its
// middle to fit its own width: "github.com…/spacewave",
// "12D3KooWQx7Hc4…N2mXq9", "src…name.tsx". It never wraps, never cuts the
// end, and always draws an ellipsis where it cuts.
//
// The fit is CSS alone and nothing measures text. The head and tail are two
// spans in a row: the tail keeps its width and the head truncates with an
// ellipsis, so the whole value shows when it fits. The tail gives way only
// once the head is down to its kept part, MID_HEAD characters and the
// ellipsis or the whole head when it is shorter, and then it loses its start:
// it is a right-to-left box, so it clips at its start, and its text sits in
// a left-to-right bdi so the characters keep their order. A head longer than
// its kept part has already drawn the ellipsis beside the tail, so the tail
// clips bare; a shorter head never truncates, so the tail draws its own
// ellipsis at its start, and keeps at least two characters so that ellipsis
// shows even when the head does not fit. The mono face makes 1ch one
// character, so each limit is a fixed width per value.
//
// An https URL draws without its scheme, which stays in the DOM text as
// visually hidden text before the head, so screen readers, text selection
// and find in page get the whole value, as the title and the copy do. The
// data-mid, data-cut, data-mid-head and data-mid-tail attributes
// mark the parts for the viewer clip check; data-cut indexes the drawn value.
export function MiddleEllipsis({ value, className }: MiddleEllipsisProps) {
  // Split the drawn value where its tail starts.
  const scheme = hiddenScheme(value)
  const shown = value.slice(scheme.length)
  const cut = splitTail(shown)
  const head = shown.slice(0, cut)
  const tail = shown.slice(cut)

  // keep is the width the head holds before the tail gives way. A short head
  // keeps all of itself and hands the ellipsis to the tail.
  const short = head.length <= MID_HEAD + 1
  const keep = short ? head.length : MID_HEAD + 1
  const style = {
    '--mid-keep': `${keep}ch`,
    '--mid-tail-min': `${short ? Math.min(tail.length, 2) : 0}ch`,
  } as CSSProperties

  // Lay out the head and tail; CSS alone decides what shows.
  return (
    <span
      data-mid
      className={cn(
        'flex min-w-0 flex-1 overflow-hidden font-mono whitespace-nowrap',
        className,
      )}
      title={value}
      data-cut={cut}
      style={style}
    >
      {scheme && <span className="sr-only">{scheme}</span>}
      <span data-mid-head className="min-w-0 truncate">
        {head}
      </span>
      <span
        data-mid-tail
        dir="rtl"
        className={cn(
          'max-w-[calc(100%-var(--mid-keep))] min-w-[var(--mid-tail-min)] shrink-0 overflow-hidden',
          short && 'text-ellipsis',
        )}
      >
        <bdi dir="ltr">{tail}</bdi>
      </span>
    </span>
  )
}
