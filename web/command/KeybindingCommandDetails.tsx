import { LuArrowLeft } from 'react-icons/lu'

import { KeybindingBindingsSection } from './KeybindingBindingsSection.js'
import { KeybindingConflictList } from './KeybindingConflictList.js'
import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

export function KeybindingCommandDetails() {
  const {
    commandConflicts,
    pendingConflict,
    selectedCommandId,
    selectedLayerStatus,
    selectedRow,
    setSelectedCommandId,
  } = useKeybindingEditorContext()

  if (!selectedRow) {
    return (
      <section className="text-foreground-alt hidden min-h-0 place-items-center p-8 text-center text-sm sm:grid">
        Choose a command to see its shortcuts.
      </section>
    )
  }

  const conflicts = pendingConflict ? [pendingConflict] : commandConflicts

  return (
    <section
      className={
        selectedCommandId
          ? 'flex min-h-0 flex-col overflow-hidden'
          : 'hidden min-h-0 flex-col overflow-hidden sm:flex'
      }
    >
      <div className="border-foreground/8 flex min-h-14 shrink-0 items-center gap-3 border-b px-4 sm:px-5">
        <button
          type="button"
          className="text-foreground-alt hover:text-foreground -ml-2 flex min-h-10 items-center gap-1 rounded px-2 text-xs sm:hidden"
          onClick={() => setSelectedCommandId(null)}
        >
          <LuArrowLeft className="size-4" />
          Back
        </button>
        <div className="min-w-0">
          <h2 className="text-foreground truncate text-sm font-semibold">
            {selectedRow.label}
          </h2>
          {selectedRow.menuPath && (
            <div className="text-foreground-alt/70 truncate text-xs">
              {selectedRow.menuPath.split('/').join(' › ')}
            </div>
          )}
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-4 pb-8 sm:p-5">
        <div className="mx-auto max-w-2xl space-y-4">
          {selectedLayerStatus && (
            <div className="border-warning/30 bg-warning/10 text-warning rounded-md border px-3 py-2 text-xs">
              {selectedLayerStatus}
            </div>
          )}
          <KeybindingBindingsSection />
          {conflicts.length > 0 && (
            <KeybindingConflictList conflicts={conflicts} />
          )}
        </div>
      </div>
    </section>
  )
}
