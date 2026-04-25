import { LuChevronDown, LuGitBranch, LuTag } from 'react-icons/lu'

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
          className={cn(
            'text-topbar-menu text-topbar-button-text hover:text-topbar-button-text-hi hover:bg-pulldown-hover flex h-5 items-center gap-1 rounded px-1 select-none',
            loading && 'opacity-60',
          )}
          disabled={loading}
        >
          <LuGitBranch className="text-foreground-alt h-3.5 w-3.5 shrink-0" />
          <span className="max-w-[120px] truncate">{displayName}</span>
          <LuChevronDown className="text-foreground-alt h-3 w-3 shrink-0" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="start"
        className="max-h-[300px] min-w-[180px] overflow-y-auto text-xs"
      >
        {branches.length > 0 && (
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <div className="flex items-center gap-1">
                <LuGitBranch className="h-3 w-3" />
                <span>Branches</span>
              </div>
            </DropdownMenuLabel>
            {branches.map((branch) => (
              <DropdownMenuItem
                key={branch.name}
                onClick={() => onRefSelect(branch.name ?? '')}
                className={cn(effectiveRef === branch.name && 'bg-accent')}
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
                <LuTag className="h-3 w-3" />
                <span>Tags</span>
              </div>
            </DropdownMenuLabel>
            {tags.map((tag) => (
              <DropdownMenuItem
                key={tag.name}
                onClick={() => onRefSelect(tag.name ?? '')}
                className={cn(effectiveRef === tag.name && 'bg-accent')}
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
