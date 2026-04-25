import { useCallback, useMemo } from 'react'
import { LuPlus, LuServer, LuTrash } from 'react-icons/lu'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import type { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

interface ClusterInfo {
  key: string
  name: string
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
    async (world: IWorldState, signal: AbortSignal) => {
      const keys = await listObjectsWithType(world, 'forge/cluster', signal)
      const results: ClusterInfo[] = []
      for (const key of keys) {
        const obj = await world.getObject(key, signal)
        if (!obj) {
          results.push({ key, name: key })
          continue
        }
        try {
          using cursor = await obj.accessWorldState(undefined, signal)
          const resp = await cursor.unmarshal({}, signal)
          if (resp.found && resp.data?.length) {
            const cluster = Cluster.fromBinary(resp.data)
            results.push({ key, name: cluster.name || key })
          } else {
            results.push({ key, name: key })
          }
        } finally {
          obj.release()
        }
      }
      return results
    },
    [],
  )
  const clusters = useMemo(
    () => clustersResource.value ?? [],
    [clustersResource.value],
  )

  const taskDefs = useMemo(() => value.taskDefs ?? [], [value.taskDefs])

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
    onValueChange({ ...value, taskDefs: [...taskDefs, { name: '' }] })
  }, [taskDefs, value, onValueChange])

  const handleRemoveTask = useCallback(
    (index: number) => {
      if (taskDefs.length <= 1) return
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
            <LuServer className="h-3.5 w-3.5" />
            Target Cluster
          </h3>
        </div>
        {clusters.length === 0 && (
          <div className="border-foreground/6 bg-background-card/30 text-foreground-alt/40 flex items-center gap-2 rounded-lg border px-3.5 py-3 text-xs">
            <LuServer className="h-3.5 w-3.5 shrink-0" />
            {clustersResource.loading ?
              'Loading clusters...'
            : 'No clusters found. Create a cluster first.'}
          </div>
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
            >
              <span className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
                <LuServer className="text-foreground-alt/50 h-3.5 w-3.5" />
              </span>
              <span className="text-foreground text-xs font-medium">
                {cluster.name}
              </span>
            </button>
          ))}
        </div>
      </section>

      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuPlus className="h-3.5 w-3.5" />
            Initial Tasks
          </h3>
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddTask}
            className="border-foreground/8 hover:border-foreground/15 hover:bg-foreground/5 text-foreground-alt hover:text-foreground h-7 bg-transparent px-2 text-xs transition-all duration-150"
          >
            <LuPlus className="h-3.5 w-3.5" />
            Add Task
          </Button>
        </div>
        <div className="border-foreground/6 bg-background-card/30 space-y-2 rounded-lg border p-3.5">
          {taskDefs.map((task, i) => (
            <div key={i} className="flex items-center gap-2">
              <Input
                value={task.name ?? ''}
                onChange={(e) => handleUpdateTaskDef(i, e.target.value)}
                placeholder={`Task ${i + 1} name...`}
                className={inputClassName}
              />
              {taskDefs.length > 1 && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleRemoveTask(i)}
                  aria-label={`Remove task ${i + 1}`}
                  className="border-foreground/8 hover:border-destructive/30 hover:bg-destructive/5 hover:text-destructive h-9 bg-transparent px-2 transition-all duration-150"
                >
                  <LuTrash className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
