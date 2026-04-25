import { useMemo } from 'react'
import { LuBriefcase } from 'react-icons/lu'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import { cn } from '@s4wave/web/style/utils.js'
import type { ForgeTaskCreateOp } from '@s4wave/core/forge/task/task.pb.js'
import { listObjectsWithType } from '@s4wave/sdk/world/types/types.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'

// ForgeTaskConfigEditor edits the config-specific fields of a ForgeTaskCreateOp.
// Renders a job picker for selecting which Job to link the new Task to.
export function ForgeTaskConfigEditor({
  value,
  onValueChange,
}: ConfigEditorProps<ForgeTaskCreateOp>) {
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
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
          <LuBriefcase className="h-3.5 w-3.5" />
          Target Job
        </h3>
      </div>
      {jobs.length === 0 && (
        <div className="border-foreground/6 bg-background-card/30 text-foreground-alt/40 flex items-center gap-2 rounded-lg border px-3.5 py-3 text-xs">
          <LuBriefcase className="h-3.5 w-3.5 shrink-0" />
          {jobsResource.loading ?
            'Loading jobs...'
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
            <span className="bg-foreground/5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md">
              <LuBriefcase className="text-foreground-alt/50 h-3.5 w-3.5" />
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
