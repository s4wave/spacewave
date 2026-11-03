import { keyframeNames } from '../style/tokens.stylex.js'
import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import {
  LuBot,
  LuCircle,
  LuCircleCheck,
  LuCircleX,
  LuLoaderCircle,
} from '@s4wave/web/images/icons.js'

import { Badge } from '@s4wave/web/ui/badge.js'
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
import { statusLabel, statusTitle } from '@s4wave/web/ui/trace-status.js'

import * as stylex from '@stylexjs/stylex'
import {
  fonts as themeFonts,
  colors as themeColors,
  layout as themeLayout,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    animationDelay: '0s',
    animationDirection: 'normal',
    animationDuration: '1s',
    animationFillMode: 'none',
    animationIterationCount: 'infinite',
    animationName: keyframeNames['spin'],
    animationPlayState: 'running',
    animationTimeline: 'auto',
    animationTimingFunction: 'linear',
  },
  rule2: {
    color: {
      default: themeColors['--color-muted-foreground'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    flexShrink: 0,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
    textUnderlineOffset: '4px',
  },
  rule3: {
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    display: 'flex',
    flexWrap: 'wrap',
  },
  rule4: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule5: {
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
  },
  rule6: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule7: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'flex',
    flexDirection: 'column',
  },
})

/** SubagentStep is one step the child session took. id is its identity. */
export type SubagentStep = {
  id: string
  title: string
  status: TraceStatus
}

/** subagentIcon picks the step icon for a child session status. */
function subagentIcon(status: TraceStatus) {
  if (status === 'running') {
    return <LuLoaderCircle sx={stylexStyles.rule1} />
  }
  if (status === 'error') {
    return <LuCircleX />
  }
  if (status === 'pending') {
    return <LuCircle />
  }
  return <LuCircleCheck />
}

/** subagentHasBody reports whether the run has anything to open. */
function subagentHasBody({
  task,
  model,
  tools,
  steps,
  result,
}: {
  task?: string
  model?: string
  tools?: string[]
  steps?: SubagentStep[]
  result?: string
}) {
  return Boolean(task || model || tools?.length || steps?.length || result)
}

/** SubagentLine is the child session's title line. */
function SubagentLine({
  name,
  status,
  seconds,
}: {
  name: string
  status: TraceStatus
  seconds?: number
}) {
  return (
    <TraceLine status={status}>
      <TraceIcon>
        <LuBot />
      </TraceIcon>
      <TraceTitle>{statusTitle(status, name)}</TraceTitle>
      {seconds !== undefined ? (
        <TraceDescription>{seconds}s</TraceDescription>
      ) : null}
    </TraceLine>
  )
}

/** SubagentLink is the viewer link, kept outside the trigger. */
function SubagentLink({ href, name }: { href?: string; name: string }) {
  if (!href) {
    return null
  }

  return (
    <a
      href={href}
      aria-label={`Open ${name} session`}
      data-slot="subagent-link"
      sx={stylexStyles.rule2}
    >
      Open session
    </a>
  )
}

/** SubagentBadges shows the model and tools. */
function SubagentBadges({
  model,
  tools = [],
}: {
  model?: string
  tools?: string[]
}) {
  if (!model && tools.length === 0) {
    return null
  }

  return (
    <div data-slot="subagent-badges" sx={stylexStyles.rule3}>
      {model ? <Badge variant="secondary">{model}</Badge> : null}
      {tools.map((tool) => (
        <Badge key={tool} variant="outline" sx={stylexStyles.rule4}>
          {tool}
        </Badge>
      ))}
    </div>
  )
}

/** SubagentSteps renders the child session's steps. */
function SubagentSteps({ steps = [] }: { steps?: SubagentStep[] }) {
  if (steps.length === 0) {
    return null
  }

  return (
    <Trace>
      {steps.map((step) => (
        <TraceLine key={step.id} status={step.status}>
          <TraceIcon>{subagentIcon(step.status)}</TraceIcon>
          <TraceTitle>{step.title}</TraceTitle>
          <span sx={stylexStyles.rule5}>{statusLabel(step.status)}</span>
        </TraceLine>
      ))}
    </Trace>
  )
}

/** SubagentNote labels task or result text so the two can be told apart. */
function SubagentNote({ label, text }: { label: string; text?: string }) {
  if (!text) {
    return null
  }

  return (
    <>
      <span sx={stylexStyles.rule5}>{label}</span>
      <p>{text}</p>
    </>
  )
}

/** Subagent renders a child session, with a link to its viewer beside the line. */
function Subagent({
  name,
  status,
  task,
  model,
  tools,
  steps,
  result,
  href,
  seconds,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    name: string
    status: TraceStatus
    task?: string
    model?: string
    tools?: string[]
    steps?: SubagentStep[]
    result?: string
    /** href is the viewer URL. It is rendered beside the trigger, never inside it. */
    href?: string
    /** seconds is the settled duration, shown beside the title. */
    seconds?: number
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  // Name the line and keep the viewer link beside it, outside any trigger.
  const line = <SubagentLine name={name} status={status} seconds={seconds} />
  const link = <SubagentLink href={href} name={name} />
  const hasBody = subagentHasBody({ task, model, tools, steps, result })
  if (!hasBody) {
    return (
      <div data-slot="subagent" {...stylexProps(sx, props)}>
        <Trace>
          <div sx={stylexStyles.rule6}>
            {line}
            {link}
          </div>
        </Trace>
      </div>
    )
  }

  return (
    <div data-slot="subagent" {...stylexProps(sx, props)}>
      <Trace>
        {/* Keyed by status so a finished run remounts closed. */}
        <TraceCollapsible key={status} defaultOpen={status === 'running'}>
          <div sx={stylexStyles.rule6}>
            <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
            {link}
          </div>
          <TraceCollapsibleContent sx={stylexStyles.rule7}>
            <SubagentBadges model={model} tools={tools} />
            <SubagentNote label="Task" text={task} />
            <SubagentSteps steps={steps} />
            <SubagentNote label="Result" text={result} />
          </TraceCollapsibleContent>
        </TraceCollapsible>
      </Trace>
    </div>
  )
}

export { Subagent }
