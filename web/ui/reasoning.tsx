import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuCircleCheck, LuLoaderCircle } from 'react-icons/lu'

import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceDescription,
  TraceIcon,
  TraceLine,
  TraceTitle,
  type TraceStatus,
} from '@s4wave/web/ui/trace.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
    animationName: 'spin',
    animationDuration: '1s',
    animationTimingFunction: 'linear',
    animationIterationCount: 'infinite',
    animationDirection: 'normal',
    animationPlayState: 'running',
    animationDelay: '0s',
    animationFillMode: 'none',
    animationTimeline: 'auto',
  },
  rule2: { whiteSpace: 'pre-wrap' },
})

/** Reasoning renders thinking as a trace line that opens while it is running. */
function Reasoning({
  text,
  status = 'done',
  seconds,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    text: string
    /** status is running while the model is still writing. */
    status?: TraceStatus
    /** seconds is the settled duration, shown beside the title. */
    seconds?: number
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const [chosen, setChosen] = React.useState<boolean>()
  const thinking = status === 'running'
  const line = (
    <TraceLine status={status}>
      <TraceIcon>
        {thinking ? (
          <LuLoaderCircle {...stylex.props(stylexStyles.rule1)} />
        ) : (
          <LuCircleCheck />
        )}
      </TraceIcon>
      <TraceTitle>{thinking ? 'Thinking' : 'Thought'}</TraceTitle>
      {seconds !== undefined ? (
        <TraceDescription>{seconds}s</TraceDescription>
      ) : null}
    </TraceLine>
  )

  return (
    <div data-slot="reasoning" sx={sx} {...props}>
      <Trace>
        {text.length === 0 ? (
          line
        ) : (
          <TraceCollapsible open={chosen ?? thinking} onOpenChange={setChosen}>
            <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
            <TraceCollapsibleContent {...stylex.props(stylexStyles.rule2)}>
              {text}
            </TraceCollapsibleContent>
          </TraceCollapsible>
        )}
      </Trace>
    </div>
  )
}

export { Reasoning }
