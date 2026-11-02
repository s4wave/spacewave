import { LuChevronDown, LuGitBranch, LuTag } from '@s4wave/web/images/icons.js'

import type { ListRefsResponse } from '@s4wave/sdk/git/repo.pb.js'

import { cn } from '@s4wave/web/style/utils.js'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@s4wave/web/ui/DropdownMenu.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    maxHeight: `calc(${themeLayout['--spacing']} * 75)`,
    minWidth: `calc(${themeLayout['--spacing']} * 45)`,
    overflowY: 'auto',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

// RefSelectorProps are props for the RefSelector component.
export interface RefSelectorProps {
  effectiveRef: string | null
  refsResponse: ListRefsResponse | null
  loading: boolean
  onRefSelect: (refName: string) => void
}

// RefSelector renders a dropdown menu for selecting branches and tags.
export function RefSelector({
  effectiveRef,
  refsResponse,
  loading,
  onRefSelect,
}: RefSelectorProps) {
  const branches = refsResponse?.branches ?? []
  const tags = refsResponse?.tags ?? []
  const displayName = effectiveRef ?? (loading ? '...' : 'HEAD')

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className={cn(
            'text-topbar-menu text-topbar-button-text hover:text-topbar-button-text-hi hover:bg-pulldown-hover flex h-5 items-center gap-1 rounded px-1 select-none',
            loading && 'opacity-60',
          )}
          disabled={loading}
        >
          <LuGitBranch sx={stylexStyles.rule1} />
          <span className="max-w-30 truncate">{displayName}</span>
          <LuChevronDown sx={stylexStyles.rule2} />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        variant="compact"
        sx={stylexStyles2.rule1}
      >
        {branches.length > 0 && (
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <div className="flex items-center gap-1">
                <LuGitBranch sx={stylexStyles.rule3} />
                <span>Branches</span>
              </div>
            </DropdownMenuLabel>
            {branches.map((branch) => (
              <DropdownMenuItem
                key={branch.name}
                onClick={() => onRefSelect(branch.name ?? '')}
                variant={effectiveRef === branch.name ? 'selected' : 'default'}
              >
                <span className="truncate">{branch.name}</span>
                {branch.isHead && (
                  <span className="text-foreground-alt/70 ml-auto shrink-0 text-xs">
                    HEAD
                  </span>
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        )}
        {branches.length > 0 && tags.length > 0 && <DropdownMenuSeparator />}
        {tags.length > 0 && (
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <div className="flex items-center gap-1">
                <LuTag sx={stylexStyles.rule3} />
                <span>Tags</span>
              </div>
            </DropdownMenuLabel>
            {tags.map((tag) => (
              <DropdownMenuItem
                key={tag.name}
                onClick={() => onRefSelect(tag.name ?? '')}
                variant={effectiveRef === tag.name ? 'selected' : 'default'}
              >
                <span className="truncate">{tag.name}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        )}
        {branches.length === 0 && tags.length === 0 && !loading && (
          <div className="text-foreground-alt px-2 py-1.5 text-xs">
            No refs found
          </div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
