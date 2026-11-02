import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuRoute } from '@s4wave/web/images/icons.js'

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
import { statusLabel, stepIcon } from '@s4wave/web/ui/trace-status.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
})

/** PlanStep is one step in the plan. id is the caller's identity for the step. */
export type PlanStep = {
  id: string
  title: string
  status: TraceStatus
}

/** planHeaderStatus is running, else error, else pending, else done. */
function planHeaderStatus(steps: PlanStep[]): TraceStatus {
  if (steps.some((step) => step.status === 'running')) {
    return 'running'
  }
  if (steps.some((step) => step.status === 'error')) {
    return 'error'
  }
  if (steps.some((step) => step.status === 'pending')) {
    return 'pending'
  }
  return 'done'
}

/** Plan renders the steps the agent decided on, and how many are done. */
function Plan({
  title,
  description,
  steps,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    title: string
    description?: string
    steps: PlanStep[]
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const done = steps.filter((step) => step.status === 'done').length

  return (
    <div data-slot="plan" {...stylexProps(sx, props)}>
      <Trace>
        <TraceCollapsible defaultOpen>
          <TraceCollapsibleTrigger>
            <TraceLine status={planHeaderStatus(steps)}>
              <TraceIcon>
                <LuRoute />
              </TraceIcon>
              <TraceTitle>{title}</TraceTitle>
              <TraceDescription>
                {done} of {steps.length} done
              </TraceDescription>
            </TraceLine>
          </TraceCollapsibleTrigger>
          <TraceCollapsibleContent sx={stylexStyles.rule1}>
            {description ? <p>{description}</p> : null}
            <Trace>
              {steps.map((step) => (
                <TraceLine key={step.id} status={step.status}>
                  <TraceIcon>{stepIcon(step.status)}</TraceIcon>
                  <TraceTitle>{step.title}</TraceTitle>
                  <span sx={stylexStyles.rule2}>
                    {statusLabel(step.status)}
                  </span>
                </TraceLine>
              ))}
            </Trace>
          </TraceCollapsibleContent>
        </TraceCollapsible>
      </Trace>
    </div>
  )
}

export { Plan }
