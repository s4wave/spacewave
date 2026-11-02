import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuTerminal } from '@s4wave/web/images/icons.js'

import { CodeBlock, CodeBlockContent } from '@s4wave/web/ui/code-block.js'
import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceIcon,
  TraceLine,
  TraceTitle,
  type TraceStatus,
} from '@s4wave/web/ui/trace.js'
import { statusTitle } from '@s4wave/web/ui/trace-status.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: { maxHeight: `calc(${themeLayout['--spacing']} * 64)` },
})

/** Tool renders a tool call as a trace line, opening once it settles with text. */
function Tool({
  name,
  status,
  input,
  output,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    name: string
    status: TraceStatus
    input?: string
    output?: string
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const expandable =
    (status === 'done' || status === 'error') && Boolean(input || output)
  const line = (
    <TraceLine status={status}>
      <TraceIcon>
        <LuTerminal />
      </TraceIcon>
      <TraceTitle>{statusTitle(status, name)}</TraceTitle>
    </TraceLine>
  )

  return (
    <div data-slot="tool" {...stylexProps(sx, props)}>
      <Trace>
        {expandable ? (
          <TraceCollapsible>
            <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
            <TraceCollapsibleContent sx={stylexStyles.rule1}>
              {input ? (
                <CodeBlock size="sm">
                  <CodeBlockContent wrap sx={stylexStyles.rule2}>
                    {input}
                  </CodeBlockContent>
                </CodeBlock>
              ) : null}
              {output ? (
                <CodeBlock
                  size="sm"
                  variant={status === 'error' ? 'destructive' : 'default'}
                >
                  <CodeBlockContent wrap sx={stylexStyles.rule2}>
                    {output}
                  </CodeBlockContent>
                </CodeBlock>
              ) : null}
            </TraceCollapsibleContent>
          </TraceCollapsible>
        ) : (
          line
        )}
      </Trace>
    </div>
  )
}

export { Tool }
