import { useCallback, useId, useState } from 'react'
import { LuArrowLeft, LuGitBranch } from 'react-icons/lu'
import {
  useResource,
  type Resource,
} from '@aptre/bldr-sdk/hooks/useResource.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'

import {
  State,
  type Task,
} from '@go/github.com/s4wave/spacewave/forge/task/task.pb.js'
import { watchTask } from '@s4wave/sdk/forge/task.js'
import { Space } from '@s4wave/sdk/space/space.js'
import type { ValidatePluginRepositoryResponse } from '@s4wave/sdk/space/space.pb.js'
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
import { PluginRepositorySheet } from './PluginRepositorySheet.js'

// A build asks its Worker for this much capacity: one core and 2 GiB.
const BUILD_MILLI_CPU = 1000n
const BUILD_MEMORY_BYTES = 2n << 30n

/**
 * PluginRepositoriesPage adds GitHub repositories to the account's developer
 * Space, checks them for newer commits, and builds a reviewed commit. Each
 * fetch and build runs as a Forge Job on a Device of that Space; the list
 * re-reads its commits on each World revision.
 */
export function PluginRepositoriesPage() {
  // Read the developer Space's Devices and repositories on each revision.
  const navigate = useNavigate()
  const { spaceResource, worldResource } = useDeveloperSpace()
  const inventory = useWorldQuery(worldResource, readPluginRepositories, [])
  const devices = inventory.value?.devices ?? []
  const repositories = inventory.value?.repositories ?? []

  // Fetch and build on the chosen Device, or the only one.
  const [deviceKey, setDeviceKey] = useState('')
  const device = deviceKey || (devices[0] ?? '')
  const jobs = usePluginRepositoryJobs(
    spaceResource.value,
    worldResource,
    device,
  )
  const canFetch = spaceResource.value != null && device !== '' && !jobs.busy

  // Review a repository's checked-out commit before building it.
  const review = usePluginRepositoryReview(spaceResource.value)
  const message = review.error || jobs.message
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
              disabled={jobs.busy}
              onChange={setDeviceKey}
            />
          )}
          <AddRepositoryForm
            disabled={!canFetch}
            busy={jobs.busy}
            onAdd={(repository) => void jobs.fetchRepository(repository)}
          />
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
                  disabled={!canFetch || review.pending}
                  onCheck={() => void jobs.fetchRepository(repo.name)}
                  onReview={() => void review.open(repo.name)}
                />
              ))}
            </ul>
          )}
        </div>
      </div>
      <ReviewSheet
        review={review}
        repositories={repositories}
        device={device}
        busy={!canFetch}
        onBuild={(reviewed) => void jobs.buildRepository(reviewed)}
      />
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

/**
 * ReviewSheet opens the review sheet for the reviewed repository and marks the
 * review stale once the repository's pinned commit moves.
 */
function ReviewSheet({
  review,
  repositories,
  device,
  busy,
  onBuild,
}: {
  review: ReturnType<typeof usePluginRepositoryReview>
  repositories: PluginRepository[]
  device: string
  busy: boolean
  onBuild: (review: Review) => void
}) {
  const reviewed = review.value
  if (!reviewed) return null
  const pinned = repositories.find((repo) => repo.name === reviewed.repository)

  return (
    <PluginRepositorySheet
      repository={reviewed.repository}
      review={reviewed.response}
      device={device}
      stale={pinned?.pinnedCommit !== reviewed.response.commit}
      busy={busy}
      onConfirm={() => {
        onBuild(reviewed)
        review.close()
      }}
      onClose={review.close}
    />
  )
}

// Review is a repository's validated checked-out commit.
interface Review {
  repository: string
  response: ValidatePluginRepositoryResponse
}

/** usePluginRepositoryReview validates a repository for the review sheet. */
function usePluginRepositoryReview(space: Space | null | undefined) {
  const [value, setValue] = useState<Review | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')

  // Validate the checked-out commit and open the sheet with the result.
  const open = useCallback(
    async (repository: string) => {
      if (!space) return
      setPending(true)
      setError('')
      try {
        const response = await space.validatePluginRepository({ repository })
        setValue({ repository, response })
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        setPending(false)
      }
    },
    [space],
  )
  const close = useCallback(() => setValue(null), [])

  return { value, pending, error, open, close }
}

// Submitted is the queued Jobs' Tasks the page watches, in order.
interface Submitted {
  action: 'fetch' | 'build'
  repository: string
  taskKeys: string[]
}

// WatchedTask is the newest state of one submitted Task.
interface WatchedTask {
  task: Task
  last: boolean
}

/**
 * usePluginRepositoryJobs queues a clone, a fetch of the newest commit, or the
 * builds of a reviewed commit, and watches their Tasks until they complete.
 */
