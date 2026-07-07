import React from 'react'
import { LuMonitor, LuNotebookPen, LuPuzzle } from 'react-icons/lu'

// KnownSpacePlugin describes a plugin the app can suggest installing into a
// Space by manifest ID.
export interface KnownSpacePlugin {
  // Id is the plugin manifest ID stored in SpaceSettings.pluginIds.
  id: string
  // Name is the human-readable plugin name.
  name: string
  // Description is a short summary of what the plugin adds.
  description: string
  // Icon renders beside the plugin name in the picker.
  icon: React.ComponentType<{ className?: string }>
}

// KNOWN_SPACE_PLUGINS is the curated presentation metadata (name, icon, fallback
// description) for plugin manifest IDs the app recognizes. The browsable catalog
// itself is streamed from the backend as SpaceContentsState.availablePlugins;
// this table supplies display polish for those entries and stands in as the
// browsable set until the catalog has synced.
export const KNOWN_SPACE_PLUGINS: readonly KnownSpacePlugin[] = [
  {
    id: 'spacewave-notes',
    name: 'Notes',
    description: 'Markdown notebooks, blogs, and documentation objects',
    icon: LuNotebookPen,
  },
  {
    id: 'spacewave-v86',
    name: 'V86 VM',
    description: 'x86 virtual machine that runs in the browser',
    icon: LuMonitor,
  },
] as const

// knownSpacePlugin returns the catalog entry for a manifest ID, or a generic
// fallback entry when the ID is not in the curated set.
export function knownSpacePlugin(id: string): KnownSpacePlugin {
  return (
    KNOWN_SPACE_PLUGINS.find((plugin) => plugin.id === id) ?? {
      id,
      name: id,
      description: '',
      icon: LuPuzzle,
    }
  )
}
