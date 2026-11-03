import { useMemo } from 'react'
import { LuBriefcase } from '@s4wave/web/images/icons.js'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

// ForgeTaskConfigEditor edits the config-specific fields of a ForgeTaskCreateOp.
// Renders a job picker for selecting which Job to link the new Task to.
export function ForgeTaskConfigEditor({
  sx,
  value,
  onValueChange,
}: ConfigEditorProps<ForgeTaskCreateOp> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const { spaceWorldResource } = SpaceContainerContext.useContext()

  const jobsResource = useResource(
    spaceWorldResource,
    async (world: IWorldState, signal: AbortSignal) => {
      return listObjectsWithType(world, 'forge/job', signal)
    },
    [],
  )
  const jobs = useMemo(() => jobsResource.value ?? [], [jobsResource.value])

  const handleSelectJob = (jobKey: string) => {
    onValueChange({ ...value, jobKey })
  }

  return (
    <section sx={sx}>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuBriefcase sx={stylexStyles.rule1} />
          Target Job
        </h3>
      </div>
      {jobs.length === 0 && (
        <div className="border-foreground/6 bg-background-card/30 text-foreground-alt/40 flex items-center gap-2 rounded-lg border px-3.5 py-3 text-xs">
          <LuBriefcase sx={stylexStyles.rule2} />
          {jobsResource.loading
            ? 'Loading jobs…'
            : 'No jobs found. Create a job first.'}
        </div>
      )}
      <div className="space-y-2">
        {jobs.map((jobKey) => (
          <button
            type="button"
            key={jobKey}
            className={cn(
              'border-foreground/6 bg-background-card/30 hover:border-foreground/12 hover:bg-background-card/50 flex w-full items-center gap-3 rounded-lg border p-3 text-left transition-all duration-150',
              value.jobKey === jobKey && 'border-brand/30 bg-brand/5',
            )}
            onClick={() => handleSelectJob(jobKey)}
          >
            <span className="bg-foreground/5 flex size-7 shrink-0 items-center justify-center rounded-md">
              <LuBriefcase sx={stylexStyles.rule3} />
            </span>
            <span className="text-foreground text-xs font-medium">
              {jobKey}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}
