import { useCallback } from 'react'
import { LuArrowDownAZ, LuArrowUpAZ } from '@s4wave/web/images/icons.js'

import { Input } from '@s4wave/web/ui/input.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'

import type { KvSortDirection } from './kv-viewer.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

const stylexStyles = stylex.create({ rule1: { flex: 1 } })

interface KvKeyListToolbarProps {
  prefix: string
  onPrefixChange: (prefix: string) => void
  sortDirection: KvSortDirection
  onToggleSortDirection: () => void
}

// KvKeyListToolbar renders the prefix filter input and sort direction toggle
// for the key list. Both controls drive the watched list reactively.
export function KvKeyListToolbar({
  prefix,
  onPrefixChange,
  sortDirection,
  onToggleSortDirection,
}: KvKeyListToolbarProps) {
  const handlePrefixChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      onPrefixChange(event.target.value)
    },
    [onPrefixChange],
  )

  return (
    <div sx={stylexStyles3.rule1}>
      <Input
        value={prefix}
        onChange={handlePrefixChange}
        placeholder="Filter by key prefix"
        aria-label="Filter by key prefix"
        variant="toolbar"
        sx={stylexStyles.rule1}
      />
      <DashboardButton
        icon={
          sortDirection === 'asc' ? (
            <LuArrowDownAZ sx={stylexStyles2.rule1} />
          ) : (
            <LuArrowUpAZ sx={stylexStyles2.rule1} />
          )
        }
        onClick={onToggleSortDirection}
        aria-label={
          sortDirection === 'asc'
            ? 'Sort keys descending'
            : 'Sort keys ascending'
        }
        title={sortDirection === 'asc' ? 'Ascending' : 'Descending'}
      />
    </div>
  )
}
