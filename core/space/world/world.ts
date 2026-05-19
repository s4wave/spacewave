// SPACE_SETTINGS_OBJECT_KEY is the object key usually used for the SpaceSettings.
export const SPACE_SETTINGS_OBJECT_KEY = 'settings'

// SPACE_SETTINGS_BLOCK_TYPE is the block type identifier for SpaceSettings.
export const SPACE_SETTINGS_BLOCK_TYPE =
  'github.com/s4wave/spacewave/core/space/world.SpaceSettings'

const DNS1123_LABEL_RE = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/
const DNS1123_LABEL_MAX_LENGTH = 63

// isValidSpacePluginId returns whether id can be stored in SpaceSettings.pluginIds.
export function isValidSpacePluginId(id: string): boolean {
  return (
    id.length > 0 &&
    id.length <= DNS1123_LABEL_MAX_LENGTH &&
    DNS1123_LABEL_RE.test(id)
  )
}
