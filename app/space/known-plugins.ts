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

// KNOWN_SPACE_PLUGINS is the curated set of plugin manifest IDs the app knows
// how to install into a Space. It stands in for a backend "available plugins"
// listing surface, which does not yet exist: FetchManifest resolves a single
// known manifest ID and SpacePluginStatus only projects plugins already on the
// Space, so no RPC enumerates the Release World plugin catalog for browsing.
// When such a listing owner lands, replace this constant with its result.
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
