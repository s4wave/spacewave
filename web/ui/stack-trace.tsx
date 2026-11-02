import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuCircleX } from '@s4wave/web/images/icons.js'

import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from '@s4wave/web/ui/trace.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

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
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    error: string
    frames: StackFrame[]
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const seen = new Map<string, number>()

  return (
    <div data-slot="stack-trace" {...stylexProps(sx, props)}>
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
          <TraceCollapsibleContent {...stylex.props(stylexStyles.rule1)}>
            <p sx={stylexStyles.rule2}>{error}</p>
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
