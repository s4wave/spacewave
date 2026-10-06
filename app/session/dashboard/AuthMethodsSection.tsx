import { useState } from 'react'
import {
  LuDownload,
  LuFingerprint,
  LuGithub,
  LuKey,
  LuLock,
  LuPlus,
  LuTrash2,
} from 'react-icons/lu'
import { FcGoogle } from 'react-icons/fc'

import { downloadPemFile } from '@s4wave/web/download.js'
import { cn } from '@s4wave/web/style/utils.js'
import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { CollapsibleSection } from '@s4wave/web/ui/CollapsibleSection.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  AuthConfirmDialog,
  buildEntityCredential,
} from './AuthConfirmDialog.js'
import type { AuthCredential } from './AuthConfirmDialog.js'
import { AuthMutationWizard } from './AuthMutationWizard.js'
import type { AddMethodType } from './AuthMutationWizard.js'
import { ChangePasswordDialog } from './ChangePasswordDialog.js'
import { PasskeySection } from './PasskeySection.js'
import { SSOLinkDialog } from './SSOLinkDialog.js'
import { AccountEscalationIntentKind } from '@s4wave/sdk/account/account.pb.js'
import type {
  WatchAccountInfoResponse,
  WatchAuthMethodsResponse,
} from '@s4wave/sdk/account/account.pb.js'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Resource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  AccountAuthMethodKind,
  type AccountAuthMethod,
} from '@s4wave/core/provider/spacewave/api/api.pb.js'
import { useStateNamespace, useStateAtom } from '@s4wave/web/state/persist.js'
import { useAccountDashboardState } from './AccountDashboardStateContext.js'

export interface AuthMethodsSectionProps {
  account: Resource<Account>
  retainStepUp?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// AuthMethodType identifies which auth method the user wants to add.
type AuthMethodType = 'pem' | 'passkey' | 'google' | 'github'

// AuthMethodsSection displays auth method list and management controls.
// When auth_threshold > 0, add/remove operations use the step-through
// AuthMutationWizard for multi-sig unlock. Otherwise, falls back to the
// simple AuthConfirmDialog.
export function AuthMethodsSection({
  account,
  retainStepUp = false,
  open,
  onOpenChange,
}: AuthMethodsSectionProps) {
  const state = useAccountDashboardState(account)
  if (state) {
    return (
      <AuthMethodsSectionContent
        account={account}
        retainStepUp={retainStepUp}
        open={open}
        onOpenChange={onOpenChange}
        authMethodsResource={state.authMethods}
        accountInfoResource={state.accountInfo}
      />
    )
  }

  return (
    <AuthMethodsSectionWithWatches
      account={account}
      retainStepUp={retainStepUp}
      open={open}
      onOpenChange={onOpenChange}
    />
  )
}

function AuthMethodsSectionWithWatches(props: AuthMethodsSectionProps) {
  const authMethodsResource = useStreamingResource(
    props.account,
    (acc, signal) => acc.watchAuthMethods({}, signal),
    [],
  )
  const accountInfoResource = useStreamingResource(
    props.account,
    (acc, signal) => acc.watchAccountInfo({}, signal),
    [],
  )

  return (
    <AuthMethodsSectionContent
      {...props}
      authMethodsResource={authMethodsResource}
      accountInfoResource={accountInfoResource}
    />
  )
}

interface AuthMethodsSectionContentProps extends AuthMethodsSectionProps {
  authMethodsResource: Resource<WatchAuthMethodsResponse>
  accountInfoResource: Resource<WatchAccountInfoResponse>
}

function AuthMethodsSectionContent({
  account,
  retainStepUp = false,
  open,
  onOpenChange,
  authMethodsResource,
  accountInfoResource,
}: AuthMethodsSectionContentProps) {
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'auth-methods', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen
  const authMethods = authMethodsResource.value?.authMethods ?? []
  const keypairCount = authMethods.length
  const threshold = accountInfoResource.value?.authThreshold ?? 0
  const useWizard = threshold > 0

  // Wizard state for multi-sig flows.
  const [wizardRemove, setWizardRemove] = useState<{
    peerId: string
    label: string
  } | null>(null)
  const [wizardAddType, setWizardAddType] = useState<AddMethodType | null>(null)

  // Simple dialog state for single-sig flows.
  const [removePeerId, setRemovePeerId] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)
  const [addType, setAddType] = useState<AuthMethodType | null>(null)
  const [changePasswordOpen, setChangePasswordOpen] = useState(false)

