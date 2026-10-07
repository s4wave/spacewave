import { useState, type ReactNode } from 'react'
import {
  LuCloud,
  LuGitBranch,
  LuKeyboard,
  LuLogOut,
  LuMerge,
  LuTerminal,
  LuTrash2,
} from 'react-icons/lu'

import type { Session } from '@s4wave/sdk/session/session.js'
import { useInvokeCommand } from '@s4wave/web/command/index.js'
import { useSessionNavigate } from '@s4wave/web/contexts/contexts.js'
import { useNavigate } from '@s4wave/web/router/router.js'
import { cn } from '@s4wave/web/style/utils.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'

import { LogoutConfirmDialog } from '../LogoutConfirmDialog.js'
import { DeleteAccountDialog } from './DeleteAccountDialog.js'
import { DeleteSpaceEscapeHatchDialog } from './DeleteSpaceEscapeHatchDialog.js'

// ActionTone holds the class names that give an action card its tone.
interface ActionTone {
  card: string
  iconBox: string
  icon: string
  title: string
  description: string
}

const neutralTone: ActionTone = {
  card: 'border-foreground/10 bg-foreground/5 hover:border-foreground/20 hover:bg-foreground/10',
  iconBox: 'bg-foreground/10 group-hover:bg-foreground/15',
  icon: 'text-foreground-alt size-3.5 transition-colors',
  title: 'text-foreground',
  description: 'text-foreground-alt',
}

const brandTone: ActionTone = {
  card: 'border-foreground/10 bg-foreground/5 hover:border-brand/30 hover:bg-brand/5',
  iconBox: 'bg-foreground/10 group-hover:bg-brand/10',
  icon: 'text-foreground-alt group-hover:text-brand size-3.5 transition-colors',
  title: 'text-foreground',
  description: 'text-foreground-alt',
}

const warningTone: ActionTone = {
  card: 'border-warning/30 bg-warning/5 hover:border-warning hover:bg-warning/10',
  iconBox: 'bg-warning/20 group-hover:bg-warning/30',
  icon: 'text-warning size-3.5',
  title: 'text-warning',
  description: 'text-warning/80',
}

const destructiveTone: ActionTone = {
  card: 'border-destructive/20 bg-destructive/5 hover:border-destructive/40 hover:bg-destructive/10',
  iconBox: 'bg-destructive/10 group-hover:bg-destructive/15',
  icon: 'text-destructive size-3.5 transition-colors',
  title: 'text-destructive transition-colors',
  description: 'text-destructive/80 transition-colors',
}

const destructiveStrongTone: ActionTone = {
  card: 'border-destructive/30 bg-destructive/5 hover:border-destructive hover:bg-destructive hover:text-destructive-foreground',
  iconBox: 'bg-destructive/20 group-hover:bg-destructive-foreground/20',
  icon: 'text-destructive group-hover:text-destructive-foreground size-3.5 transition-colors',
  title:
    'text-destructive group-hover:text-destructive-foreground transition-colors',
  description:
    'text-destructive/80 group-hover:text-destructive-foreground/80 transition-colors',
}

interface ActionCardProps {
  tone: ActionTone
  icon: ReactNode
  title: ReactNode
  description: ReactNode
  onClick: () => void
  disabled?: boolean
}

// ActionCard renders a full-width action with an icon, title, and description.
function ActionCard({
  tone,
  icon,
  title,
  description,
  onClick,
  disabled,
}: ActionCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={cn(
        'group flex w-full cursor-pointer items-center gap-3 rounded-md border p-2 text-left transition-colors',
        tone.card,
        disabled && 'cursor-not-allowed opacity-50',
      )}
    >
      <div
        className={cn(
          'flex size-7 shrink-0 items-center justify-center rounded-md transition-colors',
          tone.iconBox,
        )}
      >
        {icon}
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <h4 className={cn('text-xs font-medium select-none', tone.title)}>
          {title}
        </h4>
        <p className={cn('text-xs select-none', tone.description)}>
          {description}
        </p>
      </div>
    </button>
  )
}

export interface SessionActionsSectionProps {
  session: Session | null | undefined
  sessionIdx: number | null
  isLocal: boolean
  showTransfer: boolean
  showLogout: boolean
  loggingOut: boolean
  logoutOpen: boolean
  dangerZoneOpen: boolean
  onDangerZoneOpenChange: (open: boolean) => void
  onLogoutOpenChange: (open: boolean) => void
  onLogoutClick: () => void
  onLogoutConfirm: () => void
  onUpgradeToCloud: () => void
  // onDeleteAccountRedirect closes the details before a cloud account is
  // deleted through its own flow.
  onDeleteAccountRedirect: () => void
}

