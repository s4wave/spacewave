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
    WebkitUserSelect: 'none',
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    color: {
      default: themeColors['--color-topbar-button-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    opacity: 0.6,
    userSelect: 'none',
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    WebkitUserSelect: 'none',
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingInline: themeLayout['--spacing'],
    alignItems: 'center',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-pulldown-hover'],
      },
    },
    color: {
      default: themeColors['--color-topbar-button-text'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-topbar-button-text-hi'],
      },
    },
    display: 'flex',
    fontSize: themeLayout['--text-ui'],
    letterSpacing: `var(--tw-tracking, ${themeLayout['--text-ui--letter-spacing']})`,
    lineHeight: `var(--tw-leading, ${themeLayout['--text-ui--line-height']})`,
    userSelect: 'none',
    height: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule3: {
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    maxWidth: `calc(${themeLayout['--spacing']} * 30)`,
  },
  rule4: {
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
  },
  rule5: { overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
  rule6: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginLeft: 'auto',
  },
  rule7: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
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
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
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
