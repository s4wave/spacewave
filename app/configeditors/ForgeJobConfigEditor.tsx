import { useCallback, useMemo, useState } from 'react'
import { LuCheck, LuPlus, LuServer, LuTrash } from '@s4wave/web/images/icons.js'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'

import { Button } from '@s4wave/web/ui/button.js'
import { Input } from '@s4wave/web/ui/input.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import type { ForgeJobCreateOp } from '@s4wave/core/forge/job/job.pb.js'
import { Cluster } from '@go/github.com/s4wave/spacewave/forge/cluster/cluster.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  rule2: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3.5)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule5: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule6: {
    display: 'flex',
    width: '100%',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule7: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule8: { minWidth: 0, flex: 1 },
  rule9: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule10: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule11: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    color: themeColors['--color-brand'],
  },
})

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

let nextTaskKey = 0

// newTaskKey returns a row key no other task row has used.
function newTaskKey(): string {
  return `task-${++nextTaskKey}`
}

// fitTaskKeys returns keys trimmed or extended to one per displayed task.
function fitTaskKeys(keys: string[], count: number): string[] {
  if (keys.length >= count) {
    return keys.slice(0, count)
  }
  return [...keys, ...Array.from({ length: count - keys.length }, newTaskKey)]
}

// ForgeJobConfigEditor edits the config-specific fields of a ForgeJobCreateOp.
// Renders a cluster picker and task definitions list.
export function ForgeJobConfigEditor({
  sx,
  value,
  onValueChange,
}: ConfigEditorProps<ForgeJobCreateOp> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
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

  // Row keys follow their tasks across additions and removals. A task list
  // replaced by the parent resizes the keys during render.
  const [taskKeys, setTaskKeys] = useState(() =>
    fitTaskKeys([], displayedTaskDefs.length),
  )
  if (taskKeys.length !== displayedTaskDefs.length) {
    setTaskKeys(fitTaskKeys(taskKeys, displayedTaskDefs.length))
  }

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
    setTaskKeys((keys) => [...keys, newTaskKey()])
    onValueChange({ ...value, taskDefs: [...taskDefs, { name: '' }] })
  }, [taskDefs, value, onValueChange])

  const handleRemoveTask = useCallback(
    (index: number) => {
      if (taskDefs.length <= 1) return
      setTaskKeys((keys) => keys.filter((_, i) => i !== index))
      onValueChange({
        ...value,
        taskDefs: taskDefs.filter((_, i) => i !== index),
      })
    },
    [taskDefs, value, onValueChange],
  )

  return (
    <div sx={[false, sx]}>
      <section sx={nativeChildSpacing.s3}>
        <div sx={stylexStyles2.rule1}>
          <h3 sx={stylexStyles2.rule2}>
            <LuServer sx={stylexStyles.rule1} />
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
          <div sx={stylexStyles2.rule3}>
            No Clusters are available. Create a Cluster before creating a Job.
          </div>
        ) : (
          <p sx={stylexStyles2.rule4}>
            Choose the Cluster that will schedule this Job.
          </p>
        )}
        <div sx={false}>
          {clusters.map((cluster) => (
            <button
              type="button"
              key={cluster.key}
              sx={[
                nativeChildSpacing.s2,
                value.clusterKey === cluster.key
                  ? stylexStyles2.rule5
                  : stylexStyles2.rule6,
              ]}
              onClick={() => handleSelectCluster(cluster.key)}
              aria-pressed={value.clusterKey === cluster.key}
            >
              <span sx={stylexStyles2.rule7}>
                <LuServer sx={stylexStyles.rule2} />
              </span>
              <span sx={stylexStyles2.rule8}>
                <span sx={stylexStyles2.rule9}>{cluster.name}</span>
                <span sx={stylexStyles2.rule10}>{cluster.key}</span>
              </span>
              {value.clusterKey === cluster.key && (
                <LuCheck sx={stylexStyles.rule3} aria-label="Selected" />
              )}
            </button>
          ))}
        </div>
      </section>

      <section sx={nativeChildSpacing.s3}>
        <div sx={stylexStyles2.rule1}>
          <h3 sx={stylexStyles2.rule2}>
            <LuPlus sx={stylexStyles.rule1} />
            Initial Tasks
          </h3>
          <Button onClick={handleAddTask} variant="quiet" size="toolbarSm">
            <LuPlus sx={stylexStyles.rule1} />
            Add Task
          </Button>
        </div>
        <p sx={stylexStyles2.rule4}>
          Add at least one named task. Tasks run in the order shown.
        </p>
        <div sx={stylexStyles3.rule1}>
          {displayedTaskDefs.map((task, i) => (
            <div
              key={taskKeys[i]}
              sx={[nativeChildSpacing.s2, stylexStyles2.rule11]}
            >
              <Input
                value={task.name ?? ''}
                onChange={(e) => handleUpdateTaskDef(i, e.target.value)}
                placeholder={`Task ${i + 1} name...`}
                aria-label={`Task ${i + 1} name`}
                variant="config"
              />
              {displayedTaskDefs.length > 1 && (
                <Button
                  onClick={() => handleRemoveTask(i)}
                  aria-label={`Remove task ${i + 1}`}
                  variant="destructiveSubtle"
                >
                  <LuTrash sx={stylexStyles.rule1} />
                </Button>
              )}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
