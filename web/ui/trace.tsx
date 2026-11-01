import * as React from 'react'
import { LuChevronRight } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@s4wave/web/ui/collapsible.js'

/** TraceStatus is the live state of one trace line. Done is the settled state. */
export type TraceStatus = 'running' | 'done' | 'error' | 'pending'

/** Trace is a list of agent steps. */
function Trace({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="trace"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: *:has-[>[data-slot=trace-line]]:flex */ cn(
          'flex w-full min-w-0 flex-col gap-2 *:has-[>[data-slot=trace-line]]:flex *:has-[>[data-slot=trace-line]]:w-fit *:has-[>[data-slot=trace-line]]:max-w-full *:has-[>[data-slot=trace-line]]:min-w-0 *:has-[>[data-slot=trace-line]]:rounded-md *:has-[>[data-slot=trace-line]]:text-left *:has-[>[data-slot=trace-line]]:outline-none *:has-[>[data-slot=trace-line]]:focus-visible:ring-3 *:has-[>[data-slot=trace-line]]:focus-visible:ring-ring/50',
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceLine is one step, with an optional running, done, error, or pending status. */
function TraceLine({
  className,
  status,
  ...props
}: React.ComponentProps<'span'> & {
  status?: TraceStatus
}) {
  return (
    <span
      data-slot="trace-line"
      data-status={status}
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: in-[a,button]:cursor-pointer */ cn(
          'flex w-fit max-w-full min-w-0 items-center gap-2 text-sm text-muted-foreground transition-colors in-[a,button]:cursor-pointer in-[a:hover,button:hover]:text-foreground data-[status=error]:text-destructive data-[status=error]:in-[a:hover,button:hover]:text-destructive data-[status=pending]:opacity-60 data-[status=running]:*:data-[slot=trace-title]:shimmer',
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceIcon holds the step icon. */
function TraceIcon({ className, ...props }: React.ComponentProps<'span'>) {
  return (
    <span
      data-slot="trace-icon"
      aria-hidden="true"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: [&_svg]:pointer-events-none */ cn(
          "flex size-4 shrink-0 items-center justify-center [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 [[data-slot=trace-title]~&]:-ml-1",
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceTitle is the step name. A running line shimmers this text. */
function TraceTitle({ className, ...props }: React.ComponentProps<'span'>) {
  return (
    <span
      data-slot="trace-title"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'min-w-0 truncate motion-reduce:shimmer-none',
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceDescription is the optional detail beside the title. */
function TraceDescription({
  className,
  ...props
}: React.ComponentProps<'span'>) {
  return (
    <span
      data-slot="trace-description"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'shrink-0 text-muted-foreground/70 tabular-nums',
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceCollapsible nests the steps revealed by its trigger. */
function TraceCollapsible({
  className,
  ...props
}: Omit<React.ComponentProps<typeof Collapsible>, 'asChild' | 'open'> &
  (
    | { open?: undefined }
    | {
        open: boolean
        onOpenChange: NonNullable<
          React.ComponentProps<typeof Collapsible>['onOpenChange']
        >
      }
  )) {
  return (
    <Collapsible
      data-slot="trace-collapsible"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'flex w-full min-w-0 flex-col',
          className,
        )
      }
      {...props}
    />
  )
}

/** TraceCollapsibleTrigger toggles the nested steps and shows the disclosure icon. */
function TraceCollapsibleTrigger({
  className,
  children,
  ...props
}: Omit<React.ComponentProps<typeof CollapsibleTrigger>, 'asChild'>) {
  return (
    <CollapsibleTrigger
      data-slot="trace-collapsible-trigger"
      className={
        /* TODO(stylex): global or unknown class: group/trace-collapsible-trigger */ cn(
          'group/trace-collapsible-trigger flex w-fit max-w-full min-w-0 cursor-pointer items-center gap-1 rounded-md text-left text-muted-foreground transition-colors outline-none hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 has-[[data-status=error]]:text-destructive',
          className,
        )
      }
      {...props}
    >
      {children}
      <TraceIcon
        className={
          /* TODO(stylex): a descendant, group, peer or compound selector: group-aria-expanded/trace-collapsible-trigger:rotate-90 */ 'transition-transform group-aria-expanded/trace-collapsible-trigger:rotate-90'
        }
      >
        <LuChevronRight />
      </TraceIcon>
    </CollapsibleTrigger>
  )
}

/** TraceCollapsibleContent indents the nested steps under the trigger. */
function TraceCollapsibleContent({
  className,
  children,
  ...props
}: Omit<React.ComponentProps<typeof CollapsibleContent>, 'asChild'>) {
  return (
    <CollapsibleContent
      data-slot="trace-collapsible-content"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: data-open:animate-accordion-down */ 'data-open:animate-accordion-down data-closed:animate-accordion-up min-w-0 overflow-hidden duration-200 [--radix-accordion-content-height:var(--radix-collapsible-content-height)]'
      }
      {...props}
    >
      <div
        data-slot="trace-collapsible-content-inner"
        className={
          /* TODO(stylex): dynamic class or forwarded className */ cn(
            'mt-2 mb-1 ml-[calc(--spacing(2)-1px)] min-w-0 border-l pr-1 pl-4 leading-relaxed wrap-break-word text-muted-foreground',
            className,
          )
        }
      >
        {children}
      </div>
    </CollapsibleContent>
  )
}

export {
  Trace,
  TraceLine,
  TraceIcon,
  TraceTitle,
  TraceDescription,
  TraceCollapsible,
  TraceCollapsibleTrigger,
  TraceCollapsibleContent,
}
