import { stylexProps } from '@s4wave/web/style/props.js'
import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuListChecks, LuListTodo } from '@s4wave/web/images/icons.js'

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
import { statusLabel, stepIcon } from '@s4wave/web/ui/trace-status.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
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

/** QueueItem is one todo. id is the caller's identity for the item. */
export type QueueItem = {
  id: string
  title: string
  description?: string
  status: TraceStatus
}

/** QueueItemLine renders one todo. A description opens under the line. */
function QueueItemLine({ item }: { item: QueueItem }) {
  const line = (
    <TraceLine status={item.status}>
      <TraceIcon>{stepIcon(item.status)}</TraceIcon>
      <TraceTitle>{item.title}</TraceTitle>
      <span sx={stylexStyles.rule1}>{statusLabel(item.status)}</span>
    </TraceLine>
  )
  if (!item.description) {
    return line
  }

  return (
    <TraceCollapsible>
      <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
      <TraceCollapsibleContent>{item.description}</TraceCollapsibleContent>
    </TraceCollapsible>
  )
}

/** QueueSection renders one counted group of todos. */
function QueueSection({
  title,
  icon,
  items,
}: {
  title: string
  icon: React.ReactNode
  items: QueueItem[]
}) {
  return (
    <TraceCollapsible defaultOpen>
      <TraceCollapsibleTrigger>
        <TraceLine>
          <TraceIcon>{icon}</TraceIcon>
          <TraceTitle>
            {items.length} {title}
          </TraceTitle>
        </TraceLine>
      </TraceCollapsibleTrigger>
      <TraceCollapsibleContent>
        <Trace>
          {items.map((item) => (
            <QueueItemLine key={item.id} item={item} />
          ))}
        </Trace>
      </TraceCollapsibleContent>
    </TraceCollapsible>
  )
}

/** Queue renders the todo list, split into what is left and what is done. */
function Queue({
  items,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    items: QueueItem[]
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  const todo = items.filter((item) => item.status !== 'done')
  const completed = items.filter((item) => item.status === 'done')

  return (
    <div data-slot="queue" {...stylexProps(sx, props)}>
      <Trace>
        {todo.length > 0 ? (
          <QueueSection title="to do" icon={<LuListTodo />} items={todo} />
        ) : null}
        {completed.length > 0 ? (
          <QueueSection
            title="completed"
            icon={<LuListChecks />}
            items={completed}
          />
        ) : null}
      </Trace>
    </div>
  )
}

export { Queue }
