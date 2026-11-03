import { useState } from 'react'
import {
  LuDownload,
  LuFingerprint,
  LuGithub,
  LuKey,
  LuLock,
  LuPlus,
  LuTrash2,
} from '@s4wave/web/images/icons.js'
import { FcGoogle } from '@s4wave/web/images/icons.js'

import { downloadPemFile } from '@s4wave/web/download.js'

import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import {
  CollapsibleSection,
  collapsibleSectionActionStyles,
} from '@s4wave/web/ui/CollapsibleSection.js'
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

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    padding: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule4: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
  },
  rule5: {
    display: 'flex',
    minWidth: 0,
    flex: 1,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 8)`,
    height: `calc(${themeLayout['--spacing']} * 8)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule7: { minWidth: 0, flex: 1 },
  rule8: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule9: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule10: { display: 'flex', gap: themeLayout['--spacing'] },
  rule11: {
    width: '100%',
    cursor: 'not-allowed',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    opacity: 0.5,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule12: {
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 12%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2.5)`,
    textAlign: 'left',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule13: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule14: {
    marginLeft: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-normal)',
    fontWeight: 'var(--font-weight-normal)',
    color: themeColors['--color-foreground-alt'],
  },
  rule15: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule4: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule5: {
    display: 'inline',
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule6: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

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
  // Resolve expansion from the controlled input or the saved session setting.
  const ns = useStateNamespace(['session-settings'])
  const [storedOpen, setStoredOpen] = useStateAtom(ns, 'auth-methods', false)
  const sectionOpen = open ?? storedOpen
  const handleOpenChange = onOpenChange ?? setStoredOpen

  // Select single-signature dialogs or the threshold-based credential wizard.
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

  // Route removal to the credential flow selected by the account threshold.
  const handleRemoveClick = (peerId: string, methodLabel: string) => {
    if (useWizard) {
      setWizardRemove({ peerId, label: methodLabel })
    } else {
      setRemovePeerId(peerId)
    }
  }

  // Close the picker and open the selected method's credential flow.
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
        icon={<LuKey sx={stylexStyles.rule1} />}
        open={sectionOpen}
        onOpenChange={handleOpenChange}
        badge={
          keypairCount > 0 ? (
            <span sx={stylexStyles2.rule1}>{keypairCount}</span>
          ) : undefined
        }
        headerActions={
          <button
            type="button"
            onClick={() => setPickerOpen(true)}

            sx={collapsibleSectionActionStyles.button}
            aria-label="Add auth method"
            title="Add auth method"
          >
            <LuPlus sx={stylexStyles.rule1} />
          </button>
        }
      >
        {authMethodsResource.loading && (
          <p sx={stylexStyles2.rule2}>Loading auth methods…</p>
        )}
        {!authMethodsResource.loading && authMethods.length === 0 && (
          <div sx={stylexStyles2.rule3}>
            <LuKey sx={stylexStyles.rule2} />
            <span>No auth methods found</span>
          </div>
        )}
        {authMethods.length > 0 && (
          <div>
            {authMethods.map((method) => (
              <AuthMethodRow
                sx={nativeChildSpacing.s2}
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
  sx,
  method,
  hasOtherMethods,
  onChangePassword,
  onRemove,
}: {
  method: AccountAuthMethod
  hasOtherMethods: boolean
  onChangePassword: () => void
  onRemove: (peerId: string, label: string) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const peerId = method.peerId ?? method.keypair?.peerId ?? 'unknown'
  const label = method.label ?? 'Auth method'
  const secondary = method.secondaryLabel ?? ''

  return (
    <div sx={[stylexStyles2.rule4, sx]}>
      <div sx={stylexStyles2.rule5}>
        <div sx={stylexStyles2.rule6}>
          <AuthMethodIcon method={method} />
        </div>
        <div sx={stylexStyles2.rule7}>
          <p sx={stylexStyles2.rule8}>{label}</p>
          {secondary && <p sx={stylexStyles2.rule1}>{secondary}</p>}
          <p sx={stylexStyles2.rule9}>{truncatePeerId(peerId)}</p>
        </div>
      </div>
      <div sx={stylexStyles2.rule10}>
        {isPasswordMethod(method) && (
          <DashboardButton
            icon={<LuLock sx={stylexStyles.rule3} />}
            onClick={onChangePassword}
          >
            Change
          </DashboardButton>
        )}
        {isRemovableAuthMethod(method) && (
          <DashboardButton
            icon={<LuTrash2 sx={stylexStyles.rule3} />}
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
        <div>
          <MethodOption
            sx={nativeChildSpacingNext.s2}
            label="Backup key (.pem)"
            description="Generate a key file for offline recovery"
            onClick={() => onPick('pem')}
          />
          <MethodOption
            sx={nativeChildSpacingNext.s2}
            label="Passkey"
            description="Use biometrics or a hardware key"
            onClick={() => onPick('passkey')}
          />
          <MethodOption
            sx={nativeChildSpacingNext.s2}
            label="Google"
            description="Link your Google identity"
            icon={<FcGoogle sx={stylexStyles.rule4} />}
            onClick={() => onPick('google')}
          />
          <MethodOption
            sx={nativeChildSpacingNext.s2}
            label="GitHub"
            description="Link your GitHub identity"
            icon={<LuGithub sx={stylexStyles.rule4} />}
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
    // Generate the backup key on the live account and translate an unknown credential.
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

    // Download a generated key only when the response carries its PEM bytes.
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
            <LuDownload sx={stylexStyles.rule5} /> Generate and download
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
      return <FcGoogle sx={stylexStyles.rule4} />
    case AccountAuthMethodKind.GITHUB_SSO:
      return <LuGithub sx={stylexStyles.rule4} />
    case AccountAuthMethodKind.PASSKEY:
      return <LuFingerprint sx={stylexStyles.rule6} />
    default:
      return <LuKey sx={stylexStyles.rule6} />
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
  sx,
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
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      sx={[disabled ? stylexStyles2.rule11 : stylexStyles2.rule12, sx]}
    >
      <div sx={stylexStyles2.rule13}>
        {icon}
        <p sx={stylexStyles2.rule8}>
          {label}
          {disabled && <span sx={stylexStyles2.rule14}>Coming soon</span>}
        </p>
      </div>
      <p sx={stylexStyles2.rule15}>{description}</p>
    </button>
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
