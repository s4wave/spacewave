import { useKeybindingEditorContext } from './KeybindingEditorContext.js'

export function KeybindingDiscoverySettings() {
  const {
    selectedController,
    selectedSettingsEditable,
    updateLeaderCombo,
    updateWhichKeyDelay,
  } = useKeybindingEditorContext()

  return (
    <div className="space-y-2">
      <div className="text-foreground text-xs font-medium">
        Discovery settings
      </div>
      <div className="border-foreground/8 grid gap-3 rounded border p-3">
        <label className="grid gap-1 text-xs">
          <span className="text-foreground-alt">Leader combo</span>
          <input
            aria-label="Leader combo"
            className="bg-background border-foreground/10 text-foreground rounded border px-2 py-1.5 font-mono text-sm outline-none disabled:opacity-50"
            placeholder="Ctrl+Space"
            value={selectedController.overrideSet.settings.leaderCombo ?? ''}
            disabled={!selectedSettingsEditable}
            onChange={(event) => updateLeaderCombo(event.currentTarget.value)}
          />
        </label>
        <label className="grid gap-1 text-xs">
          <span className="text-foreground-alt">Which-key delay (ms)</span>
          <input
            aria-label="Which-key delay"
            className="bg-background border-foreground/10 text-foreground rounded border px-2 py-1.5 font-mono text-sm outline-none disabled:opacity-50"
            min={0}
            type="number"
            value={
              selectedController.overrideSet.settings.whichKeyDelayMs ?? ''
            }
            disabled={!selectedSettingsEditable}
            onChange={(event) => updateWhichKeyDelay(event.currentTarget.value)}
          />
        </label>
      </div>
    </div>
  )
}
