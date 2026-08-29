import { LuChevronDown, LuSearch, LuSettings2 } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

import { formatKeybindingHint } from './CommandPalette.js'
import { KeybindingDiscoverySettings } from './KeybindingDiscoverySettings.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { scopeLabels, type KeybindingEditorScope } from './component.js'

const scopes: KeybindingEditorScope[] = ['local', 'account', 'space']

export function KeybindingCommandList() {
  const {
    accountOverridesAvailable,
    spaceOverridesAvailable,
    rows,
    query,
    selectedCommandId,
    selectedRow,
    selectedScope,
    setQuery,
    setSelectedCommandId,
    setSelectedScope,
  } = useKeybindingEditorContext()

  return (
    <aside
      className={cn(
        'bg-background-card-alt/40 min-h-0 flex-col border-r border-foreground/8',
        selectedCommandId ? 'hidden sm:flex' : 'flex',
      )}
    >
      <div className="border-foreground/8 space-y-3 border-b p-3">
        <div>
          <div className="text-foreground-alt mb-1.5 text-xs font-medium">
            Save changes to
          </div>
          <div className="bg-background/60 border-foreground/8 grid grid-cols-3 rounded-md border p-0.5">
            {scopes.map((scope) => {
              const available =
                scope === 'local' ||
                (scope === 'account'
                  ? accountOverridesAvailable
                  : spaceOverridesAvailable)
              return (
                <button
                  key={scope}
                  type="button"
                  className={cn(
                    'min-h-8 rounded px-2 text-xs transition-colors',
                    scope === selectedScope
                      ? 'bg-background-tertiary text-foreground shadow-sm'
                      : 'text-foreground-alt hover:text-foreground',
                  )}
                  disabled={!available}
                  title={
                    available
                      ? undefined
                      : scope === 'account'
                        ? 'Sign in to save shortcuts to your account'
                        : 'Open a Space to save shortcuts there'
                  }
                  onClick={() => setSelectedScope(scope)}
                >
                  {scope === 'local' ? 'This device' : scopeLabels[scope]}
                </button>
              )
            })}
          </div>
        </div>
        <details className="group">
          <summary className="text-foreground-alt hover:text-foreground flex cursor-pointer list-none items-center gap-2 text-xs">
            <LuSettings2 className="size-3.5" />
            Discovery settings
            <LuChevronDown className="ml-auto size-3 transition-transform group-open:rotate-180" />
          </summary>
          <div className="pt-3">
            <KeybindingDiscoverySettings />
          </div>
        </details>
      </div>
      <label className="border-foreground/8 flex min-h-11 items-center gap-2 border-b px-3">
        <LuSearch className="text-brand size-3.5" />
        <span className="sr-only">Search commands or shortcuts</span>
        <input
          autoFocus
          className="placeholder:text-foreground-alt/50 text-foreground min-w-0 flex-1 bg-transparent text-sm outline-none"
          placeholder="Search commands or shortcuts…"
          value={query}
          onChange={(event) => setQuery(event.currentTarget.value)}
        />
      </label>
      <div className="min-h-0 flex-1 overflow-auto p-2">
        {rows.length === 0 && (
          <div className="text-foreground-alt px-2 py-8 text-center text-sm">
            {query
              ? `No shortcuts match “${query}”.`
              : 'No commands are available.'}
            {query && (
              <button
                type="button"
                className="text-brand hover:text-brand-highlight mt-2 block w-full text-xs"
                onClick={() => setQuery('')}
              >
                Clear search
              </button>
            )}
          </div>
        )}
        {rows.map((row) => (
          <button
            key={row.commandId}
            type="button"
            className={cn(
              'hover:bg-foreground/5 flex min-h-11 w-full items-center gap-3 rounded-md border border-transparent px-2.5 py-2 text-left transition-colors',
              row.commandId === selectedRow?.commandId &&
                'border-brand/20 bg-brand/10 text-foreground',
            )}
            onClick={() => setSelectedCommandId(row.commandId)}
          >
            <span className="text-foreground min-w-0 flex-1 truncate text-xs font-medium">
              {row.label}
            </span>
            {row.displayBindings.length > 0 && (
              <kbd className="bg-background-tertiary text-foreground-alt border-foreground/8 max-w-28 shrink-0 truncate rounded border px-1.5 py-1 font-mono text-[0.65rem]">
                {formatKeybindingHint(row.displayBindings)}
              </kbd>
            )}
          </button>
        ))}
      </div>
    </aside>
  )
}