  const handleRemoveClick = (peerId: string, methodLabel: string) => {
    if (useWizard) {
      setWizardRemove({ peerId, label: methodLabel })
    } else {
      setRemovePeerId(peerId)
    }
  }

  const handlePickMethod = (method: AuthMethodType) => {
    setPickerOpen(false)
    if (useWizard && method === 'pem') {
      setWizardAddType('pem')
    } else {
      setAddType(method)
    }
  }

  return (
    <>
      <CollapsibleSection
        title="Auth Methods"
        icon={<LuKey className="size-3.5" />}
        open={sectionOpen}
        onOpenChange={handleOpenChange}
        badge={
          keypairCount > 0 ? (
            <span className="text-foreground-alt/50 text-xs">
              {keypairCount}
            </span>
          ) : undefined
        }
        headerActions={
          <button
            type="button"
            onClick={() => setPickerOpen(true)}
            className="text-foreground-alt hover:text-foreground flex size-4 items-center justify-center transition-colors"
            aria-label="Add auth method"
            title="Add auth method"
          >
            <LuPlus className="size-3.5" />
          </button>
        }
      >
        {authMethodsResource.loading && (
          <p className="text-foreground-alt/40 text-xs">
            Loading auth methods…
          </p>
        )}
        {!authMethodsResource.loading && authMethods.length === 0 && (
          <div className="text-foreground-alt/40 flex items-center gap-2 p-1 text-xs">
            <LuKey className="size-3.5 shrink-0" />
            <span>No auth methods found</span>
          </div>
        )}
        {authMethods.length > 0 && (
          <div className="space-y-2">
            {authMethods.map((method) => (
              <AuthMethodRow
                key={method.peerId ?? method.keypair?.peerId ?? 'unknown'}
                method={method}
                hasOtherMethods={authMethods.length > 1}
                onChangePassword={() => setChangePasswordOpen(true)}
                onRemove={handleRemoveClick}
              />
            ))}
          </div>
        )}
      </CollapsibleSection>

      <AuthMethodPickerDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        onPick={handlePickMethod}
      />

      {useWizard ? (
        <>
          {/* Multi-sig wizard: remove auth method */}
          <AuthMutationWizard
            open={wizardRemove !== null}
            onClose={() => setWizardRemove(null)}
            mode="remove"
            account={account}
            retainAfterClose={retainStepUp}
            removePeerId={wizardRemove?.peerId}
            removeMethodLabel={wizardRemove?.label}
          />

          {/* Multi-sig wizard: add backup key */}
          <AuthMutationWizard
            open={wizardAddType !== null}
            onClose={() => setWizardAddType(null)}
            mode="add"
            account={account}
            retainAfterClose={retainStepUp}
            addMethodType={wizardAddType ?? undefined}
          />
        </>
      ) : (
        <SingleSigDialogs
          account={account}
          retainStepUp={retainStepUp}
          removePeerId={removePeerId}
          addType={addType}
          onRemoveClose={() => setRemovePeerId(null)}
          onAddClose={() => setAddType(null)}
        />
      )}

      {/* Register passkey dialog */}
      <PasskeySection
        open={addType === 'passkey'}
        onOpenChange={(open) => {
          if (!open) setAddType(null)
        }}
        account={account}
      />

      {(addType === 'google' || addType === 'github') && (
        <SSOLinkDialog
          open={true}
          provider={addType}
          account={account}
          retainStepUp={retainStepUp}
          onOpenChange={(next) => {
            if (!next) {
              setAddType(null)
            }
          }}
        />
      )}

      {/* Change password dialog */}
      <ChangePasswordDialog
        open={changePasswordOpen}
        onOpenChange={setChangePasswordOpen}
        account={account}
      />
    </>
  )
}

