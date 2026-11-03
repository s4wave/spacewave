import { useCallback, useState } from 'react'
import { LuBuilding2, LuPlus } from '@s4wave/web/images/icons.js'

import type { Session } from '@s4wave/sdk/session/session.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SessionContext,
  useSessionNavigate,
} from '@s4wave/web/contexts/contexts.js'
import { SpacewaveOrgListContext } from '@s4wave/web/contexts/SpacewaveOrgListContext.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { LoadingInline } from '@s4wave/web/ui/loading/LoadingInline.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  motion as themeMotion,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule3: {
    display: 'flex',
    width: '100%',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingBlock: themeLayout['--spacing'],
    textAlign: 'left',
  },
  rule4: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule6: {
    display: 'flex',
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 60%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule7: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      ':focus': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    },
  },
  rule8: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule10: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule11: {
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
      },
    },
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    cursor: { default: null, ':disabled': 'not-allowed' },
    opacity: { default: null, ':disabled': 0.5 },
  },
  rule12: {
    flex: 1,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

export interface OrganizationsSectionProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onNavigateToOrganization?: (orgId: string) => void
}

// OrganizationsSection shows a collapsible list of orgs with create button.
export function OrganizationsSection({
  sx,
  open,
  onOpenChange,
  onNavigateToOrganization,
}: OrganizationsSectionProps & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'orgs', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen
  const navigateSession = useSessionNavigate()

  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)
  const { providerId } = useSessionInfo(session)
  const isLocal = providerId === 'local'

  const orgList = SpacewaveOrgListContext.useContextSafe()
  const orgs = orgList?.organizations ?? []
  const loading = orgList?.loading ?? false

  const [showCreate, setShowCreate] = useState(false)

  const handleOpenOrganization = (orgId: string) => {
    if (onNavigateToOrganization) {
      onNavigateToOrganization(orgId)
      return
    }
    navigateSession({ path: `org/${orgId}/` })
  }

  if (isLocal && !orgList) return null

  const count = orgs.length

  return (
    <CollapsibleSection
      sx={sx}
      title="Organizations"
      icon={<LuBuilding2 sx={stylexStyles.rule1} />}
      open={sectionOpen}
      onOpenChange={handleOpenChange}
      badge={
        count > 0 ? <span sx={stylexStyles2.rule1}>{count}</span> : undefined
      }
    >
      <div>
        {loading && (
          <LoadingInline
            sx={nativeChildSpacingNext.s2}
            label="Loading organizations"
            tone="muted"
            size="sm"
          />
        )}
        {!loading && orgs.length === 0 && !showCreate && (
          <p sx={[nativeChildSpacingNext.s2, stylexStyles2.rule2]}>
            No organizations yet.
          </p>
        )}
        {!loading &&
          orgs.map((org) => (
            <button
              type="button"
              key={org.id}
              onClick={() => handleOpenOrganization(org.id ?? '')}
              sx={[nativeChildSpacingNext.s2, stylexStyles2.rule3]}
            >
              <span sx={stylexStyles2.rule4}>{org.displayName || org.id}</span>
              <span sx={stylexStyles2.rule5}>Open &gt;</span>
            </button>
          ))}

        {showCreate ? (
          <CreateOrganizationForm
            sx={nativeChildSpacingNext.s2}
            session={session}
            onClose={() => setShowCreate(false)}
          />
        ) : (
          <button
            type="button"
            onClick={() => setShowCreate(true)}
            sx={[nativeChildSpacingNext.s2, stylexStyles2.rule6]}
          >
            <LuPlus sx={stylexStyles.rule2} />
            Create Organization
          </button>
        )}
      </div>
    </CollapsibleSection>
  )
}

// CreateOrganizationForm collects an organization name and creates it. It owns
// the draft state, so closing the form discards the draft and any error.
function CreateOrganizationForm({
  sx,
  session,
  onClose,
}: {
  session: Session | null | undefined
  onClose: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [orgName, setOrgName] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const trimmedOrgName = orgName.trim()
  const handleOrgNameInputRef = useCallback((node: HTMLInputElement | null) => {
    node?.focus()
  }, [])

  const handleCreate = async () => {
    if (!session || !trimmedOrgName || creating) return
    setCreating(true)
    setError(null)
    try {
      await session.spacewave.createOrganization(trimmedOrgName)
      onClose()
    } catch (err) {
      setError(
        err instanceof Error ? err.message : 'Failed to create organization',
      )
      setCreating(false)
    }
  }

  return (
    <div sx={sx}>
      <input
        ref={handleOrgNameInputRef}
        value={orgName}
        disabled={creating}
        aria-busy={creating}
        aria-label="Organization name"
        onChange={(e) => {
          setOrgName(e.target.value)
          setError(null)
        }}
        placeholder="Organization name"
        sx={[nativeChildSpacing.s2, [stylexStyles2.rule7]]}
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return
          if (e.key === 'Enter' && trimmedOrgName && !creating) {
            void handleCreate()
          }
        }}
      />
      {creating && (
        <div sx={[nativeChildSpacing.s2, stylexStyles2.rule8]}>
          <Spinner size="sm" />
          <span>Creating organization…</span>
        </div>
      )}
      {error && (
        <p sx={[nativeChildSpacing.s2, stylexStyles2.rule9]}>{error}</p>
      )}
      <div sx={[nativeChildSpacing.s2, stylexStyles2.rule10]}>
        <button
          type="button"
          onClick={() => void handleCreate()}
          disabled={creating || !trimmedOrgName}
          aria-busy={creating}
          sx={[stylexStyles2.rule11]}
        >
          {creating ? 'Creating…' : 'Create'}
        </button>
        <button
          type="button"
          onClick={onClose}
          disabled={creating}
          sx={stylexStyles2.rule12}
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