function usePluginRepositoryJobs(
  space: Space | null | undefined,
  worldResource: Resource<IWorldState>,
  deviceKey: string,
) {
  // Watch the submitted Tasks in order until one fails or all complete.
  const [submitted, setSubmitted] = useState<Submitted | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const status = useStreamingResource(
    worldResource,
    async function* (world, signal) {
      if (submitted) yield* watchTasks(world, submitted.taskKeys, signal)
    },
    [submitted],
  )
  const watched = status.loading ? undefined : status.value
  const complete = watched?.task.taskState === State.TaskState_COMPLETE
  const failError = complete ? watched?.task.result?.failError : undefined
  const running =
    submitted != null && !(complete && (watched?.last || failError))

  // Queue Jobs through submit; the Task watch above reports their progress.
  const submit = useCallback(
    async (
      action: Submitted['action'],
      repository: string,
      queue: (space: Space) => Promise<string[]>,
    ) => {
      if (!space) return
      setPending(true)
      setError('')
      try {
        setSubmitted({ action, repository, taskKeys: await queue(space) })
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        setPending(false)
      }
    },
    [space],
  )

  // Fetch the repository's newest commit, or clone it on first add.
  const fetchRepository = useCallback(
    (repository: string) =>
      submit('fetch', repository, async (space) => {
        const response = await space.fetchPluginRepository({
          repository,
          deviceKey,
        })
        return [response.taskKey ?? '']
      }),
    [submit, deviceKey],
  )

  // Build each plugin of the reviewed commit.
  const buildRepository = useCallback(
    ({ repository, response }: Review) =>
      submit('build', repository, (space) =>
        Promise.all(
          (response.validation?.plugins ?? []).map(async (plugin) => {
            const build = await space.buildSpacePlugin({
              sourceKey: response.sourceKey,
              commit: response.commit,
              manifestId: plugin.manifestId,
              deviceKey,
              milliCpu: BUILD_MILLI_CPU,
              memoryBytes: BUILD_MEMORY_BYTES,
            })
            return build.taskKey ?? ''
          }),
        ),
      ),
    [submit, deviceKey],
  )

  return {
    busy: pending || running,
    message:
      error || jobMessage(submitted, running, failError, complete && !running),
    fetchRepository,
    buildRepository,
  }
}

/** watchTasks follows each Task to completion and stops at a failure. */
async function* watchTasks(
  world: IWorldState,
  taskKeys: string[],
  signal: AbortSignal,
): AsyncGenerator<WatchedTask> {
  for (const [index, key] of taskKeys.entries()) {
    const last = index === taskKeys.length - 1
    let task: Task | undefined
    for await (task of watchTask(world, key, signal)) yield { task, last }
    if (task?.result?.failError) return
  }
}

/** jobMessage describes the submitted Jobs' progress or result. */
function jobMessage(
  submitted: Submitted | null,
  running: boolean,
  failError: string | undefined,
  succeeded: boolean,
): string {
  // Prefer progress over a stale result while a Task runs.
  if (!submitted) return ''
  const { action, repository } = submitted
  if (running) {
    const verb = action === 'fetch' ? 'Fetching' : 'Building'
    return `${verb} ${repository} on your device.`
  }
  if (failError) return failError
  if (succeeded)
    return `${action === 'fetch' ? 'Fetched' : 'Built'} ${repository}.`
  return ''
}

/** AddRepositoryForm takes the owner/repo of a repository to add. */
function AddRepositoryForm({
  disabled,
  busy,
  onAdd,
}: {
  disabled: boolean
  busy: boolean
  onAdd: (repository: string) => void
}) {
  const id = useId()
  const [repository, setRepository] = useState('')
  const trimmed = repository.trim()

  return (
    <form
      className="space-y-1"
      onSubmit={(event) => {
        event.preventDefault()
        if (!disabled && trimmed !== '') onAdd(trimmed)
      }}
    >
      <label htmlFor={id} className="text-xs">
        Add from GitHub
      </label>
      <div className="flex gap-2">
        <Input
          id={id}
          value={repository}
          onChange={(event) => setRepository(event.target.value)}
          placeholder="owner/repo"
          disabled={busy}
        />
        <Button type="submit" size="sm" disabled={disabled || trimmed === ''}>
          Add
        </Button>
      </div>
    </form>
  )
}

/** DeviceSelect picks the Device that runs fetches and builds. */
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

/**
 * PluginRepositoryRow shows one repository's pinned and fetched commits, and
 * offers a review of the pinned commit before it builds.
 */
function PluginRepositoryRow({
  repository,
  disabled,
  onCheck,
  onReview,
}: {
  repository: PluginRepository
  disabled: boolean
  onCheck: () => void
  onReview: () => void
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
      <Button size="sm" variant="ghost" disabled={disabled} onClick={onReview}>
        Review
      </Button>
    </li>
  )
}
