import { Button } from '@s4wave/web/ui/button.js'

import { useKeybindingEditorContext } from './KeybindingEditorContext.js'
import { scopeLabels } from './component.js'

export function KeybindingDiscoverySettings() {
  const {
    selectedController,
    selectedSettingsEditable,
    selectedScope,
    updateLeaderCombo,
    updateWhichKeyDelay,
  } = useKeybindingEditorContext()

  return (
    <div className="grid gap-3">
      <label className="grid gap-1 text-xs">
        <span className="text-foreground-alt">Leader key</span>
        <input
          aria-label="Leader combo"
          className="bg-background border-foreground/10 text-brand focus:border-brand/50 min-h-9 rounded border px-2 font-mono text-sm outline-none disabled:opacity-50"
          placeholder="Ctrl+Space"
          value={selectedController.overrideSet.settings.leaderCombo ?? ''}
          disabled={!selectedSettingsEditable}
          onChange={(event) => updateLeaderCombo(event.currentTarget.value)}
        />
      </label>
      <label className="grid gap-1 text-xs">
        <span className="text-foreground-alt">Show key guide after</span>
        <span className="flex items-center gap-2">
          <input
            aria-label="Which-key delay"
            className="bg-background border-foreground/10 text-foreground focus:border-brand/50 min-h-9 min-w-0 flex-1 rounded border px-2 font-mono text-sm outline-none disabled:opacity-50"
            min={0}
            placeholder="0"
            type="number"
            value={
              selectedController.overrideSet.settings.whichKeyDelayMs ?? ''
            }
            disabled={!selectedSettingsEditable}
            onChange={(event) => updateWhichKeyDelay(event.currentTarget.value)}
          />
          <span className="text-foreground-alt">ms</span>
        </span>
      </label>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="justify-start px-0"
        disabled={!selectedSettingsEditable}
        onClick={selectedController.resetLayer}
      >
        Reset {scopeLabels[selectedScope]} settings
      </Button>
    </div>
  )
}
