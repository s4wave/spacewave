import { ResourceDetailsPanel } from '@s4wave/web/devtools/ResourceDetailsPanel.js'
import { ResourceTreeTab } from '@s4wave/web/devtools/ResourceTreeTab.js'
import { StateDetailsPanel } from '@s4wave/web/devtools/StateDetailsPanel.js'
import {
  useSelectedResourceId,
  useTrackedResources,
} from '@s4wave/web/devtools/index.js'
import { useSelectedStateAtomId } from '@s4wave/web/devtools/StateDevToolsContext.js'
import { StateTreeTab } from '@s4wave/web/devtools/StateTreeTab.js'
import { useStateInspectorEntryMap } from '@s4wave/web/devtools/useStateInspectorEntries.js'

// UnderTheHoodInspector embeds the devtools trees: the SDK resources this app
// holds open, or the UI state atoms, each beside the selected entry's details.
export function UnderTheHoodInspector({
  kind,
}: {
  kind: 'resources' | 'atoms'
}) {
  const selectedResourceId = useSelectedResourceId()
  const resources = useTrackedResources()
  const selectedAtomId = useSelectedStateAtomId()
  const atoms = useStateInspectorEntryMap()

  const selectedResource =
    kind === 'resources' && selectedResourceId
      ? resources.get(selectedResourceId)
      : undefined
  const selectedAtom =
    kind === 'atoms' && selectedAtomId ? atoms.get(selectedAtomId) : undefined

  return (
    <div className="border-foreground/8 bg-background-card/30 flex min-h-0 flex-1 overflow-hidden rounded-lg border">
      <div className="min-w-0 flex-1 overflow-auto">
        {kind === 'resources' ? <ResourceTreeTab /> : <StateTreeTab />}
      </div>
      {selectedResource && <ResourceDetailsPanel resource={selectedResource} />}
      {selectedAtom && <StateDetailsPanel entry={selectedAtom} />}
    </div>
  )
}
