import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

export interface SpacewaveAppContextSnapshot {
  objectKey?: string
  objectType?: string
  viewerComponentID?: string
  tabID?: string
  layoutObjectKey?: string
}

export interface SpacewaveViewerCatalog {
  base?: ObjectViewerComponent[]
  product?: ObjectViewerComponent[]
  downstream?: ObjectViewerComponent[]
}

export function createViewerCatalog({
  base = [],
  product = [],
  downstream = [],
}: SpacewaveViewerCatalog): ObjectViewerComponent[] {
  return [...base, ...product, ...downstream]
}

export function mergeViewerCatalogs(
  ...catalogs: SpacewaveViewerCatalog[]
): ObjectViewerComponent[] {
  return catalogs.flatMap(createViewerCatalog)
}
