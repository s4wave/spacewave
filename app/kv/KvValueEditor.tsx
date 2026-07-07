import { useCallback } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import { KvDisplayModeToggle } from './KvDisplayModeToggle.js'
import type { KvDisplayMode } from './kv-encoding.js'

interface KvValueEditorProps {
  label: string
  draft: string
  onDraftChange: (draft: string) => void
  mode: KvDisplayMode
  onModeChange: (mode: KvDisplayMode) => void
  parseError: string | null
  disabled?: boolean
  rows?: number
  ariaLabel: string
}

// KvValueEditor renders a labeled value textarea with a display-mode toggle and
// inline parse error, shared by the create and update flows.
export function KvValueEditor({
  label,
  draft,
  onDraftChange,
  mode,
  onModeChange,
  parseError,
  disabled,
  rows = 6,
  ariaLabel,
}: KvValueEditorProps) {
  const updateDraft = useCallback(
    (event: React.ChangeEvent<HTMLTextAreaElement>) => {
      onDraftChange(event.target.value)
    },
    [onDraftChange],
  )

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between">
        <label className="text-foreground text-xs font-medium select-none">
          {label}
        </label>
        <KvDisplayModeToggle mode={mode} onModeChange={onModeChange} />
      </div>
      <textarea
        value={draft}
        onChange={updateDraft}
        disabled={disabled}
        rows={rows}
        aria-label={ariaLabel}
        aria-invalid={parseError ? true : undefined}
        spellCheck={false}
        className={cn(
          'border-foreground/10 bg-background/20 text-foreground w-full rounded-md border px-2.5 py-1.5 font-mono text-xs',
          'placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 focus-visible:ring-[3px] focus-visible:outline-none',
          'disabled:cursor-not-allowed disabled:opacity-50',
          parseError && 'border-destructive/60',
        )}
      />
      {parseError ? (
        <p className="text-destructive text-[0.6rem]">{parseError}</p>
      ) : null}
    </div>
  )
}
