import { lazy, useId, useMemo, useState } from 'react'

import type { BuildSpacePluginRequest } from '@s4wave/sdk/space/space.pb.js'
import {
  SpaceContext,
  SpaceContentsContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useObjectMetadata } from '@s4wave/web/hooks/useObjectMetadata.js'
import { ViewerRegistryProvider } from '@s4wave/web/hooks/useViewerRegistry.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from '@s4wave/web/object/object.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { resolvePath } from '@s4wave/web/router/router.js'
import {
  usePluginFrontend,
  type PluginFrontendSession,
} from '@s4wave/web/space/usePluginFrontend.js'
import { Button } from '@s4wave/web/ui/button.js'

import { SpacePluginObjects } from './SpacePluginObjects.js'

// previewObjectLimit bounds the preview picker to one World listing page.
const previewObjectLimit = 1000

/**
 * liveViewer builds the preview viewer that loads the entrypoint module from
 * the plugin frontend transport.
 */
function liveViewer(
  attached: PluginFrontendSession,
  entrypoint: string,
  typeID: string,
): ObjectViewerComponent {
  return {
    componentID: `live/${attached.session.id}/${entrypoint}`,
    typeID,
    name: 'Live preview',
    component: lazy(async () => {
      const url = await attached.transport.resolve(entrypoint)
      return import(/* @vite-ignore */ url) as Promise<{
        default: React.ComponentType<ObjectViewerComponentProps>
      }>
    }),
  }
}

/** SpacePluginWorkbench retains source editing and a live custom viewer together. */
export function SpacePluginWorkbench({
  request,
}: {
  request: BuildSpacePluginRequest
}) {
  // The open Space owns data; this workbench owns only its compiler attachment.
  const id = useId()
  const space = SpaceContext.useContext()
  const contents = SpaceContentsContext.useContext()
  const { spaceId, spaceWorldResource } = SpaceContainerContext.useContext()
  const frontend = usePluginFrontend(space, request)
  const [sourcePath, setSourcePath] = useState('/')
  const [entry, setEntry] = useState('')
  const [objectKey, setObjectKey] = useState('')
  const objectKeys = useWorldQuery(
    spaceWorldResource,
    async (world, signal) => {
      const page = await world.listObjects(
        { limit: previewObjectLimit },
        signal,
      )
      return (page.objects ?? []).map((object) => object.objectKey ?? '')
    },
    [],
  ).value
  const selectedType = useObjectMetadata(spaceWorldResource, objectKey).value
    ?.typeId
  const entrypoints = frontend.value?.session.entrypoints ?? []
  const entrypoint = entrypoints.includes(entry) ? entry : entrypoints[0]

  // A source navigation stays inside the workbench, keeping its compiler alive.
  const sourceInfo = useMemo<ObjectInfo>(
    () => ({
      info: {
        case: 'worldObjectInfo',
        value: { objectKey: request.sourceKey, objectType: 'unixfs/fs-node' },
      },
    }),
    [request.sourceKey],
  )
  const previewInfo = useMemo<ObjectInfo>(
    () => ({
      info: {
        case: 'worldObjectInfo',
        value: { objectKey, objectType: selectedType },
      },
    }),
    [objectKey, selectedType],
  )

  // Each loaded module keeps its React identity through compatible Vite edits.
  // The override is confined to this preview; installed viewers keep their artifact.
  const viewers = useMemo<ObjectViewerComponent[]>(() => {
    const attached = frontend.loading ? null : frontend.value
    if (!attached || !entrypoint || !selectedType) {
      return []
    }
    return [liveViewer(attached, entrypoint, selectedType)]
  }, [frontend.value, frontend.loading, entrypoint, selectedType])

  // Keep compiler feedback separate from source editing and accepted app data.
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        role="status"
        className="border-foreground/10 flex flex-wrap items-center gap-2 border-b px-4 py-2 text-xs"
      >
        {frontend.error ? (
          <>
            <span role="alert" className="text-destructive">
              {frontend.error.message}
            </span>
            <Button size="sm" variant="ghost" onClick={frontend.retry}>
              Restart preview
            </Button>
          </>
        ) : frontend.loading ? (
          'Starting preview on your build device…'
        ) : (
          'Live preview connected. Save a source file to apply its changes.'
        )}
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-1 divide-y md:grid-cols-2 md:divide-x md:divide-y-0">
        <section
          className="flex min-h-64 min-w-0 flex-col"
          aria-label="Plugin source"
        >
          <h3 className="border-foreground/10 border-b px-4 py-2 text-xs font-medium">
            Source
          </h3>
          <div className="min-h-0 flex-1">
            <ObjectViewer
              objectInfo={sourceInfo}
              worldState={spaceWorldResource}
              spaceContents={contents}
              standalone
              bottomBarId={`${id}-source`}
              path={sourcePath}
              onNavigate={(to) => setSourcePath(resolvePath(sourcePath, to))}
              stateNamespace={[
                'pluginSource',
                spaceId,
                request.sourceKey ?? '',
              ]}
            />
          </div>
        </section>

        <section
          className="flex min-h-64 min-w-0 flex-col"
          aria-label="Plugin preview"
        >
          <div className="border-foreground/10 grid gap-2 border-b p-3 text-xs">
            {space.value && (
              <SpacePluginObjects
                space={space.value}
                manifestId={request.manifestId ?? ''}
                onCreated={setObjectKey}
              />
            )}
            <label htmlFor={`${id}-entry`}>Viewer module</label>
            <select
              id={`${id}-entry`}
              value={entrypoint ?? ''}
              onChange={(event) => setEntry(event.target.value)}
              className="bg-background rounded border p-2"
              disabled={!entrypoints.length}
            >
              {!entrypoints.length && (
                <option value="">Waiting for frontend modules</option>
              )}
              {entrypoints.map((path) => (
                <option key={path} value={path}>
                  {path}
                </option>
              ))}
            </select>
            <label htmlFor={`${id}-object`}>Preview object</label>
            <select
              id={`${id}-object`}
              value={objectKey}
              onChange={(event) => setObjectKey(event.target.value)}
              className="bg-background rounded border p-2"
            >
              <option value="">Choose an object created by this plugin</option>
              {objectKeys?.flatMap((key) =>
                key === request.sourceKey
                  ? []
                  : [
                      <option key={key} value={key}>
                        {key}
                      </option>,
                    ],
              )}
            </select>
          </div>
          <div className="min-h-0 flex-1">
            {viewers.length > 0 ? (
              <ViewerRegistryProvider staticViewers={viewers}>
                <ObjectViewer
                  objectInfo={previewInfo}
                  worldState={spaceWorldResource}
                  spaceContents={contents}
                  standalone
                  bottomBarId={`${id}-preview`}
                  preferredComponentID={viewers[0].componentID}
                  stateNamespace={['pluginPreview', spaceId, objectKey]}
                />
              </ViewerRegistryProvider>
            ) : (
              <p className="text-foreground-alt/70 p-4 text-sm">
                Build and install the plugin, create one of its objects, then
                select it here.
              </p>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
