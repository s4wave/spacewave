import { utilityState as paneUtilityState } from '@s4wave/web/style/tokens.stylex.js'
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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    flex: 1,
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 48rem)': 'repeat(2, minmax(0, 1fr))',
    },
    minHeight: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: { flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 },
  rule2: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexWrap: 'wrap',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: { color: themeColors['--color-destructive'] },
  rule4: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: `calc(${themeLayout['--spacing']} * 64)`,
    minWidth: 0,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule6: { flex: 1, minHeight: 0 },
  rule7: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule8: {
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: themeColors['--color-background'],
  },
  rule9: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
})

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
    <div sx={stylexStyles.rule1}>
      <div role="status" sx={stylexStyles.rule2}>
        {frontend.error ? (
          <>
            <span role="alert" sx={stylexStyles.rule3}>
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

      <div sx={stylexStyles2.rule1}>
        <section
          sx={[stylexStyles.rule4, paneDivider.first]}
          aria-label="Plugin source"
        >
          <h3 sx={stylexStyles.rule5}>Source</h3>
          <div sx={stylexStyles.rule6}>
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

        <section sx={stylexStyles.rule4} aria-label="Plugin preview">
          <div sx={stylexStyles.rule7}>
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
              sx={stylexStyles.rule8}
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
              sx={stylexStyles.rule8}
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
          <div sx={stylexStyles.rule6}>
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
              <p sx={stylexStyles.rule9}>
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

// paneDivider follows the source pane from stacked rows to side-by-side columns.
const paneDivider = stylex.create({
  first: {
    '--tw-divide-x-reverse': {
      default: null,
      '@media (min-width:48rem)': { default: null, ':not(:last-child)': 0 },
    },
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderInlineEndStyle: {
      default: null,
      '@media (min-width:48rem)': {
        default: null,
        ':not(:last-child)': paneUtilityState['--tw-border-style'],
      },
    },
    borderInlineEndWidth: {
      default: null,
      '@media (min-width:48rem)': {
        default: null,
        ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-x-reverse)))',
      },
    },
    borderInlineStartStyle: {
      default: null,
      '@media (min-width:48rem)': {
        default: null,
        ':not(:last-child)': paneUtilityState['--tw-border-style'],
      },
    },
    borderInlineStartWidth: {
      default: null,
      '@media (min-width:48rem)': {
        default: null,
        ':not(:last-child)': 'calc(1px * var(--tw-divide-x-reverse))',
      },
    },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': paneUtilityState['--tw-border-style'],
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
      '@media (min-width:48rem)': { default: null, ':not(:last-child)': 0 },
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': paneUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
  },
})
