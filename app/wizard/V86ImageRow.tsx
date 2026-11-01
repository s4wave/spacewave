import type { ReactNode } from 'react'

import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'

const stylexStyles = stylex.create({
  rule1: {
    width: '100%',
    justifyContent: 'flex-start',
    textAlign: 'left',
    whiteSpace: 'normal',
  },
})

// V86ImageRowProps describes one image a new VM can boot.
export interface V86ImageRowProps {
  icon: ReactNode
  title: string
  detail: string
  selected: boolean
  onSelect: () => void
}

// V86ImageRow is one image choice in the VM wizard and its catalog.
export function V86ImageRow({
  icon,
  title,
  detail,
  selected,
  onSelect,
}: V86ImageRowProps) {
  return (
    <Button
      type="button"
      aria-pressed={selected}
      variant={selected ? 'selectedRow' : 'unselectedRow'}
      size="row"
      sx={stylexStyles.rule1}
      onClick={onSelect}
    >
      <span className="bg-foreground/5 text-foreground-alt flex size-8 shrink-0 items-center justify-center rounded-md">
        {icon}
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-foreground truncate text-sm font-medium">
          {title}
        </span>
        <span className="text-foreground-alt text-xs leading-relaxed font-normal">
          {detail}
        </span>
      </span>
    </Button>
  )
}
