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

import * as stylex from '@stylexjs/stylex'
import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    flex: 1,
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    minHeight: 0,
  },
  rule2: { flex: 1, overflow: 'auto', minWidth: 0 },
})

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
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        {kind === 'resources' ? <ResourceTreeTab /> : <StateTreeTab />}
      </div>
      {selectedResource && <ResourceDetailsPanel resource={selectedResource} />}
      {selectedAtom && <StateDetailsPanel entry={selectedAtom} />}
    </div>
  )
}
