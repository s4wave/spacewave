import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import * as React from 'react'
import { LuTerminal } from 'react-icons/lu'

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
  rule1: { maxHeight: `calc(${themeLayout['--spacing']} * 64)` },
})

/** terminalStatus is running until the command exits, then done or error. */
function terminalStatus(exitCode: number | undefined): TraceStatus {
  if (exitCode === undefined) {
    return 'running'
  }
  if (exitCode === 0) {
    return 'done'
  }
  return 'error'
}

/** terminalTitle names the line from the command and its exit code. */
function terminalTitle(command: string, exitCode: number | undefined) {
  if (exitCode !== undefined && exitCode !== 0) {
    return `${command} exited with code ${exitCode}`
  }
  const status = exitCode === undefined ? 'running' : 'done'
  return statusTitle(status, command)
}

/** Terminal renders a command. It opens once output arrives and keeps that choice. */
function Terminal({
  command,
  output = '',
  exitCode,
  sx,
  ...props
}: Omit<
  React.ComponentProps<'div'> & {
    command: string
    output?: string
    /** exitCode is unset while the command is still running. */
    exitCode?: number
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  // Remember a person's open choice. Until then, open once output arrives.
  const [chosen, setChosen] = React.useState<boolean>()
  const status = terminalStatus(exitCode)
  const failed = status === 'error'
  const line = (
    <TraceLine status={status}>
      <TraceIcon>
        <LuTerminal />
      </TraceIcon>
      <TraceTitle>{terminalTitle(command, exitCode)}</TraceTitle>
    </TraceLine>
  )
  if (output.length === 0) {
    return (
      <div data-slot="terminal" sx={sx} {...props}>
        <Trace>{line}</Trace>
      </div>
    )
  }

  return (
    <div data-slot="terminal" sx={sx} {...props}>
      <Trace>
        <TraceCollapsible open={chosen ?? true} onOpenChange={setChosen}>
          <TraceCollapsibleTrigger>{line}</TraceCollapsibleTrigger>
          <TraceCollapsibleContent>
            <CodeBlock size="sm" variant={failed ? 'destructive' : 'default'}>
              <CodeBlockContent wrap {...stylex.props(stylexStyles.rule1)}>
                {output}
              </CodeBlockContent>
            </CodeBlock>
          </TraceCollapsibleContent>
        </TraceCollapsible>
      </Trace>
    </div>
  )
}

export { Terminal }
