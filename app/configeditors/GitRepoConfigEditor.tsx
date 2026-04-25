import { LuCopy, LuGitBranch, LuLayers, LuPlus } from 'react-icons/lu'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { Input } from '@s4wave/web/ui/input.js'
import { cn } from '@s4wave/web/style/utils.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'

const inputClassName =
  'border-foreground/10 bg-background/20 text-foreground placeholder:text-foreground-alt/40 focus-visible:border-brand/50 focus-visible:ring-brand/15 h-9 text-xs md:text-xs'

// GitRepoConfigEditor edits the config-specific fields of a CreateGitRepoWizardOp.
// Renders a new/clone toggle and clone options (URL, ref, depth, recursive).
export function GitRepoConfigEditor({
  value,
  onValueChange,
}: ConfigEditorProps<CreateGitRepoWizardOp>) {
  const cloneOpts = value.cloneOpts ?? {}

  const handleSelectMode = (clone: boolean) => {
    onValueChange({ ...value, clone })
  }

  return (
    <div className="space-y-3">
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuGitBranch className="h-3.5 w-3.5" />
            Repository Source
          </h3>
        </div>
        <div className="space-y-2">
          <RadioOption
            selected={!value.clone}
            onSelect={() => handleSelectMode(false)}
            icon={<LuPlus className="h-3.5 w-3.5" />}
            label="New empty repository"
            description="Start with an empty Git repository."
          />
          <RadioOption
            selected={value.clone ?? false}
            onSelect={() => handleSelectMode(true)}
            icon={<LuCopy className="h-3.5 w-3.5" />}
            label="Clone a repository"
            description="Import history from an existing Git URL."
          />
        </div>
      </section>

      {value.clone && (
        <section>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
              <LuCopy className="h-3.5 w-3.5" />
              Clone Options
            </h3>
          </div>
          <div className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5">
            <div className="space-y-2">
              <label className="text-foreground text-xs font-medium select-none">
                Clone URL
              </label>
              <Input
                value={cloneOpts.url ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, url: e.target.value },
                  })
                }
                placeholder="https://github.com/user/repo.git"
                className={inputClassName}
              />
            </div>
            <div className="space-y-2">
              <label className="text-foreground text-xs font-medium select-none">
                Branch / Ref
              </label>
              <Input
                value={cloneOpts.ref ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, ref: e.target.value },
                  })
                }
                placeholder="main (leave empty for default)"
                className={inputClassName}
              />
            </div>
            <div className="space-y-2">
              <label className="text-foreground text-xs font-medium select-none">
                Depth
              </label>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  className={cn(
                    'border-foreground/10 bg-background/20 text-foreground-alt hover:border-foreground/20 hover:bg-background/30 rounded-md border px-3 py-2 text-left text-xs transition-all duration-150 select-none',
                    (cloneOpts.depth ?? 0) === 0 &&
                      'border-brand/30 bg-brand/5 text-foreground',
                  )}
                  onClick={() =>
                    onValueChange({
                      ...value,
                      cloneOpts: { ...cloneOpts, depth: 0 },
                    })
                  }
                >
                  Full history
                </button>
                <button
                  type="button"
                  className={cn(
                    'border-foreground/10 bg-background/20 text-foreground-alt hover:border-foreground/20 hover:bg-background/30 rounded-md border px-3 py-2 text-left text-xs transition-all duration-150 select-none',
                    cloneOpts.depth === 1 &&
                      'border-brand/30 bg-brand/5 text-foreground',
                  )}
                  onClick={() =>
                    onValueChange({
                      ...value,
                      cloneOpts: { ...cloneOpts, depth: 1 },
                    })
                  }
                >
                  Shallow clone
                </button>
              </div>
            </div>
            <label className="border-foreground/6 bg-background/20 hover:border-foreground/12 hover:bg-background/30 flex items-center gap-3 rounded-md border px-3 py-2 transition-all duration-150">
              <input
                type="checkbox"
                checked={cloneOpts.recursive ?? false}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, recursive: e.target.checked },
                  })
                }
                className="accent-brand h-4 w-4 rounded"
              />
              <span className="text-foreground flex min-w-0 items-center gap-2 text-xs font-medium select-none">
                <LuLayers className="text-foreground-alt/50 h-3.5 w-3.5 shrink-0" />
                Clone submodules recursively
              </span>
            </label>
          </div>
        </section>
      )}
    </div>
  )
}
