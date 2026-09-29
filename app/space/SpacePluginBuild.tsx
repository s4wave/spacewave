import { useId, useMemo, useState } from 'react'
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

function BuildPanel({
  space,
  context,
}: {
  space: Space
  context: SpaceContainerContextValue
}) {
  const id = useId()
  // Selections remain personal; source and build state come from the open World.
  const { spaceWorldResource, spaceState, navigateToObjects } = context
  const objects = spaceState.worldContents?.objects ?? []
  const sources = objects.filter(
    (object) => object.objectType === 'unixfs/fs-node',
  )
  const devices = objects.filter((object) => object.objectType === DeviceTypeID)
  const [sourceKey, setSourceKey] = useState('')
  const [deviceKey, setDeviceKey] = useState('')
  const [manifestId, setManifestId] = useState('')
  const [submitted, setSubmitted] = useState<SubmittedBuild | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [authoring, setAuthoring] = useState(false)

  // The subscription releases on panel close; an immutable build keeps running.
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
  const ready = result?.success === true
  const canBuild =
    sourceKey !== '' &&
    deviceKey !== '' &&
    isValidSpacePluginId(manifestId.trim()) &&
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

  // Use the existing settings layout and a retained source-and-preview dialog.
  const selectClass =
    'border-foreground/10 bg-background w-full rounded-md border p-2 text-xs [@media(pointer:coarse)]:h-11'
  const touchTargetClass = '[@media(pointer:coarse)]:min-h-11'
  return (
    <details className="border-foreground/10 rounded-lg border p-3">
      <summary className="cursor-pointer text-xs font-medium [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:py-3.5">
        Build a TypeScript plugin
      </summary>
      <div className="mt-3 space-y-3">
        <p className="text-foreground-alt/70 text-xs">
          Choose a source folder with bldr.yaml and a registered build device. A
          completed build installs into this Space.
        </p>
        <div className="space-y-1">
          <label htmlFor={`${id}-source`} className="text-xs">
            Source folder
          </label>
          <select
            id={`${id}-source`}
            className={selectClass}
            value={sourceKey}
            onChange={(event) => setSourceKey(event.target.value)}
            disabled={pending || building}
          >
            <option value="">
              {sources.length
                ? 'Choose a source folder'
                : 'Add a source folder to this Space'}
            </option>
            {sources.map((source) => (
              <option key={source.objectKey} value={source.objectKey}>
                {source.objectKey}
              </option>
            ))}
          </select>
          {sourceKey && (
            <Button
              size="sm"
              variant="ghost"
              className={touchTargetClass}
              onClick={() => navigateToObjects([sourceKey])}
            >
              Edit source
            </Button>
          )}
        </div>
        <div className="space-y-1">
          <label htmlFor={`${id}-device`} className="text-xs">
            Build device
          </label>
          <select
            id={`${id}-device`}
            className={selectClass}
            value={deviceKey}
            onChange={(event) => setDeviceKey(event.target.value)}
            disabled={pending || building}
          >
            <option value="">
              {devices.length
                ? 'Choose a build device'
                : 'Register a device in Computers first'}
            </option>
            {devices.map((device) => (
              <option key={device.objectKey} value={device.objectKey}>
                {device.objectKey}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <label htmlFor={`${id}-manifest`} className="text-xs">
            Plugin manifest ID
          </label>
          <Input
            id={`${id}-manifest`}
            value={manifestId}
            onChange={(event) => setManifestId(event.target.value)}
            placeholder="my-colors"
            variant="plugin"
            className={touchTargetClass}
            disabled={pending || building}
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="brandOutline"
            className={touchTargetClass}
            disabled={!canBuild}
            onClick={() => void build()}
          >
            {building ? 'Building…' : 'Build'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className={touchTargetClass}
            disabled={!canBuild}
            onClick={() => setAuthoring(true)}
          >
            Edit with live preview
          </Button>
          {submitted && (
            <Button
              size="sm"
              variant="ghost"
              className={touchTargetClass}
              onClick={() => navigateToObjects([submitted.jobKey])}
            >
              Build logs
            </Button>
          )}
        </div>
        <div role="status" className="text-foreground-alt/70 text-xs">
          {ready
            ? 'Plugin installed. Its status appears in the installed list.'
            : result?.canceled
              ? 'Build canceled. You can build again.'
              : result?.failError ||
                (building
                  ? 'The build continues on your device and installs when it completes.'
                  : '')}
        </div>
        <SpacePluginObjects
          space={space}
          manifestId={request.manifestId}
          onCreated={(key) => navigateToObjects([key])}
        />
        {error && (
          <p role="alert" className="text-destructive text-xs">
            {error}
          </p>
        )}
        {status.error && (
          <div role="alert" className="text-destructive text-xs">
            {status.error.message}
            <Button
              size="sm"
              variant="ghost"
              className={touchTargetClass}
              onClick={status.retry}
            >
              Reconnect to build
            </Button>
          </div>
        )}
      </div>
      <Dialog open={authoring} onOpenChange={setAuthoring}>
        <DialogContent variant="editor">
          <DialogHeader variant="editor">
            <DialogTitle variant="editor">Edit {manifestId.trim()}</DialogTitle>
            <DialogDescription>
              Source changes update this preview. Close it and build to publish
              a new plugin version into the Space.
            </DialogDescription>
          </DialogHeader>
          {authoring && <SpacePluginWorkbench request={request} />}
        </DialogContent>
      </Dialog>
    </details>
  )
}
