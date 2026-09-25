import {
  LuArrowRight,
  LuArrowRightLeft,
  LuIdCard,
  LuLock,
  LuLockOpen,
  LuUser,
} from 'react-icons/lu'

import { useSessionMetadata } from '@s4wave/app/hooks/useSessionMetadata.js'
import {
  SessionLockMode,
  type SessionListEntry,
} from '@s4wave/core/session/session.pb.js'
import type { SpaceSoListEntry } from '@s4wave/core/space/space.pb.js'
import { useSessionIndex } from '@s4wave/web/contexts/contexts.js'
import { useBottomBarSetOpenMenu } from '@s4wave/web/frame/bottom-bar-context.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { cn } from '@s4wave/web/style/utils.js'

import { Facts } from './Facts.js'
import { InspectorSection } from './InspectorSection.js'
import { spaceEngineId } from './useSystemModel.js'

// SpacesInspector lists every account session on this device and every Space
// in the current session, with actions to switch to or open each one.
export function SpacesInspector({
  sessions,
  spaces,
  onClose,
}: {
  sessions: SessionListEntry[] | null
  spaces: SpaceSoListEntry[] | null
  onClose: () => void
}) {
  const navigate = useNavigate()
  const sessionIndex = useSessionIndex()

  return (
    <div className="grid gap-3 @3xl:grid-cols-2">
      <InspectorSection
        title="Accounts"
        description="Every session on this device. Each one keeps its own keys and Spaces."
      >
        {!sessions ? (
          <p className="text-foreground-alt/50 text-xs">Reading sessions…</p>
        ) : (
          <ul className="space-y-3">
            {sessions.map((session) => (
              <SessionRow
                key={session.sessionIndex}
                sessionIndex={session.sessionIndex ?? 0}
                current={session.sessionIndex === sessionIndex}
                onClose={onClose}
              />
            ))}
          </ul>
        )}
      </InspectorSection>

      <InspectorSection
        title="Spaces in this session"
        description="Spaces mounted by the current session."
      >
        {!spaces ? (
          <p className="text-foreground-alt/50 text-xs">Reading Spaces…</p>
        ) : spaces.length === 0 ? (
          <p className="text-foreground-alt/50 text-xs">
            This session has no Spaces yet.
          </p>
        ) : (
          <ul className="divide-foreground/6 -mx-2 divide-y">
            {spaces.map((space) => {
              const id = space.entry?.ref?.providerResourceRef?.id ?? ''
              const name = space.spaceMeta?.name || 'Untitled Space'
              return (
                <li key={spaceEngineId(space)}>
                  <button
                    type="button"
                    disabled={!id}
                    onClick={() => {
                      navigate({ path: `/u/${sessionIndex}/so/${id}` })
                      onClose()
                    }}
                    className="group hover:bg-foreground/[0.03] focus-visible:ring-brand/50 flex w-full items-center gap-3 rounded-md px-2 py-2 text-left text-xs outline-none focus-visible:ring-2"
                    aria-label={`Open ${name}`}
                  >
                    <span className="min-w-0 flex-1">
                      <span className="text-foreground block truncate font-medium">
                        {name}
                      </span>
                      <span className="text-foreground-alt/50 block truncate font-mono">
                        {id || 'unknown'}
                      </span>
                    </span>
                    <span className="text-foreground-alt/60 shrink-0">
                      {space.entry?.source === 'shared' ? 'Shared' : 'Owned'}
                    </span>
                    <LuArrowRight
                      className="text-foreground-alt/40 group-hover:text-foreground size-3.5 shrink-0 transition-colors"
                      aria-hidden="true"
                    />
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </InspectorSection>
    </div>
  )
}

// SessionRow is one account session with its provider facts and actions.
function SessionRow({
  sessionIndex,
  current,
  onClose,
}: {
  sessionIndex: number
  current: boolean
  onClose: () => void
}) {
  const metadata = useSessionMetadata(sessionIndex)
  const navigate = useNavigate()
  const setOpenMenu = useBottomBarSetOpenMenu()
  const name = metadata?.displayName || `Session ${sessionIndex}`
  const autoUnlock = metadata?.lockMode === SessionLockMode.AUTO_UNLOCK

  return (
    <li
      className={cn(
        'rounded-md border p-3',
        current ? 'border-brand/30 bg-brand/5' : 'border-foreground/8',
      )}
    >
      <div className="flex items-center gap-2 text-xs">
        <LuUser
          className="text-foreground-alt/60 size-3.5 shrink-0"
          aria-hidden="true"
        />
        <span className="text-foreground min-w-0 flex-1 truncate font-medium">
          {name}
        </span>
        <span className="text-foreground-alt/50 font-mono">
          /u/{sessionIndex}
        </span>
        {current && <span className="text-brand">Current</span>}
      </div>

      <div className="mt-2">
        <Facts
          facts={[
            { label: 'Provider', value: metadata?.providerDisplayName },
            {
              label: 'Account',
              value: metadata?.cloudAccountId || metadata?.providerAccountId,
              mono: true,
            },
            { label: 'Entity', value: metadata?.cloudEntityId, mono: true },
            {
              label: 'Created',
              value: metadata?.createdAt
                ? new Date(Number(metadata.createdAt)).toLocaleDateString(
                    'en-US',
                    { month: 'short', day: 'numeric', year: 'numeric' },
                  )
                : null,
            },
          ]}
        />
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        {metadata?.lockMode != null && (
          <span className="text-foreground-alt/60 mr-auto flex items-center gap-1.5 text-xs">
            {autoUnlock ? (
              <LuLockOpen className="size-3.5" aria-hidden="true" />
            ) : (
              <LuLock className="size-3.5" aria-hidden="true" />
            )}
            {autoUnlock ? 'Unlocks automatically' : 'Locked with a PIN'}
          </span>
        )}
        {!current && (
          <DashboardButton
            icon={<LuArrowRightLeft className="size-3.5" />}
            onClick={() => {
              navigate({ path: `/u/${sessionIndex}` })
              onClose()
            }}
          >
            Switch to account
          </DashboardButton>
        )}
        <DashboardButton
          icon={<LuIdCard className="size-3.5" />}
          onClick={() => {
            if (!current) {
              navigate({ path: `/u/${sessionIndex}` })
            }
            queueMicrotask(() => setOpenMenu?.('account'))
          }}
        >
          Account details
        </DashboardButton>
      </div>
    </li>
  )
}