// AuthMethodRow renders an auth method with its change and remove actions.
// The only remaining method cannot be removed.
function AuthMethodRow({
  method,
  hasOtherMethods,
  onChangePassword,
  onRemove,
}: {
  method: AccountAuthMethod
  hasOtherMethods: boolean
  onChangePassword: () => void
  onRemove: (peerId: string, label: string) => void
}) {
  const peerId = method.peerId ?? method.keypair?.peerId ?? 'unknown'
  const label = method.label ?? 'Auth method'
  const secondary = method.secondaryLabel ?? ''

  return (
    <div className="border-foreground/6 bg-background-card/30 flex items-center justify-between gap-3 rounded-lg border px-3 py-2.5">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <div className="bg-foreground/5 flex size-8 shrink-0 items-center justify-center rounded-md">
          <AuthMethodIcon method={method} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-foreground text-sm font-medium">{label}</p>
          {secondary && (
            <p className="text-foreground-alt/50 text-xs">{secondary}</p>
          )}
          <p className="text-foreground-alt/50 font-mono text-xs">
            {truncatePeerId(peerId)}
          </p>
        </div>
      </div>
      <div className="flex gap-1">
        {isPasswordMethod(method) && (
          <DashboardButton
            icon={<LuLock className="size-3" />}
            onClick={onChangePassword}
          >
            Change
          </DashboardButton>
        )}
        {isRemovableAuthMethod(method) && (
          <DashboardButton
            icon={<LuTrash2 className="size-3" />}
            disabled={!hasOtherMethods}
            variant={hasOtherMethods ? 'destructive' : undefined}
            onClick={() => onRemove(peerId, label)}
          >
            Remove
          </DashboardButton>
        )}
      </div>
    </div>
  )
}

// AuthMethodPickerDialog lets the user choose which type of auth method to add.
function AuthMethodPickerDialog({
  open,
  onOpenChange,
  onPick,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onPick: (method: AuthMethodType) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add auth method</DialogTitle>
          <DialogDescription>
            Choose which type of auth method to add.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <MethodOption
            label="Backup key (.pem)"
            description="Generate a key file for offline recovery"
            onClick={() => onPick('pem')}
          />
          <MethodOption
            label="Passkey"
            description="Use biometrics or a hardware key"
            onClick={() => onPick('passkey')}
          />
          <MethodOption
            label="Google"
            description="Link your Google identity"
            icon={<FcGoogle className="size-4" />}
            onClick={() => onPick('google')}
          />
          <MethodOption
            label="GitHub"
            description="Link your GitHub identity"
            icon={<LuGithub className="size-4" />}
            onClick={() => onPick('github')}
          />
        </div>
      </DialogContent>
    </Dialog>
  )
}

// SingleSigDialogs renders the confirm dialogs that remove an auth method and
// add a backup key for accounts without a multi-sig threshold.
function SingleSigDialogs({
  account,
  retainStepUp,
  removePeerId,
  addType,
  onRemoveClose,
  onAddClose,
}: {
  account: Resource<Account>
  retainStepUp: boolean
  removePeerId: string | null
  addType: AuthMethodType | null
  onRemoveClose: () => void
  onAddClose: () => void
}) {
  const handleRemove = async (credential: AuthCredential) => {
    if (!removePeerId || !account.value) return
    await account.value.removeAuthMethod({
      peerId: removePeerId,
      credential: buildEntityCredential(credential),
    })
  }

  const handleAddBackupKey = async (credential: AuthCredential) => {
    if (!account.value) return
    let resp
    try {
      resp = await account.value.generateBackupKey({
        credential: buildEntityCredential(credential),
      })
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err)
      if (msg.includes('unknown_keypair')) {
        throw new Error(
          'Incorrect password or unrecognized key. Please try again.',
          { cause: err },
        )
      }
      throw err
    }
    const pemData = resp.pemData
    if (!pemData || pemData.length === 0) return

    downloadPemFile(pemData)
  }

  return (
    <>
      {/* Remove auth method dialog */}
      <AuthConfirmDialog
        open={removePeerId !== null}
        onOpenChange={(open) => {
          if (!open) onRemoveClose()
        }}
        title="Remove auth method"
        description="Confirm your identity to remove this auth method. This cannot be undone."
        confirmLabel="Remove"
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_REMOVE_AUTH_METHOD,
          title: 'Remove auth method',
          description:
            'Confirm your identity to remove this auth method. This cannot be undone.',
          targetPeerId: removePeerId ?? undefined,
        }}
        onConfirm={handleRemove}
        account={account}
        retainAfterClose={retainStepUp}
      />

      {/* Add backup key dialog */}
      <AuthConfirmDialog
        open={addType === 'pem'}
        onOpenChange={(open) => {
          if (!open) onAddClose()
        }}
        title="Add backup key"
        description="Confirm your identity to generate and register a backup key. The .pem file will download automatically."
        confirmLabel={
          <>
            <LuDownload className="inline size-3.5" /> Generate and download
          </>
        }
        intent={{
          kind: AccountEscalationIntentKind.AccountEscalationIntentKind_ACCOUNT_ESCALATION_INTENT_KIND_ADD_BACKUP_KEY,
          title: 'Add backup key',
          description:
            'Confirm your identity to generate and register a backup key. The .pem file will download automatically.',
        }}
        onConfirm={handleAddBackupKey}
        account={account}
        retainAfterClose={retainStepUp}
      />
    </>
  )
}

