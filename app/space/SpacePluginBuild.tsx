import { useId, useMemo, useState, type ReactNode } from 'react'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import type { Space } from '@s4wave/sdk/space/space.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import { watchTask } from '@s4wave/sdk/forge/task.js'
import { State } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import {
  SpaceContainerContext,
  type SpaceContainerContextValue,
} from '@s4wave/web/contexts/SpaceContainerContext.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { isValidSpacePluginId } from '@s4wave/core/space/world/world.js'

import { SpacePluginWorkbench } from './SpacePluginWorkbench.js'
import { SpacePluginObjects } from './SpacePluginObjects.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { layout as themeLayout2 } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout2['--spacing']} * 3)` },
})

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    backgroundColor: themeColors['--color-background'],
    padding: `calc(${themeLayout2['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    height: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    padding: `calc(${themeLayout2['--spacing']} * 3)`,
  },
  rule4: {
    cursor: 'pointer',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 3.5)`,
    },
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule6: {
    display: 'flex',
    flexWrap: 'wrap',
    gap: `calc(${themeLayout2['--spacing']} * 2)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout2['--spacing']} * 11)`,
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
  },
})

interface SubmittedBuild {
  jobKey: string
  taskKey: string
}

// The build asks its Worker for this much capacity: one core and 2 GiB.
const BUILD_MILLI_CPU = 1000n
const BUILD_MEMORY_BYTES = 2n << 30n

/** SpacePluginBuild uses the open Space's inventory and existing Forge execution. */
export function SpacePluginBuild({
  space,
}: {
  space: Space | null | undefined
}) {
  const context = SpaceContainerContext.useContextSafe()
  if (!space || !context) return null
  return <BuildPanel key={space.id} space={space} context={context} />
}

interface BuildSelections {
  sourceKey: string
  deviceKey: string
  manifestId: string
}

/** useBuildChoices lists the Space's source folders and build devices. */
function useBuildChoices(
  spaceWorldResource: SpaceContainerContextValue['spaceWorldResource'],
) {
  const choices = useWorldQuery(
    spaceWorldResource,
    async (world, signal) => {
      const [sources, devices] = await Promise.all([
        world.listObjectsWithType('unixfs/fs-node', signal),
        world.listObjectsWithType(DeviceTypeID, signal),
      ])
      return { sources, devices }
    },
    [],
  ).value

  return { sources: choices?.sources ?? [], devices: choices?.devices ?? [] }
}

/**
 * useSpacePluginBuild queues a pinned build for the selections and watches its
 * task. The watch releases on panel close; an immutable build keeps running.
 */
function useSpacePluginBuild(
  space: Space,
  spaceWorldResource: SpaceContainerContextValue['spaceWorldResource'],
  { sourceKey, deviceKey, manifestId }: BuildSelections,
) {
  const [submitted, setSubmitted] = useState<SubmittedBuild | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  const status = useStreamingResource(
    spaceWorldResource,
    async function* (world, signal) {
      if (submitted) {
        yield* watchTask(world, submitted.taskKey, signal)
      }
    },
    [submitted],
  )
  const request = useMemo(
    () => ({ sourceKey, deviceKey, manifestId: manifestId.trim() }),
    [sourceKey, deviceKey, manifestId],
  )

  // Show only the current submission's state while a new watch starts.
  const task = status.loading ? undefined : status.value
  const result = task?.result
  const building =
    submitted != null && task?.taskState !== State.TaskState_COMPLETE
  const canBuild =
    sourceKey !== '' &&
    deviceKey !== '' &&
    isValidSpacePluginId(request.manifestId) &&
    !pending &&
    !building

  // Queue a pinned build without tying its lifetime to this panel.
  async function build() {
    if (!canBuild) return
    setPending(true)
    setError('')
    try {
      const response = await space.buildSpacePlugin({
        ...request,
        milliCpu: BUILD_MILLI_CPU,
        memoryBytes: BUILD_MEMORY_BYTES,
      })
      if (!response.jobKey || !response.taskKey) {
        throw new Error('Build returned no job')
      }
      setSubmitted({
        jobKey: response.jobKey,
        taskKey: response.taskKey,
      })
    } catch (cause) {
      setError(String(cause))
    } finally {
      setPending(false)
    }
  }

  return {
    request,
    submitted,
    pending,
    error,
    status,
    building,
    canBuild,
    build,
    message: buildMessage(result, building),
  }
}

/** buildMessage returns the status line for the build result. */
function buildMessage(
  result:
    | { success?: boolean; canceled?: boolean; failError?: string }
    | undefined,
  building: boolean,
): string {
  if (result?.success === true) {
    return 'Plugin installed. Its status appears in the installed list.'
  }
  if (result?.canceled) return 'Build canceled. You can build again.'
  if (result?.failError) return result.failError
  if (building) {
    return 'The build continues on your device and installs when it completes.'
  }
  return ''
}

interface BuildSelectProps {
  label: string
  value: string
  options: string[]
  placeholder: string
  disabled: boolean
  onChange: (value: string) => void
  children?: (childSx: stylex.StyleXStyles) => ReactNode
}

/** BuildSelect is a labeled select over object keys, with optional trailing actions. */
function BuildSelect({
  sx,
  label,
  value,
  options,
  placeholder,
  disabled,
  onChange,
  children,
}: BuildSelectProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const id = useId()

  return (
    <div sx={[false, sx]}>
      <label htmlFor={id} sx={[slotChildren.child, stylexStyles3.rule1]}>
        {label}
      </label>
      <select
        id={id}
        sx={[slotChildren.child, stylexStyles3.rule2]}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
      >
        <option value="">{placeholder}</option>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
      {children?.(slotChildren.child)}
    </div>
  )
}

