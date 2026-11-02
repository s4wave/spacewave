import * as React from 'react'
import { LuCircleX } from 'react-icons/lu'

import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from '@s4wave/web/ui/trace.js'

/** StackFrame is one frame. An own frame may link to its source. */
export type StackFrame = {
  text: string
  href?: string
}

/** StackFrameLine renders one frame, linking it when it has a source. */
function StackFrameLine({ frame }: { frame: StackFrame }) {
  const line = (
    <TraceLine>
      <TraceTitle>{frame.text}</TraceTitle>
    </TraceLine>
  )
  if (!frame.href) {
    return line
  }

  return <a href={frame.href}>{line}</a>
}

/** frameKey distinguishes repeated frames the way numbered code lines do. */
function frameKey(seen: Map<string, number>, text: string) {
  const count = seen.get(text) ?? 0
  seen.set(text, count + 1)
  return `${count}:${text}`
}

/** StackTrace renders a failed call and its frames. */
function StackTrace({
  error,
  frames,
  className,
  ...props
}: React.ComponentProps<'div'> & {
  error: string
  frames: StackFrame[]
}) {
  const seen = new Map<string, number>()

  return (
    <div data-slot="stack-trace" className={className} {...props}>
      <Trace>
        <TraceCollapsible defaultOpen>
          <TraceCollapsibleTrigger>
            <TraceLine status="error">
              <TraceIcon>
                <LuCircleX />
              </TraceIcon>
              <TraceTitle>Failed: {error}</TraceTitle>
            </TraceLine>
          </TraceCollapsibleTrigger>
          <TraceCollapsibleContent className="flex flex-col gap-2">
            <p className="text-destructive font-mono text-xs">{error}</p>
            <Trace>
              {frames.map((frame) => (
                <StackFrameLine
                  key={frameKey(seen, frame.text)}
                  frame={frame}
                />
              ))}
            </Trace>
          </TraceCollapsibleContent>
        </TraceCollapsible>
      </Trace>
    </div>
  )
}

export { StackTrace }