// SessionActionsSection renders the account actions and their confirmation
// dialogs.
export function SessionActionsSection({
  session,
  sessionIdx,
  isLocal,
  showTransfer,
  showLogout,
  loggingOut,
  logoutOpen,
  dangerZoneOpen,
  onDangerZoneOpenChange,
  onLogoutOpenChange,
  onLogoutClick,
  onLogoutConfirm,
  onUpgradeToCloud,
  onDeleteAccountRedirect,
}: SessionActionsSectionProps) {
  const navigate = useNavigate()
  const navigateSession = useSessionNavigate()
  const invokeCommand = useInvokeCommand()
  const [deleteAcctOpen, setDeleteAcctOpen] = useState(false)
  const [deleteSpaceOpen, setDeleteSpaceOpen] = useState(false)

  const handleDeleteAccount = async () => {
    if (!isLocal && sessionIdx != null) {
      setDeleteAcctOpen(false)
      onDeleteAccountRedirect()
      navigateSession({ path: 'delete-account' })
      return
    }
    if (session && sessionIdx != null) {
      await session.deleteAccount(sessionIdx)
    }
    navigate({ path: '/sessions' })
  }

  return (
    <section>
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-foreground-alt text-xs font-medium select-none">
          Actions
        </h2>
      </div>

      <div className="space-y-2">
        {isLocal && sessionIdx != null && (
          <ActionCard
            tone={brandTone}
            icon={<LuCloud className={brandTone.icon} />}
            title="Upgrade to Cloud"
            description="Sync across devices with Spacewave Cloud"
            onClick={onUpgradeToCloud}
          />
        )}

        <ActionCard
          tone={neutralTone}
          icon={<LuKeyboard className={neutralTone.icon} />}
          title="Keyboard Shortcuts"
          description="Open account keyboard shortcut overrides"
          onClick={() =>
            invokeCommand('spacewave.preferences.keyboard-shortcuts', {
              scope: 'account',
            })
          }
        />

        <ActionCard
          tone={neutralTone}
          icon={<LuTerminal className={neutralTone.icon} />}
          title="Command Line"
          description="Connect the spacewave CLI to this session"
          onClick={() => navigateSession({ path: 'settings/cli' })}
        />

        <ActionCard
          tone={neutralTone}
          icon={<LuGitBranch className={neutralTone.icon} />}
          title="Plugins"
          description="Add a plugin from GitHub and check it for updates"
          onClick={() => navigateSession({ path: 'settings/plugins' })}
        />

        {showTransfer && (
          <ActionCard
            tone={neutralTone}
            icon={<LuMerge className={neutralTone.icon} />}
            title="Transfer Sessions"
            description="Merge spaces from another session into this one"
            onClick={() => navigateSession({ path: 'settings/transfer' })}
          />
        )}

        {showLogout && (
          <ActionCard
            tone={warningTone}
            icon={<LuLogOut className={warningTone.icon} />}
            title={loggingOut ? 'Logging out…' : 'Log Out'}
            description="Sign out and remove session data"
            onClick={onLogoutClick}
            disabled={loggingOut}
          />
        )}

        <CollapsibleSection
          title="Danger Zone"
          open={dangerZoneOpen}
          onOpenChange={onDangerZoneOpenChange}
        >
          <div className="space-y-2">
            <ActionCard
              tone={destructiveTone}
              icon={<LuTrash2 className={destructiveTone.icon} />}
              title="Delete a Space"
              description="Permanently remove a broken space without opening it"
              onClick={() => setDeleteSpaceOpen(true)}
              disabled={!session}
            />
            <ActionCard
              tone={destructiveStrongTone}
              icon={<LuTrash2 className={destructiveStrongTone.icon} />}
              title={isLocal ? 'Delete Local Data' : 'Delete Account'}
              description={
                isLocal
                  ? 'Permanently remove this account and all local data'
                  : 'Permanently delete this account and all data'
              }
              onClick={() => setDeleteAcctOpen(true)}
            />
          </div>
        </CollapsibleSection>
      </div>

      <DeleteAccountDialog
        open={deleteAcctOpen}
        onOpenChange={setDeleteAcctOpen}
        isCloud={!isLocal}
        onConfirm={handleDeleteAccount}
      />
      <DeleteSpaceEscapeHatchDialog
        open={deleteSpaceOpen}
        onOpenChange={setDeleteSpaceOpen}
        session={session ?? null}
      />
      <LogoutConfirmDialog
        open={logoutOpen}
        onOpenChange={onLogoutOpenChange}
        loggingOut={loggingOut}
        onConfirm={onLogoutConfirm}
      />
    </section>
  )
}