function AuthMethodIcon({ method }: { method: AccountAuthMethod }) {
  switch (getAuthMethodKind(method)) {
    case AccountAuthMethodKind.GOOGLE_SSO:
      return <FcGoogle className="size-4" />
    case AccountAuthMethodKind.GITHUB_SSO:
      return <LuGithub className="size-4" />
    case AccountAuthMethodKind.PASSKEY:
      return <LuFingerprint className="text-foreground-alt size-4" />
    default:
      return <LuKey className="text-foreground-alt size-4" />
  }
}

function getAuthMethodKind(method: AccountAuthMethod): AccountAuthMethodKind {
  if (method.kind !== undefined) {
    return method.kind
  }
  switch (method.provider ?? method.keypair?.authMethod ?? '') {
    case 'google':
    case 'google_sso':
      return AccountAuthMethodKind.GOOGLE_SSO
    case 'github':
    case 'github_sso':
      return AccountAuthMethodKind.GITHUB_SSO
    case 'password':
      return AccountAuthMethodKind.PASSWORD
    case 'pem':
      return AccountAuthMethodKind.BACKUP_KEY
    case 'passkey':
    case 'webauthn':
      return AccountAuthMethodKind.PASSKEY
    default:
      return AccountAuthMethodKind.UNKNOWN
  }
}

function isPasswordMethod(method: AccountAuthMethod): boolean {
  return getAuthMethodKind(method) === AccountAuthMethodKind.PASSWORD
}

function isRemovableAuthMethod(method: AccountAuthMethod): boolean {
  switch (getAuthMethodKind(method)) {
    case AccountAuthMethodKind.PASSWORD:
    case AccountAuthMethodKind.BACKUP_KEY:
    case AccountAuthMethodKind.PASSKEY:
    case AccountAuthMethodKind.GOOGLE_SSO:
    case AccountAuthMethodKind.GITHUB_SSO:
      return !!(method.peerId ?? method.keypair?.peerId)
    default:
      return false
  }
}

// MethodOption renders a selectable auth method type in the picker.
function MethodOption({
  label,
  description,
  icon,
  disabled,
  onClick,
}: {
  label: string
  description: string
  icon?: React.ReactNode
  disabled?: boolean
  onClick?: () => void
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'border-foreground/6 bg-background-card/30 w-full rounded-md border px-3 py-2.5 text-left transition-colors',
        disabled
          ? 'cursor-not-allowed opacity-50'
          : 'hover:border-foreground/12 hover:bg-background-card/50',
      )}
    >
      <div className="flex items-center gap-2">
        {icon}
        <p className="text-foreground text-sm font-medium">
          {label}
          {disabled && (
            <span className="text-foreground-alt ml-2 text-xs font-normal">
              Coming soon
            </span>
          )}
        </p>
      </div>
      <p className="text-foreground-alt text-xs">{description}</p>
    </button>
  )
}