/** BuildAuthoringDialog hosts the source-and-preview workbench for the manifest. */
function BuildAuthoringDialog({
  open,
  onOpenChange,
  request,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  request: BuildSelections
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent variant="editor">
        <DialogHeader variant="editor">
          <DialogTitle variant="editor">Edit {request.manifestId}</DialogTitle>
          <DialogDescription>
            Source changes update this preview. Close it and build to publish a
            new plugin version into the Space.
          </DialogDescription>
        </DialogHeader>
        {open && <SpacePluginWorkbench request={request} />}
      </DialogContent>
    </Dialog>
  )
}

function BuildPanel({
  space,
  context,
}: {
  space: Space
  context: SpaceContainerContextValue
}) {
  const id = useId()
  // Selections remain personal; source and build state come from the open World.
  const { spaceWorldResource, navigateToObjects } = context
  const { sources, devices } = useBuildChoices(spaceWorldResource)
  const [sourceKey, setSourceKey] = useState('')
  const [deviceKey, setDeviceKey] = useState('')
  const [manifestId, setManifestId] = useState('')
  const [authoring, setAuthoring] = useState(false)
  const {
    request,
    submitted,
    pending,
    error,
    status,
    building,
    canBuild,
    build,
    message,
  } = useSpacePluginBuild(space, spaceWorldResource, {
    sourceKey,
    deviceKey,
    manifestId,
  })
  const locked = pending || building

  // Use the existing settings layout and a retained source-and-preview dialog.
  return (
    <details sx={stylexStyles3.rule3}>
      <summary sx={stylexStyles3.rule4}>Build a TypeScript plugin</summary>
      <div sx={stylexStyles4.rule1}>
        <p sx={[nativeChildSpacingNext.s3, stylexStyles3.rule5]}>
          Choose a source folder with bldr.yaml and a registered build device. A
          completed build installs into this Space.
        </p>
        <BuildSelect
          sx={nativeChildSpacingNext.s3}
          label="Source folder"
          value={sourceKey}
          options={sources}
          placeholder={
            sources.length
              ? 'Choose a source folder'
              : 'Add a source folder to this Space'
          }
          disabled={locked}
          onChange={setSourceKey}
        >
          {(childSx) => (
            <>
              {sourceKey && (
                <Button
                  size="sm"
                  variant="ghost"
                  sx={[childSx, stylexStyles2.rule1]}
                  onClick={() => navigateToObjects([sourceKey])}
                >
                  Edit source
                </Button>
              )}
            </>
          )}
        </BuildSelect>
        <BuildSelect
          sx={nativeChildSpacingNext.s3}
          label="Build device"
          value={deviceKey}
          options={devices}
          placeholder={
            devices.length
              ? 'Choose a build device'
              : 'Register a device in Computers first'
          }
          disabled={locked}
          onChange={setDeviceKey}
        />
        <div sx={nativeChildSpacingNext.s3}>
          <label
            htmlFor={`${id}-manifest`}
            sx={[nativeChildSpacing.s1, stylexStyles3.rule1]}
          >
            Plugin manifest ID
          </label>
          <Input
            id={`${id}-manifest`}
            value={manifestId}
            onChange={(event) => setManifestId(event.target.value)}
            placeholder="my-colors"
            variant="plugin"
            sx={[nativeChildSpacing.s1, stylexStyles.rule1]}
            disabled={locked}
          />
        </div>
        <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule6]}>
          <Button
            size="sm"
            variant="brandOutline"
            sx={stylexStyles2.rule1}
            disabled={!canBuild}
            onClick={() => void build()}
          >
            {building ? 'Building…' : 'Build'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            sx={stylexStyles2.rule1}
            disabled={!canBuild}
            onClick={() => setAuthoring(true)}
          >
            Edit with live preview
          </Button>
          {submitted && (
            <Button
              size="sm"
              variant="ghost"
              sx={stylexStyles2.rule1}
              onClick={() => navigateToObjects([submitted.jobKey])}
            >
              Build logs
            </Button>
          )}
        </div>
        <div
          role="status"
          sx={[nativeChildSpacingNext.s3, stylexStyles3.rule5]}
        >
          {message}
        </div>
        <SpacePluginObjects
          sx={nativeChildSpacingNext.s3}
          space={space}
          manifestId={request.manifestId}
          onCreated={(key) => navigateToObjects([key])}
        />
        {error && (
          <p role="alert" sx={[nativeChildSpacingNext.s3, stylexStyles3.rule7]}>
            {error}
          </p>
        )}
        {status.error && (
          <div
            role="alert"
            sx={[nativeChildSpacingNext.s3, stylexStyles3.rule7]}
          >
            {status.error.message}
            <Button
              size="sm"
              variant="ghost"
              sx={stylexStyles2.rule1}
              onClick={status.retry}
            >
              Reconnect to build
            </Button>
          </div>
        )}
      </div>
      <BuildAuthoringDialog
        open={authoring}
        onOpenChange={setAuthoring}
        request={request}
      />
    </details>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// slotChildren keeps the optional action in the original physical child sequence.
const slotChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 1 * var(--tw-space-y-reverse))',
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))',
    },
  },
})
