import { useId } from 'react'
import {
  LuCopy,
  LuGitBranch,
  LuLayers,
  LuPlus,
} from '@s4wave/web/images/icons.js'

import type { ConfigEditorProps } from '@s4wave/web/configtype/configtype.js'
import { Input } from '@s4wave/web/ui/input.js'
import { cn } from '@s4wave/web/style/utils.js'
import { RadioOption } from '@s4wave/web/ui/RadioOption.js'
import type { CreateGitRepoWizardOp } from '@s4wave/core/git/git.pb.js'

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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

// GitRepoConfigEditor edits the config-specific fields of a CreateGitRepoWizardOp.
// Renders a new/clone toggle and clone options (URL, ref, depth, recursive).
export function GitRepoConfigEditor({
  sx,
  value,
  onValueChange,
}: ConfigEditorProps<CreateGitRepoWizardOp> & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const cloneOpts = value.cloneOpts ?? {}
  const cloneUrlId = useId()
  const cloneRefId = useId()

  const handleSelectMode = (clone: boolean) => {
    onValueChange({ ...value, clone })
  }

  return (
    <div sx={sx} className="space-y-3">
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
            <LuGitBranch sx={stylexStyles.rule1} />
            Repository Source
          </h3>
        </div>
        <div className="space-y-2">
          <RadioOption
            selected={!value.clone}
            onSelect={() => handleSelectMode(false)}
            icon={<LuPlus sx={stylexStyles.rule1} />}
            label="New empty repository"
            description="Start with an empty Git repository."
          />
          <RadioOption
            selected={value.clone ?? false}
            onSelect={() => handleSelectMode(true)}
            icon={<LuCopy sx={stylexStyles.rule1} />}
            label="Clone a repository"
            description="Import history from an existing Git URL."
          />
        </div>
      </section>

      {value.clone && (
        <section>
          <div className="mb-2 flex items-center justify-between">
            <h3 className="text-foreground flex items-center gap-1.5 text-xs font-medium select-none">
              <LuCopy sx={stylexStyles.rule1} />
              Clone Options
            </h3>
          </div>
          <div className="border-foreground/6 bg-background-card/30 space-y-3 rounded-lg border p-3.5">
            <div className="space-y-2">
              <label
                htmlFor={cloneUrlId}
                className="text-foreground text-xs font-medium select-none"
              >
                Clone URL
              </label>
              <Input
                id={cloneUrlId}
                value={cloneOpts.url ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, url: e.target.value },
                  })
                }
                placeholder="https://github.com/user/repo.git"
                variant="configCompact"
              />
            </div>
            <div className="space-y-2">
              <label
                htmlFor={cloneRefId}
                className="text-foreground text-xs font-medium select-none"
              >
                Branch / Ref
              </label>
              <Input
                id={cloneRefId}
                value={cloneOpts.ref ?? ''}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, ref: e.target.value },
                  })
                }
                placeholder="main (leave empty for default)"
                variant="configCompact"
              />
            </div>
            <div className="space-y-2">
              <span className="text-foreground text-xs font-medium select-none">
                Depth
              </span>
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
            <label className="border-foreground/6 bg-background/20 hover:border-foreground/12 hover:bg-background/30 flex items-center gap-3 rounded-md border px-3 py-2 transition duration-150">
              <input
                type="checkbox"
                checked={cloneOpts.recursive ?? false}
                onChange={(e) =>
                  onValueChange({
                    ...value,
                    cloneOpts: { ...cloneOpts, recursive: e.target.checked },
                  })
                }
                className="accent-brand size-4 rounded"
              />
              <span className="text-foreground flex min-w-0 items-center gap-2 text-xs font-medium select-none">
                <LuLayers sx={stylexStyles.rule2} />
                Clone submodules recursively
              </span>
            </label>
          </div>
        </section>
      )}
    </div>
  )
}
