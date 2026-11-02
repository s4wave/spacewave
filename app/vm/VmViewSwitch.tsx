import type { IconType } from '@s4wave/web/images/icons.js'

import { cn } from '@s4wave/web/style/utils.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

// VmViewOption is one panel a running VM can show.
export interface VmViewOption<V extends string> {
  view: V
  label: string
  icon: IconType
}

// VmViewSwitchProps configures the panel switch of a running VM.
export interface VmViewSwitchProps<V extends string> {
  options: readonly VmViewOption<V>[]
  view: V
  onViewChange: (view: V) => void
}

// VmViewSwitch is the segmented switch between the panels of a running VM,
// such as its screen and its console. Labels collapse to icons in a narrow
// header.
export function VmViewSwitch<V extends string>({
  options,
  view,
  onViewChange,
}: VmViewSwitchProps<V>) {
  return (
    <div
      role="group"
      aria-label="VM panel"
      className="bg-foreground/5 flex h-6 shrink-0 items-center rounded-md p-0.5"
    >
      {options.map(({ view: option, label, icon: Icon }) => (
        <button
          key={option}
          type="button"
          aria-pressed={view === option}
          aria-label={label}
          onClick={() => onViewChange(option)}
          className={cn(
            'focus-visible:ring-ring/50 flex h-5 items-center gap-1.5 rounded px-2 text-xs transition-colors outline-none focus-visible:ring-2',
            view === option
              ? 'bg-background text-foreground shadow-sm'
              : 'text-foreground-alt hover:text-foreground',
          )}
        >
          <Icon sx={stylexStyles.rule1} />
          <span className="hidden @lg:inline">{label}</span>
        </button>
      ))}
    </div>
  )
}
