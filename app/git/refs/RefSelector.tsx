import { LuChevronDown, LuGitBranch, LuTag } from '@s4wave/web/images/icons.js'

import type { ListRefsResponse } from '@s4wave/sdk/git/repo.pb.js'

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

import { fonts as themeFonts } from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    paddingInline: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-topbar-button-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    opacity: 0.6,
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 5)`,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    borderRadius: '.25rem',
    paddingInline: themeLayout['--spacing'],
    color: {
      default: themeColors['--color-topbar-button-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    WebkitUserSelect: 'none',
    userSelect: 'none',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    fontSize: themeLayout['--text-ui'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
  },
  rule3: {
    maxWidth: `calc(${themeLayout['--spacing']} * 30)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
  },
  rule5: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule6: {
    marginLeft: 'auto',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule7: {
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

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
          sx={loading ? stylexStyles3.rule1 : stylexStyles3.rule2}
          disabled={loading}
        >
          <LuGitBranch sx={stylexStyles.rule1} />
          <span sx={stylexStyles3.rule3}>{displayName}</span>
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
              <div sx={stylexStyles3.rule4}>
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
                <span sx={stylexStyles3.rule5}>{branch.name}</span>
                {branch.isHead && <span sx={stylexStyles3.rule6}>HEAD</span>}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        )}
        {branches.length > 0 && tags.length > 0 && <DropdownMenuSeparator />}
        {tags.length > 0 && (
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              <div sx={stylexStyles3.rule4}>
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
                <span sx={stylexStyles3.rule5}>{tag.name}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        )}
        {branches.length === 0 && tags.length === 0 && !loading && (
          <div sx={stylexStyles3.rule7}>No refs found</div>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
