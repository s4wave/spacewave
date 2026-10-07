import { useCallback, useId, useState } from 'react'
import { LuArrowLeft, LuGitBranch } from 'react-icons/lu'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import { State } from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { watchTask } from '@s4wave/sdk/forge/task.js'
import { Space } from '@s4wave/sdk/space/space.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'

import {
  readPluginRepositories,
  type PluginRepository,
} from './plugin-repositories.js'

/**
 * PluginRepositoriesPage adds GitHub repositories to the account's developer
 * Space and checks them for newer commits. Each fetch runs as a Forge Job on a
 * Device of that Space; the list re-reads its commits on each World revision.
 */
export function PluginRepositoriesPage() {
  // Read the developer Space's Devices and repositories on each revision.
  const navigate = useNavigate()
  const id = useId()
  const { spaceResource, worldResource } = useDeveloperSpace()
  const inventory = useWorldQuery(worldResource, readPluginRepositories, [])
  const devices = inventory.value?.devices ?? []
  const repositories = inventory.value?.repositories ?? []

  // Fetch on the chosen Device, or the only one.
  const [repository, setRepository] = useState('')
  const [deviceKey, setDeviceKey] = useState('')
  const device = deviceKey || (devices[0] ?? '')
  const { busy, message, fetchRepository } = usePluginRepositoryFetch(
    spaceResource.value,
    worldResource,
    device,
  )
  const canFetch = spaceResource.value != null && device !== '' && !busy
  const loadError = spaceResource.error ?? inventory.error

  return (
    <div className="bg-background-landing flex flex-1 flex-col overflow-y-auto p-6 md:p-10">
      <div className="mx-auto w-full max-w-lg">
        <button
          type="button"
          onClick={() => navigate({ path: '../../' })}
          className="text-foreground-alt hover:text-foreground mb-6 flex items-center gap-1.5 text-sm transition-colors"
        >
          <LuArrowLeft className="size-4" />
          Back to dashboard
        </button>

        <div className="mb-6">
          <h1 className="text-foreground text-lg font-semibold tracking-wide">
            Plugins
          </h1>
          <p className="text-foreground-alt mt-1 text-sm">
            Add a plugin from GitHub. Spacewave stores its newest commit in your
            developer Space and keeps it pinned until you update.
          </p>
        </div>

        <div className="border-foreground/20 bg-background-get-started space-y-4 rounded-lg border p-6 shadow-lg backdrop-blur-sm">
          {loadError && (
            <p role="alert" className="text-destructive text-xs">
              {loadError.message}
            </p>
          )}
          {inventory.value && devices.length === 0 && (
            <p className="text-foreground-alt text-xs">
              Fetching needs a desktop or command-line Device linked to your
              developer Space. Link one, then add the repository.
            </p>
          )}
          {devices.length > 1 && (
            <DeviceSelect
              devices={devices}
              value={device}
              disabled={busy}
              onChange={setDeviceKey}
            />
          )}
          <form
            className="space-y-1"
            onSubmit={(event) => {
              event.preventDefault()
              if (canFetch) void fetchRepository(repository.trim())
            }}
          >
            <label htmlFor={`${id}-repository`} className="text-xs">
              Add from GitHub
            </label>
            <div className="flex gap-2">
              <Input
                id={`${id}-repository`}
                value={repository}
                onChange={(event) => setRepository(event.target.value)}
                placeholder="owner/repo"
                disabled={busy}
              />
              <Button
                type="submit"
                size="sm"
                disabled={!canFetch || repository.trim() === ''}
              >
                Add
              </Button>
            </div>
          </form>
          {message && (
            <p role="status" className="text-foreground-alt text-xs">
              {message}
            </p>
          )}
          {repositories.length > 0 && (
            <ul className="divide-foreground/10 divide-y">
              {repositories.map((repo) => (
                <PluginRepositoryRow
                  key={repo.name}
                  repository={repo}
                  disabled={!canFetch}
                  onCheck={() => void fetchRepository(repo.name)}
                />
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  )
}

/** useDeveloperSpace mounts the account's developer Space and its World. */
function useDeveloperSpace() {
  const sessionResource = SessionContext.useContext()
  const spaceResource = useResource(
    sessionResource,
    async (session, signal, cleanup) => {
      // Find or create the developer Space, then mount its body.
      if (!session) return null
      const { sharedObjectId } = await session.ensureDeveloperSpace(signal)
      const so = cleanup(
        await session.mountSharedObject({ sharedObjectId }, signal),
      )
      const body = cleanup(await so.mountSharedObjectBody({}, signal))
      return cleanup(new Space(body.resourceRef.createRef(body.id)))
    },
    [],
  )
  const worldResource = useResource(
    spaceResource,
    async (space, signal, cleanup) =>
      space ? cleanup(await space.accessWorldState(true, signal)) : null,
    [],
  )
  return { spaceResource, worldResource }
}

// SubmittedFetch is the fetch Job whose Task the page watches.
interface SubmittedFetch {
  repository: string
  taskKey: string
}

/**
 * usePluginRepositoryFetch queues a clone, or a fetch of the newest commit
 * when the Space has the repository, and watches its Task until it completes.
 */
function usePluginRepositoryFetch(
  space: Space | null | undefined,
  worldResource: Resource<IWorldState>,
  deviceKey: string,
) {
  // Watch the submitted fetch's Task until it completes.
  const [submitted, setSubmitted] = useState<SubmittedFetch | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const status = useStreamingResource(
    worldResource,
    async function* (world, signal) {
      if (submitted) yield* watchTask(world, submitted.taskKey, signal)
    },
    [submitted],
  )
  const task = status.loading ? undefined : status.value
  const fetching =
    submitted != null && task?.taskState !== State.TaskState_COMPLETE

  // Queue the Job; the Task watch above reports its progress.
  const fetchRepository = useCallback(
    async (repository: string) => {
      if (!space) return
      setPending(true)
      setError('')
      try {
        const response = await space.fetchPluginRepository({
          repository,
          deviceKey,
        })
        setSubmitted({ repository, taskKey: response.taskKey ?? '' })
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        setPending(false)
      }
    },
    [space, deviceKey],
  )

  return {
    busy: pending || fetching,
    message: error || fetchMessage(submitted, fetching, task?.result),
    fetchRepository,
  }
}

/** fetchMessage describes the submitted fetch's progress or result. */
function fetchMessage(
  submitted: SubmittedFetch | null,
  fetching: boolean,
  result: { success?: boolean; failError?: string } | undefined,
): string {
  // Prefer progress over a stale result while the Task runs.
  if (!submitted) return ''
  if (fetching) return `Fetching ${submitted.repository} on your device.`
  if (result?.failError) return result.failError
  if (result?.success) return `Fetched ${submitted.repository}.`
  return ''
}

/** DeviceSelect picks the Device that runs the fetch. */
function DeviceSelect({
  devices,
  value,
  disabled,
  onChange,
}: {
  devices: string[]
  value: string
  disabled: boolean
  onChange: (value: string) => void
}) {
  const id = useId()

  return (
    <div className="space-y-1">
      <label htmlFor={id} className="text-xs">
        Device
      </label>
      <select
        id={id}
        className="border-foreground/10 bg-background w-full rounded-md border p-2 text-xs"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
      >
        {devices.map((key) => (
          <option key={key} value={key}>
            {key}
          </option>
        ))}
      </select>
    </div>
  )
}

/** shortCommit abbreviates a commit hash for display. */
function shortCommit(hash: string): string {
  return hash.slice(0, 7)
}

/** PluginRepositoryRow shows one repository's pinned and fetched commits. */
function PluginRepositoryRow({
  repository,
  disabled,
  onCheck,
}: {
  repository: PluginRepository
  disabled: boolean
  onCheck: () => void
}) {
  const { name, pinnedCommit, fetchedCommit } = repository
  const update = fetchedCommit !== '' && fetchedCommit !== pinnedCommit

  return (
    <li className="flex items-center gap-3 py-3">
      <LuGitBranch className="text-foreground-alt size-4 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="text-foreground truncate text-sm font-medium">{name}</p>
        <p className="text-foreground-alt text-xs">
          Pinned at {shortCommit(pinnedCommit)}
          {update
            ? ` · Update available: ${shortCommit(fetchedCommit)}`
            : ' · Up to date'}
        </p>
      </div>
      <Button size="sm" variant="ghost" disabled={disabled} onClick={onCheck}>
        Check for updates
      </Button>
    </li>
  )
}
