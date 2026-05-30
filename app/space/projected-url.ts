import { pluginPathPrefix } from '@s4wave/app/urls.js'
import {
  buildProjectedObjectPath,
  normalizeProjectedSubpath,
} from '@s4wave/sdk/space/projected-path.js'

export interface ProjectedObjectURLOpts {
  sessionIndex: number
  sharedObjectId: string
  objectKey: string
  path: string
}

export function buildProjectedFileURL(opts: ProjectedObjectURLOpts): string {
  return `${pluginPathPrefix}/fs/${buildProjectedObjectContentPath(opts)}`
}

export function buildProjectedFileInlineURL(
  opts: ProjectedObjectURLOpts,
): string {
  return `${buildProjectedFileURL(opts)}?inline=1`
}

export function buildProjectedObjectContentPath(
  opts: ProjectedObjectURLOpts,
): string {
  const projectedPath = buildProjectedObjectPath(opts)
  return normalizeProjectedSubpath(opts.path)
    ? projectedPath
    : `${projectedPath}/-`
}

export function buildProjectedExportURL(opts: ProjectedObjectURLOpts): string {
  return `${pluginPathPrefix}/export/${buildProjectedObjectContentPath(opts)}`
}
