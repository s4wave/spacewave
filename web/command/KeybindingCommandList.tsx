import { LuSearch } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { formatKeybindingHint } from './CommandPalette.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

export function KeybindingCommandList() {
  const {
    accountOverridesAvailable,
    spaceOverridesAvailable,
    rows,
    query,
    selectedRow,
    selectedScope,
    setQuery,
    setSelectedCommandId,
    setSelectedScope,
  } = useKeybindingEditorContext()

  return (
    <aside className="border-foreground/8 bg-background-card/20 flex min-h-0 flex-col border-r">
      <div className="border-foreground/8 border-b p-3">
        <label className="text-foreground-alt mb-1 block text-xs font-medium">
          Scope
        </label>
        <select
          className="bg-background border-foreground/10 text-foreground w-full rounded border px-2 py-1.5 text-sm outline-none"
          value={selectedScope}
          onChange={(event) => {
            const scope = event.target.value
            if (scope === 'local' || scope === 'account' || scope === 'space') {
              setSelectedScope(scope)
            }
          }}
        >
          <option value="local">Local</option>
          <option value="account" disabled={!accountOverridesAvailable}>
            {accountOverridesAvailable ? 'Account' : 'Account (unavailable)'}
          </option>
          <option value="space" disabled={!spaceOverridesAvailable}>
            {spaceOverridesAvailable ? 'Space' : 'Space (unavailable)'}
          </option>
        </select>
      </div>
      <div className="border-foreground/8 flex items-center gap-2 border-b px-3 py-2">
        <LuSearch className="text-brand h-3.5 w-3.5" />
        <input
          className="placeholder:text-foreground-alt/50 text-foreground min-w-0 flex-1 bg-transparent text-sm outline-none"
          placeholder="Search commands..."
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onInput={(event) => setQuery(event.currentTarget.value)}
        />
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-2">
        {rows.length === 0 && (
          <div className="text-foreground-alt/40 px-2 py-4 text-sm">
            No commands found.
          </div>
        )}
        {rows.map((row) => (
          <button
            key={row.commandId}
            type="button"
            className={cn(
              'hover:bg-foreground/5 border-foreground/6 flex w-full items-center gap-3 rounded border border-transparent px-2 py-2 text-left transition-colors',
              row.commandId === selectedRow?.commandId &&
                'border-brand/30 bg-brand/15 text-foreground',
            )}
            onClick={() => setSelectedCommandId(row.commandId)}
          >
            <span className="min-w-0 flex-1">
              <span className="text-foreground block truncate text-xs font-medium">
                {row.label}
              </span>
              <span className="text-foreground-alt/50 block truncate text-[0.6rem]">
                {row.commandId}
              </span>
            </span>
            {row.displayBindings.length > 0 && (
              <span className="text-brand/90 shrink-0 text-right font-mono text-[0.6rem]">
                {formatKeybindingHint(row.displayBindings)}
              </span>
            )}
          </button>
        ))}
      </div>
    </aside>
  )
}
