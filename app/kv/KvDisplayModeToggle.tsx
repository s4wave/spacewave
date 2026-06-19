import { cn } from '@s4wave/web/style/utils.js'

import {
  KV_DISPLAY_MODES,
  KV_DISPLAY_MODE_LABELS,
  type KvDisplayMode,
} from './kv-encoding.js'

interface KvDisplayModeToggleProps {
  mode: KvDisplayMode
  onModeChange: (mode: KvDisplayMode) => void
}

// KvDisplayModeToggle renders the text/hex/JSON value display mode toggle.
export function KvDisplayModeToggle({
  mode,
  onModeChange,
}: KvDisplayModeToggleProps) {
  return (
    <div
      role="radiogroup"
      aria-label="Value display mode"
      className="border-foreground/10 inline-flex overflow-hidden rounded-md border"
    >
      {KV_DISPLAY_MODES.map((option) => (
        <button
          key={option}
          type="button"
          role="radio"
          aria-checked={option === mode}
          onClick={() => onModeChange(option)}
          className={cn(
            'px-2 py-0.5 text-[0.6rem] font-medium transition-colors',
            option === mode
              ? 'bg-brand/15 text-foreground'
              : 'text-foreground-alt/60 hover:text-foreground hover:bg-foreground/5',
          )}
        >
          {KV_DISPLAY_MODE_LABELS[option]}
        </button>
      ))}
    </div>
  )
}
