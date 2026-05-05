import { useCallback } from 'react'

import { useCommand } from '@s4wave/web/command/useCommand.js'
import { useIsTabActive } from '@s4wave/web/contexts/TabActiveContext.js'

import type { QuickstartOption } from './options.js'
import { useVisibleQuickstartOptions } from './useQuickstartOptions.js'

interface QuickstartCommandsProps {
  onQuickstart: (opt: QuickstartOption) => void
}

// QuickstartCommands registers quickstart commands for landing and dashboard.
export function QuickstartCommands({ onQuickstart }: QuickstartCommandsProps) {
  const isTabActive = useIsTabActive()
  const quickstartOptions = useVisibleQuickstartOptions()

  return (
    <>
      {quickstartOptions
        .filter((opt) => opt.category !== 'account')
        .map((opt) => (
          <QuickstartCommand
            key={opt.id}
            opt={opt}
            isTabActive={isTabActive}
            onQuickstart={onQuickstart}
          />
        ))}
    </>
  )
}

function QuickstartCommand({
  opt,
  isTabActive,
  onQuickstart,
}: {
  opt: QuickstartOption
  isTabActive: boolean
  onQuickstart: (opt: QuickstartOption) => void
}) {
  useCommand({
    commandId: `spacewave.create.${opt.id}`,
    label: opt.name,
    description: opt.description,
    menuPath: `File/New Space/${opt.name}`,
    menuGroup: 1,
    menuOrder: 10,
    active: isTabActive,
    handler: useCallback(() => onQuickstart(opt), [onQuickstart, opt]),
  })

  return null
}
