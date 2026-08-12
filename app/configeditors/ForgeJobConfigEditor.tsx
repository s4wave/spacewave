import { useCallback, useMemo, useRef } from 'react'
import { LuCheck, LuPlus, LuServer, LuTrash } from 'react-icons/lu'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

export interface ForgeClusterOption {
  key: string
  name: string
}

export async function loadForgeClusterOptions(
  world: IWorldState,
  signal: AbortSignal,
): Promise<ForgeClusterOption[]> {
  const keys = await listObjectsWithType(world, 'forge/cluster', signal)
  return Promise.all(
    keys.map(async (key) => {
      const obj = await world.getObject(key, signal)
      if (!obj) return { key, name: key }
      try {
        using cursor = await obj.accessWorldState(undefined, signal)
        const resp = await cursor.unmarshal(
          { blockType: 'forge/cluster' },
          signal,
        )
        if (!resp.found || !resp.data?.length) return { key, name: key }
        return { key, name: Cluster.fromBinary(resp.data).name || key }
      } finally {
        obj.release()
      }
    }),
  )
}

const inputClassName =
  'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9'

// ForgeJobConfigEditor edits the config-specific fields of a ForgeJobCreateOp.
// Renders a cluster picker and task definitions list.
export function ForgeJobConfigEditor({
  value,
  onValueChange,
}: ConfigEditorProps<ForgeJobCreateOp>) {
  const { spaceWorldResource } = SpaceContainerContext.useContext()

  const clustersResource = useResource(
    spaceWorldResource,
    (world: IWorldState, signal: AbortSignal) =>
      loadForgeClusterOptions(world, signal),
    [],
  )
  const clusters = useMemo(
    () => clustersResource.value ?? [],
    [clustersResource.value],
  )

  const taskDefs = useMemo(() => value.taskDefs ?? [], [value.taskDefs])
  const displayedTaskDefs = useMemo(
    () => (taskDefs.length > 0 ? taskDefs : [{ name: '' }]),
    [taskDefs],
  )
  const nextTaskKeyRef = useRef(1)
  const taskKeysRef = useRef<string[]>([])
  while (taskKeysRef.current.length < displayedTaskDefs.length) {
    taskKeysRef.current.push(`task-${nextTaskKeyRef.current++}`)
  }
  taskKeysRef.current.length = displayedTaskDefs.length
  const handleSelectCluster = useCallback(
    (clusterKey: string) => {
      onValueChange({ ...value, clusterKey })
    },
    [value, onValueChange],
  )

  const handleUpdateTaskDef = useCallback(
    (index: number, name: string) => {
      const next = [...taskDefs]
      next[index] = { ...next[index], name }
      onValueChange({ ...value, taskDefs: next })
    },
    [taskDefs, value, onValueChange],
  )

  const handleAddTask = useCallback(() => {
    taskKeysRef.current.push(`task-${nextTaskKeyRef.current++}`)
    onValueChange({ ...value, taskDefs: [...taskDefs, { name: '' }] })
  }, [taskDefs, value, onValueChange])

  const handleRemoveTask = useCallback(
    (index: number) => {
      if (taskDefs.length <= 1) return
      taskKeysRef.current.splice(index, 1)
      onValueChange({
        ...value,
        taskDefs: taskDefs.filter((_, i) => i !== index),
      })
    },
    [taskDefs, value, onValueChange],
  )

  return (
    <div className="space-y-3">
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuServer className="size-3.5" />
            Target Cluster
          </h3>
        </div>
        {clustersResource.error ? (
          <LoadingCard
            view={{
              state: 'error',
              title: 'Clusters unavailable',
              detail: 'Forge could not load the available clusters.',
              error: 'Try again to choose where this Job will run.',
              onRetry: clustersResource.retry,
            }}
          />
        ) : clustersResource.loading && clusters.length === 0 ? (
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Loading clusters…',
              detail: 'Reading the available Forge Clusters.',
              progressIndeterminate: true,
            }}
          />
        ) : clusters.length === 0 ? (
          <div className="border-foreground/6 bg-background-card/30 text-foreground-alt/60 rounded-lg border px-3.5 py-3 text-xs leading-relaxed">
            No Clusters are available. Create a Cluster before creating a Job.
          </div>
        ) : (
          <p className="text-foreground-alt/60 mb-2 text-xs leading-relaxed">
            Choose the Cluster that will schedule this Job.
          </p>
        )}
        <div className="space-y-2">
          {clusters.map((cluster) => (
            <button
              type="button"
              key={cluster.key}
              className={cn(
                'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
                value.clusterKey === cluster.key &&
                  'border-brand/30 bg-brand/5',
              )}
              onClick={() => handleSelectCluster(cluster.key)}
              aria-pressed={value.clusterKey === cluster.key}
            >
              <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
                <LuServer className="text-foreground-alt/50 size-3.5" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="text-foreground block truncate text-xs font-medium">
                  {cluster.name}
                </span>
                <span className="text-foreground-alt/50 block truncate text-xs">
                  {cluster.key}
                </span>
              </span>
              {value.clusterKey === cluster.key && (
                <LuCheck
                  className="text-brand size-4 shrink-0"
                  aria-label="Selected"
                />
              )}
            </button>
          ))}
        </div>
      </section>

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuPlus className="size-3.5" />
            Initial Tasks
          </h3>
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddTask}
            className="border-foreground/8 hover:border-foreground/15 hover:bg-foreground/5 text-foreground-alt hover:text-foreground h-7 bg-transparent px-2 text-xs transition duration-150"
          >
            <LuPlus className="size-3.5" />
            Add Task
          </Button>
        </div>
        <p className="text-foreground-alt/60 mb-2 text-xs leading-relaxed">
          Add at least one named task. Tasks run in the order shown.
        </p>
        <div className="border-foreground/6 bg-background-card/30 space-y-2 rounded-lg border p-3.5">
          {displayedTaskDefs.map((task, i) => (
            <div
              key={taskKeysRef.current[i]}
              className="flex items-center gap-2"
            >
              <Input
                value={task.name ?? ''}
                onChange={(e) => handleUpdateTaskDef(i, e.target.value)}
                placeholder={`Task ${i + 1} name...`}
                aria-label={`Task ${i + 1} name`}
                className={inputClassName}
              />
              {displayedTaskDefs.length > 1 && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleRemoveTask(i)}
                  aria-label={`Remove task ${i + 1}`}
                  className="border-foreground/8 hover:border-destructive/30 hover:bg-destructive/5 hover:text-destructive h-9 bg-transparent px-2 transition duration-150"
                >
                  <LuTrash className="size-3.5" />
                </Button>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
