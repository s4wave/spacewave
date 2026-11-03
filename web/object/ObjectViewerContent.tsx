import { Suspense, useCallback } from 'react'
import { WebViewErrorBoundary } from '@aptre/bldr-react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { Button } from '@s4wave/web/ui/button.js'
import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import type { IObjectState } from '@s4wave/sdk/world/object-state.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import type { SpaceContents } from '@s4wave/sdk/space/contents.js'
import {
  SpacePluginLifecycleState,
  type SpaceContentsState,
} from '@s4wave/sdk/space/space.pb.js'

import type {
  ObjectViewerComponent,
  ObjectViewerComponentProps,
} from './object.js'
import { getObjectKey } from './object.js'
import type { ObjectInfo } from './object.pb.js'
import { ObjectViewerLoadingState } from './ObjectViewerLoadingState.js'
import { debugViewerComponentID } from './useObjectViewer.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
} from '../style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
    textAlign: 'left',
    maxWidth: 'var(--container-lg)',
    width: '100%',
  },
  rule2: { textAlign: 'center' },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule4: {
    '--tw-leading': 'var(--leading-relaxed)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: themeLayout['--spacing'],
  },
  rule5: {
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule7: {
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    fontFamily: themeFonts['--font-mono'],
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule8: { display: 'flex', justifyContent: 'center' },
  rule9: {
    '--tw-leading': 'var(--leading-relaxed)',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    textAlign: 'center',
  },
  rule10: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    color: themeColors['--color-muted-foreground'],
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
  },
  rule11: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderColor: themeColors['--color-border'],
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-dark'],
    display: 'flex',
    justifyContent: 'center',
    textAlign: 'center',
    height: '100%',
    width: '100%',
  },
  rule12: {
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    width: '100%',
  },
  rule13: { padding: `calc(${themeLayout['--spacing']} * 1.25)` },
})

interface ObjectViewerContentProps {
  objectInfo: ObjectInfo
  worldState: Resource<IWorldState>
  objectState?: IObjectState
  spaceContents?: Resource<SpaceContents>
  typeID: string
  component?: ObjectViewerComponent
  availableComponents?: ObjectViewerComponent[]
  missingComponentID?: string
  onSelectComponent?: (component: ObjectViewerComponent) => void
  standalone?: boolean
}

// noSpaceContents stands in for the Space contents of a viewer outside a Space.
const noSpaceContents: Resource<SpaceContents> = {
  value: null,
  loading: false,
  error: null,
  retry: () => {},
}

// pluginsSettled reports whether no Space plugin runtime is still starting, so
// a viewer it registers would already be listed.
function pluginsSettled(state: SpaceContentsState | null): boolean {
  if (!state?.ready) {
    return false
  }
  return !(state.plugins ?? []).some(
    (plugin) =>
      plugin.state ===
      SpacePluginLifecycleState.SpacePluginLifecycleState_LOADING,
  )
}

// MissingViewerCard explains why no viewer is mounted and offers the raw object
// viewer when it is installed. While a Space plugin is still loading, it shows
// the loading state, since that plugin may register the viewer.
function MissingViewerCard({
  objectInfo,
  spaceContents,
  typeID,
  availableComponents,
  missingComponentID,
  onSelectComponent,
}: Pick<
  ObjectViewerContentProps,
  | 'objectInfo'
  | 'spaceContents'
  | 'typeID'
  | 'availableComponents'
  | 'missingComponentID'
  | 'onSelectComponent'
>) {
  const contentsState = useStreamingResource(
    spaceContents ?? noSpaceContents,
    useCallback(
      (contents: SpaceContents, signal: AbortSignal) =>
        contents.watchState({}, signal),
      [],
    ),
    [],
  )
  if (
    spaceContents &&
    !contentsState.error &&
    !pluginsSettled(contentsState.value)
  ) {
    return (
      <ObjectViewerLoadingState
        title="Loading plugins"
        detail="Waiting for the Space plugins that open this object."
      />
    )
  }

  const objectKey = getObjectKey(objectInfo)
  const debugComponent = availableComponents?.find(
    (candidate) => candidate.componentID === debugViewerComponentID,
  )

  let body
  if (!objectKey) {
    body = <span>No object selected</span>
  } else if (!typeID) {
    body = <span>Object has no type</span>
  } else {
    body = (
      <div sx={stylexStyles.rule1}>
        <div sx={stylexStyles.rule2}>
          <h2 sx={stylexStyles.rule3}>Can't open this object yet</h2>
          <p sx={stylexStyles.rule4}>
            No installed viewer handles this object type. You can inspect its
            object record when the raw viewer is available.
          </p>
        </div>
        <InfoCard title="About this object">
          <div sx={stylexStyles.rule5}>
            <CopyableField label="Object key" value={objectKey} />
            <CopyableField label="Object type" value={typeID} />
          </div>
          {missingComponentID && missingComponentID !== typeID ? (
            <p sx={stylexStyles.rule6}>
              Requested viewer unavailable:{' '}
              <span sx={stylexStyles.rule7}>{missingComponentID}</span>
            </p>
          ) : null}
        </InfoCard>
        {debugComponent ? (
          <div sx={stylexStyles.rule8}>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => onSelectComponent?.(debugComponent)}
            >
              Open raw object
            </Button>
          </div>
        ) : (
          <p sx={stylexStyles.rule9}>No raw object viewer is installed.</p>
        )}
      </div>
    )
  }

  return (
    <div sx={stylexStyles.rule10}>
      <div sx={stylexStyles.rule11}>{body}</div>
    </div>
  )
}

// MountedViewer mounts the selected viewer component with the object it shows.
function MountedViewer({
  component,
  standalone,
  ...props
}: ObjectViewerComponentProps & {
  component: ObjectViewerComponent
  standalone?: boolean
}) {
  const Component = component.component
  const disablePadding = standalone || component.disablePadding === true

  return (
    <div sx={[stylexStyles.rule12, !disablePadding && stylexStyles.rule13]}>
      <WebViewErrorBoundary>
        <Suspense fallback={<ObjectViewerLoadingState />}>
          <Component {...props} />
        </Suspense>
      </WebViewErrorBoundary>
    </div>
  )
}

// ObjectViewerContent mounts the selected viewer, or explains that none is
// available for the object.
export function ObjectViewerContent({
  objectInfo,
  worldState,
  objectState,
  spaceContents,
  typeID,
  component,
  availableComponents,
  missingComponentID,
  onSelectComponent,
  standalone,
}: ObjectViewerContentProps) {
  if (!component) {
    return (
      <MissingViewerCard
        objectInfo={objectInfo}
        spaceContents={spaceContents}
        typeID={typeID}
        availableComponents={availableComponents}
        missingComponentID={missingComponentID}
        onSelectComponent={onSelectComponent}
      />
    )
  }

  return (
    <MountedViewer
      component={component}
      standalone={standalone}
      objectInfo={objectInfo}
      worldState={worldState}
      objectState={objectState}
      spaceContents={spaceContents}
    />
  )
}
