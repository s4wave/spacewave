import { useCallback } from 'react'
import { LuBox } from '@s4wave/web/images/icons.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import type { OrgSpaceInfo } from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

// OrgSpaceList renders the list of spaces owned by an organization.
export function OrgSpaceList(props: { orgId: string; spaces: OrgSpaceInfo[] }) {
  return (
    <div className="space-y-1">
      {props.spaces.length === 0 && (
        <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
          <LuBox sx={stylexStyles.rule1} />
          <span>No spaces yet</span>
        </div>
      )}
      {props.spaces.map((space) => (
        <OrgSpaceRow key={space.id} orgId={props.orgId} space={space} />
      ))}
    </div>
  )
}

function OrgSpaceRow(props: { orgId: string; space: OrgSpaceInfo }) {
  const { orgId, space } = props
  const navigateSession = useSessionNavigate()

  const handleSpaceSelect = useCallback(() => {
    navigateSession({ path: `org/${orgId}/so/${space.id}` })
  }, [navigateSession, orgId, space.id])

  return (
    <button
      type="button"
      onClick={handleSpaceSelect}
      className="hover:bg-foreground/5 flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors"
    >
      <LuBox sx={stylexStyles.rule2} />
      <div className="min-w-0 flex-1">
        <div className="text-foreground truncate text-xs font-medium">
          {space.displayName || space.id}
        </div>
      </div>
      {space.objectType && (
        <span className="text-foreground-alt/40 text-xs">
          {space.objectType}
        </span>
      )}
    </button>
  )
}
