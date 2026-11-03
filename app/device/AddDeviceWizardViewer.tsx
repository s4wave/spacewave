import { useCallback, useId, useMemo, useState, type ReactNode } from 'react'
import {
  LuCircleCheck,
  LuClipboardCheck,
  LuHardDrive,
  LuKeyRound,
  LuLogIn,
  LuServer,
  LuTerminal,
} from '@s4wave/web/images/icons.js'

import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  SpaceLinkCallback,
  SpaceLinkCompletionMode,
} from '@s4wave/sdk/provider/spacewave/spacewave.pb.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import {
  SessionContext,
  SharedObjectContext,
  SpaceContext,
} from '@s4wave/web/contexts/contexts.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { CopyButton } from '@s4wave/web/ui/CopyButton.js'
import { Button } from '@s4wave/web/ui/button.js'

import { parseObjectUri } from '@s4wave/sdk/space/object-uri.js'
import { DeviceTypeID } from '@s4wave/sdk/device/device.js'
import {
  SecretKindSSHPassword,
  SecretKindSSHPrivateKey,
  SecretKindSSHPassphrase,
  SSHPrivateKeyContentType,
  SSHTextCredentialContentType,
} from '@s4wave/sdk/secret/secret.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { Space } from '@s4wave/sdk/space/space.js'
import type { IWorldState } from '@s4wave/sdk/world/world-state.js'
import { CREATE_SSH_HOST_OP_ID } from '@s4wave/sdk/sshhost/create-ssh-host.js'
import {
  CreateSshHostOp,
  type SshHost,
  type SshHostCredentialRefs,
  type SshHostKeyPin,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import { useWorldQuery } from '@s4wave/web/hooks/useWorldQuery.js'
import { useNavigate } from '@s4wave/web/router/router.js'

import { applySpaceIndexPath } from '../space/space-settings.js'
import { buildObjectKey } from '../space/create-op-builders.js'
import { useWizardState } from '../wizard/useWizardState.js'
import type { UseWizardStateResult } from '../wizard/useWizardState.js'
import { WizardShell } from '../wizard/WizardShell.js'
import { WizardField } from '../wizard/WizardField.js'
import { WizardTextareaField } from '../wizard/WizardTextareaField.js'
import { ComputersDashboardTypeID } from './computers.js'
import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import { fonts as themeFonts } from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
})

const stylexStyles4 = stylex.create({
  rule1: {
    display: 'grid',
    gridTemplateColumns: {
      default: 'repeat(1, minmax(0, 1fr))',
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule2: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule5: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule6: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule7: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule8: {
    display: 'flex',
    width: '100%',
    minWidth: 0,
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: themeLayout['--spacing'],
    textAlign: 'left',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    backgroundColor: {
      default: null,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
  },
  rule9: { textOverflow: 'ellipsis', whiteSpace: 'nowrap', overflow: 'hidden' },
  rule10: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
  },
  rule11: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule12: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 5%, transparent)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule13: {
    display: 'flex',
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
  },
  rule14: { width: '100%', maxWidth: 'var(--container-sm)' },
  rule15: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'left',
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule16: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
    color: themeColors['--color-foreground'],
  },
  rule17: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-background-card']} 50%, transparent)`,
      },
    },
    color: themeColors['--color-foreground-alt'],
  },
  rule18: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 7)`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: themeRadii['--radius-md'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule19: { minWidth: 0 },
  rule20: {
    display: 'block',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule21: {
    display: 'block',
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule22: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule23: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule24: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'minmax(8rem, .3fr) minmax(0, 1fr)',
    },
  },
  rule25: {
    marginBottom: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule26: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'center',
    justifyContent: 'space-between',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule27: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule28: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule29: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule30: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'minmax(8rem, .35fr) minmax(0, 1fr)',
    },
  },
  rule31: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule32: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule33: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule34: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
  },
  rule35: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule36: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    transitionProperty: 'all',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule37: { cursor: 'not-allowed', opacity: 0.5 },
  rule38: {
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
      },
    },
    color: themeColors['--color-foreground-alt'],
  },
  rule39: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule40: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule41: {
    overflowX: 'auto',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 40%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: '.7rem',
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule42: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule43: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule44: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    minHeight: `calc(${themeLayout['--spacing']} * 24)`,
    width: '100%',
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
    },
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 20%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
    '::placeholder': {
      color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    },
    '--tw-ring-color': {
      default: null,
      ':focus-visible': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
    },
    opacity: { default: null, ':disabled': 0.6 },
  },
  rule45: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    display: 'grid',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule46: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule47: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
  },
  rule48: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 5%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-destructive'],
  },
  rule49: { marginTop: `calc(${themeLayout['--spacing']} * .5)` },
})

const stylexStyles3 = stylex.create({
  rule1: { minHeight: `calc(${themeLayout['--spacing']} * 28)` },
  rule2: {
    minHeight: `calc(${themeLayout['--spacing']} * 16)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    marginRight: `calc(${themeLayout['--spacing']} * 2)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
})

const stylexStyles = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 3)` },
})

const DEFAULT_SSH_PORT = 22
const DEVICE_APPROVAL_CLOUD_REQUIRED =
  'Device linking requires a Spacewave Cloud session. Sign in or create an account to continue.'
const INSTALL_AGENT_UNAVAILABLE =
  'Install Agent is unavailable until secure bootstrap is configured.'

const noKeys: string[] = []

type AddDeviceWizardMode = 'spacelink' | 'ssh'
type SshAuthMode = 'password' | 'private-key'
type SshSetupMode = 'host' | 'install-agent'

interface SshHostWizardConfig {
  host?: string
  port?: number
  username?: string
  authMode?: SshAuthMode
  setupMode?: SshSetupMode
  hostKeyAlgorithm?: string
  hostKeyFingerprint?: string
  hostKeyPublicKey?: string
}

interface SshCredentialDraft {
  password: string
  privateKey: string
  passphrase: string
}

interface AddDeviceWizardConfig {
  mode?: AddDeviceWizardMode
  ticket?: string
  completion?: string
  preview?: {
    label?: string
    agentPeerId?: string
    targetHint?: string
    expiresAt?: string
  }
  ssh?: SshHostWizardConfig
}

export { AddDeviceWizardTypeID } from './add-device-wizard.js'

// useAddDeviceWizardController owns wizard persistence, approval, and SSH
// object creation.
function useAddDeviceWizardController(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws
  const currentStep = state?.step ?? 0
  const space = useResourceValue(SpaceContext.useContext())
  const session = useResourceValue(SessionContext.useContext()) as
    | Session
    | undefined
  const {
    sessionInfo,
    loading: sessionInfoLoading,
    supportsDeviceApproval,
  } = useSessionInfo(session)
  const sharedObject = useResourceValue(SharedObjectContext.useContext())
  const sharedObjectId = sharedObject?.meta.sharedObjectId ?? ''
  const { spaceWorldResource } = SpaceContainerContext.useContext()
  const navigate = useNavigate()
  const handleSignIn = useCallback(() => {
    navigate({ path: '/login' })
  }, [navigate])
  const [busy, setBusy] = useState(false)
  const [approvalError, setApprovalError] = useState('')
  const [operationError, setOperationError] = useState('')
  const [openingStep, setOpeningStep] = useState(false)
  const isOpeningStep = openingStep && currentStep === 0
  const [sshCredentialDraft, setSshCredentialDraft] =
    useState<SshCredentialDraft>({
      password: '',
      privateKey: '',
      passphrase: '',
    })
  const config = useMemo(() => decodeConfig(ws.configData), [ws.configData])
  const mode = config.mode ?? 'spacelink'
  const sshConfig = useMemo(() => config.ssh ?? {}, [config.ssh])
  const sshAuthMode = sshConfig.authMode ?? 'password'
  const sshSetupMode = sshConfig.setupMode ?? 'host'
  const isSshInstallMode = mode === 'ssh' && sshSetupMode === 'install-agent'
  const ticket = config.ticket ?? ''
  const completion = config.completion ?? ''
  const setupCommand = buildSetupCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const installSetupCommand = buildInstallSetupCommand(
    ws.localName || 'Device',
    sharedObjectId,
  )
  const completeCommand = completion
    ? `spacewave device complete --completion ${quoteShellArg(completion)}`
    : 'spacewave device complete --completion <completion>'
  const computers = useWorldQuery(
    spaceWorldResource,
    async (world, signal) => {
      const [devices, dashboards] = await Promise.all([
        world.listObjectsWithType(DeviceTypeID, signal),
        world.listObjectsWithType(ComputersDashboardTypeID, signal),
      ])
      return { devices, dashboards }
    },
    [],
  ).value
  const deviceKeys = computers?.devices ?? noKeys
  const dashboardKey = computers?.dashboards[0] ?? ''

  const handleConfigUpdate = useCallback(
    async (next: AddDeviceWizardConfig, step?: number) => {
      ws.handleConfigDataChange(encodeConfig(next))
      const handle = ws.wizardResource.value
      if (!handle) return
      const update: { configData: Uint8Array; step?: number } = {
        configData: encodeConfig(next),
      }
      if (step !== undefined) update.step = step
      await handle.updateState(update)
    },
    [ws],
  )

  const handleModeChange = useCallback(
    (mode: AddDeviceWizardMode) => {
      ws.handleConfigDataChange(encodeConfig({ ...config, mode }))
    },
    [config, ws],
  )

  const handleTicketChange = useCallback(
    (value: string) => {
      ws.handleConfigDataChange(
        encodeConfig({ ...config, mode, ticket: value }),
      )
    },
    [config, mode, ws],
  )

  const handleSshConfigChange = useCallback(
    (patch: Partial<SshHostWizardConfig>) => {
      if (patch.setupMode && patch.setupMode !== 'host') {
        setOperationError(INSTALL_AGENT_UNAVAILABLE)
        toast.error(INSTALL_AGENT_UNAVAILABLE)
        return
      }
      ws.handleConfigDataChange(
        encodeConfig({
          ...config,
          mode: 'ssh',
          ssh: { ...sshConfig, ...patch },
        }),
      )
    },
    [config, sshConfig, ws],
  )

  const handleNameNext = useCallback(async () => {
    const handle = ws.wizardResource.value
    if (!handle) {
      setOperationError('Device setup is still loading. Try again.')
      return
    }
    setOperationError('')
    setOpeningStep(true)
    try {
      await ws.persistDraftState()
      await handle.updateState({ step: 1 })
    } catch {
      setOpeningStep(false)
      setOperationError(
        mode === 'ssh'
          ? 'SSH setup could not be opened. Try again.'
          : 'Device setup could not be opened. Try again.',
      )
    }
  }, [mode, ws])

  const handleFinishSpaceLink = useCallback(async () => {
    if (!state || ws.creating) return
    ws.setCreating(true)
    try {
      await ws.persistDraftState()
      if (dashboardKey) {
        await replaceSpaceIndexIfWizardIsCurrent(ws, dashboardKey)
      }
      await ws.spaceWorld.deleteObject(ws.objectKey)
      const deviceKey = deviceKeys[0] ?? ''
      const openKey = deviceKey || dashboardKey
      if (openKey) ws.navigateToObjects([openKey])
    } catch {
      const message = 'Device setup could not be completed. Try again.'
      setOperationError(message)
      toast.error(message)
    } finally {
      ws.setCreating(false)
    }
  }, [dashboardKey, deviceKeys, state, ws])

  const createSshHostTerminalObjects = useCallback(
    async (terminalCommand?: string) => {
      if (!state) throw new Error('Wizard state is not ready')
      if (!space) throw new Error('Space resource is not ready')
      const readerPublicKeyPem =
        sessionInfo?.cryptoInfo?.publicKeyPem?.trim() ?? ''
      if (!readerPublicKeyPem) {
        throw new Error('Session public key is not available')
      }

      const label = (ws.localName || state.name || 'SSH Host').trim()
      const timestamp = new Date()
      const [hostObjectKey, credentials] = await Promise.all([
        buildObjectKey(ws.spaceWorld, 'ssh-host/', label),
        createSshCredentialSecrets({
          space,
          label,
          config: sshConfig,
          draft: sshCredentialDraft,
          world: ws.spaceWorld,
          readerPublicKeyPem,
        }),
      ])
      const hostKeyPins = buildSshHostKeyPins(
        sshConfig,
        ws.sessionPeerId,
        timestamp,
      )
      const endpoint = {
        host: sshConfig.host?.trim(),
        port: normalizeSshPort(sshConfig.port),
        username: sshConfig.username?.trim(),
      }
      const host: SshHost = {
        label,
        endpoint,
        credentials,
        hostKeyPins,
        createdAt: timestamp,
        updatedAt: timestamp,
      }
      await ws.spaceWorld.applyWorldOp(
        CREATE_SSH_HOST_OP_ID,
        CreateSshHostOp.toBinary({
          objectKey: hostObjectKey,
          label,
          endpoint,
          credentials,
          hostKeyPins,
          timestamp,
        }),
        ws.sessionPeerId,
      )

      const terminal = await buildCreateSshHostTerminalOpData({
        host,
        hostObjectKey,
        world: ws.spaceWorld,
        command: terminalCommand,
      })
      if (terminal) {
        await ws.spaceWorld.applyWorldOp(
          CREATE_TERMINAL_OP_ID,
          terminal.opData,
          ws.sessionPeerId,
        )
      }
      return {
        hostObjectKey,
        terminalObjectKey: terminal?.objectKey ?? '',
      }
    },
    [
      sessionInfo?.cryptoInfo?.publicKeyPem,
      space,
      sshConfig,
      sshCredentialDraft,
      state,
      ws,
    ],
  )

  const handleCreateSshHost = useCallback(async () => {
    if (!state || ws.creating) return
    const error = getSshCreateError(sshConfig, sshCredentialDraft)
    if (error) {
      setOperationError(error)
      toast.error(error)
      return
    }

    setOperationError('')
    ws.setCreating(true)
    try {
      const { hostObjectKey, terminalObjectKey } =
        await createSshHostTerminalObjects()
      if (dashboardKey) {
        await replaceSpaceIndexIfWizardIsCurrent(ws, dashboardKey)
      }
      await ws.spaceWorld.deleteObject(ws.objectKey)
      toast.success('SSH Host added')
      ws.navigateToObjects([terminalObjectKey || hostObjectKey])
    } catch {
      const message = 'SSH Host could not be added. Try again.'
      setOperationError(message)
      toast.error(message)
    } finally {
      ws.setCreating(false)
    }
  }, [
    createSshHostTerminalObjects,
    dashboardKey,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  ])

  const handleFinalize = useCallback(() => {
    if (isSshInstallMode) {
      setOperationError(INSTALL_AGENT_UNAVAILABLE)
      toast.error(INSTALL_AGENT_UNAVAILABLE)
      return
    }
    if (mode === 'ssh') {
      void handleCreateSshHost()
      return
    }
    void handleFinishSpaceLink()
  }, [handleCreateSshHost, handleFinishSpaceLink, isSshInstallMode, mode])

  const handleCancel = useCallback(() => {
    void ws.handleCancel()
  }, [ws])

  return {
    approvalError,
    busy,
    completeCommand,
    completion,
    config,
    currentStep,
    deviceKeys,
    handleCancel,
    handleConfigUpdate,
    handleFinalize,
    handleModeChange,
    handleNameNext,
    handleSignIn,
    handleSshConfigChange,
    handleTicketChange,
    isOpeningStep,
    isSshInstallMode,
    installSetupCommand,
    mode,
    operationError,
    session,
    sessionInfoLoading,
    setApprovalError,
    setBusy,
    setOpeningStep,
    setOperationError,
    setSshCredentialDraft,
    setupCommand,
    sharedObjectId,
    sshAuthMode,
    sshConfig,
    sshCredentialDraft,
    sshSetupMode,
    state,
    supportsDeviceApproval,
    ticket,
    ws,
  }
}

type AddDeviceWizardController = ReturnType<typeof useAddDeviceWizardController>

// AddDeviceWizardSteps renders the connection-specific step bodies from the
// controller projection.
function AddDeviceWizardSteps({
  sx,
  controller,
}: {
  controller: AddDeviceWizardController
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const {
    approvalError,
    busy,
    completeCommand,
    config,
    currentStep,
    deviceKeys,
    handleConfigUpdate,
    handleModeChange,
    handleSignIn,
    handleSshConfigChange,
    handleTicketChange,
    isOpeningStep,
    installSetupCommand,
    mode,
    operationError,
    session,
    sessionInfoLoading,
    setApprovalError,
    setBusy,
    setSshCredentialDraft,
    setupCommand,
    sharedObjectId,
    sshAuthMode,
    sshConfig,
    sshCredentialDraft,
    sshSetupMode,
    supportsDeviceApproval,
    ticket,
    ws,
  } = controller

  return (
    <>
      {currentStep === 0 && (
        <section sx={[stylexStyles4.rule1, sx]}>
          <ModeButton
            active={mode === 'spacelink'}
            icon={<LuHardDrive sx={stylexStyles2.rule1} />}
            label="Managed Device"
            detail="Install Spacewave for persistent status, policy, and capabilities. 3 steps"
            onClick={() => handleModeChange('spacelink')}
          />
          <ModeButton
            active={mode === 'ssh'}
            icon={<LuServer sx={stylexStyles2.rule1} />}
            label="SSH Host"
            detail="Connect on demand over SSH without installing an agent. 2 steps"
            onClick={() => handleModeChange('ssh')}
          />
        </section>
      )}
      {currentStep === 1 && (
        <>
          {mode === 'spacelink' && (
            <section sx={[false, sx]}>
              <SpaceLinkApprovalPanel
                sx={nativeChildSpacingNextNext.s3}
                supported={supportsDeviceApproval}
                loading={sessionInfoLoading}
                onSignIn={handleSignIn}
                config={config}
                ticket={ticket}
                setupCommand={setupCommand}
                installSetupCommand={installSetupCommand}
                busy={busy}
                error={approvalError}
                onTicketChange={(value) => {
                  setApprovalError('')
                  handleTicketChange(value)
                }}
                onApprove={() => {
                  setApprovalError('')
                  void approveTicket({
                    ws,
                    sharedObjectId,
                    ticket,
                    config,
                    session,
                    setBusy,
                    setError: setApprovalError,
                    persist: handleConfigUpdate,
                  })
                }}
              />
            </section>
          )}
          {mode === 'ssh' && (
            <section sx={sx}>
              <SshHostSetupForm
                sx={nativeChildSpacingNext.s3}
                config={sshConfig}
                credentialDraft={sshCredentialDraft}
                authMode={sshAuthMode}
                setupMode={sshSetupMode}
                onConfigChange={handleSshConfigChange}
                onCredentialChange={(patch) =>
                  setSshCredentialDraft((prev) => ({ ...prev, ...patch }))
                }
              />
            </section>
          )}
        </>
      )}
      {mode === 'spacelink' && currentStep === 2 && (
        <section sx={sx}>
          <div sx={[nativeChildSpacingNext.s3, stylexStyles4.rule2]}>
            <div sx={stylexStyles4.rule3}>
              <LuCircleCheck sx={stylexStyles2.rule2} />
              Approval ready
            </div>
            <p sx={stylexStyles4.rule4}>
              Import the completion on the Device daemon, then open the Device
              object after it appears.
            </p>
          </div>
          <CommandPanel
            sx={nativeChildSpacingNext.s3}
            title="Complete setup"
            command={completeCommand}
            icon={<LuTerminal sx={stylexStyles2.rule1} />}
          />
          {deviceKeys.length > 0 && (
            <div sx={[nativeChildSpacingNext.s3, stylexStyles4.rule5]}>
              <div sx={stylexStyles4.rule6}>Device Objects</div>
              <div sx={stylexStyles4.rule7}>
                {deviceKeys.map((deviceKey) => (
                  <button
                    key={deviceKey}
                    type="button"
                    onClick={() => ws.navigateToObjects([deviceKey])}
                    sx={[stylexStyles4.rule8, nativeChildSpacing.s1]}
                  >
                    <span sx={stylexStyles4.rule9}>{deviceKey}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </section>
      )}
      {(operationError || isOpeningStep) && (
        <div
          sx={[
            [
              stylexStyles4.rule10,
              operationError ? stylexStyles4.rule11 : stylexStyles4.rule12,
            ],
            sx,
          ]}
          role={operationError ? 'alert' : 'status'}
        >
          {operationError ||
            (mode === 'ssh' ? 'Opening SSH setup…' : 'Preparing Device setup…')}
        </div>
      )}
    </>
  )
}

interface WizardCopy {
  totalSteps: number
  stepName: string
  nameLabel: string
  createLabel: string
  creatingLabel: string
  nextBusyLabel: string
}

/** wizardCopy returns the step count and labels for the connection mode and step. */
function wizardCopy(
  mode: AddDeviceWizardMode,
  currentStep: number,
  isSshInstallMode: boolean,
): WizardCopy {
  if (mode === 'ssh') {
    return {
      totalSteps: currentStep === 0 ? 3 : 2,
      stepName: currentStep === 0 ? 'Choose connection' : 'Configure SSH',
      nameLabel: 'Host Name',
      createLabel: isSshInstallMode
        ? 'Install Agent unavailable'
        : 'Add SSH Host and open terminal',
      creatingLabel: isSshInstallMode
        ? 'Install Agent unavailable'
        : 'Adding SSH Host and opening terminal…',
      nextBusyLabel: 'Opening SSH setup…',
    }
  }
  const stepNames = ['Choose connection', 'Set up Device', 'Finish']
  return {
    totalSteps: 3,
    stepName: stepNames[Math.min(currentStep, 2)] ?? 'Finish',
    nameLabel: 'Device Name',
    createLabel: 'Open Device',
    creatingLabel: 'Opening Device…',
    nextBusyLabel: 'Preparing Device setup…',
  }
}

// AddDeviceWizardContent selects the loading state and configures the shared
// wizard shell.
function AddDeviceWizardContent({
  controller,
}: {
  controller: AddDeviceWizardController
}) {
  const {
    completion,
    currentStep,
    handleCancel,
    handleFinalize,
    handleNameNext,
    isOpeningStep,
    isSshInstallMode,
    mode,
    setOpeningStep,
    setOperationError,
    sshConfig,
    sshCredentialDraft,
    state,
    ws,
  } = controller

  if (!state) {
    return (
      <div sx={stylexStyles4.rule13}>
        <div sx={stylexStyles4.rule14}>
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading Add Device',
              detail: 'Preparing the Device setup wizard.',
            }}
          />
        </div>
      </div>
    )
  }

  const copy = wizardCopy(mode, currentStep, isSshInstallMode)
  return (
    <WizardShell
      title={
        <>
          <LuHardDrive sx={stylexStyles2.rule3} />
          Add Device
        </>
      }
      step={currentStep}
      totalSteps={copy.totalSteps}
      stepName={copy.stepName}
      localName={ws.localName || 'Device'}
      onUpdateName={ws.handleUpdateName}
      onBack={() => {
        setOpeningStep(false)
        setOperationError('')
        void ws.handleBack()
      }}
      onCancel={handleCancel}
      nameLabel={copy.nameLabel}
      namePlaceholder="Build server"
      nameStep={0}
      creating={ws.creating}
      createLabel={copy.createLabel}
      creatingLabel={copy.creatingLabel}
      onFinalize={handleFinalize}
      canFinalize={
        mode === 'ssh'
          ? !isSshInstallMode &&
            !getSshCreateError(sshConfig, sshCredentialDraft)
          : !!completion
      }
      onNext={currentStep === 0 ? () => void handleNameNext() : undefined}
      nextBusyLabel={copy.nextBusyLabel}
      nextBusy={isOpeningStep}
      canNext={!!ws.localName.trim() && !isOpeningStep}
      finalizeStep={mode === 'ssh' ? 1 : 2}
      width={mode === 'ssh' ? 'wide' : 'default'}
    >
      {(childSx) => (
        <>
          <AddDeviceWizardSteps sx={childSx} controller={controller} />
        </>
      )}
    </WizardShell>
  )
}

export function AddDeviceWizardViewer(props: ObjectViewerComponentProps) {
  const controller = useAddDeviceWizardController(props)
  return <AddDeviceWizardContent controller={controller} />
}

function ModeButton({
  active,
  icon,
  label,
  detail,
  onClick,
}: {
  active: boolean
  icon: ReactNode
  label: string
  detail: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      sx={[
        stylexStyles4.rule15,
        active ? stylexStyles4.rule16 : stylexStyles4.rule17,
      ]}
    >
      <span sx={stylexStyles4.rule18}>{icon}</span>
      <span sx={stylexStyles4.rule19}>
        <span sx={stylexStyles4.rule20}>{label}</span>
        <span sx={stylexStyles4.rule21}>{detail}</span>
      </span>
    </button>
  )
}

/** hostKeyTrustMode describes how the host key of the SSH host is verified. */
function hostKeyTrustMode(config: SshHostWizardConfig): string {
  const fingerprint = config.hostKeyFingerprint?.trim()
  return fingerprint ? `Pinned to ${fingerprint}` : 'Ask on first connection'
}

/** fieldError renders an inline validation message, or nothing when valid. */
function fieldError(message: string): ReactNode {
  if (!message) return undefined
  return <span sx={stylexStyles4.rule22}>{message}</span>
}

function SshHostSetupForm({
  sx,
  config,
  credentialDraft,
  authMode,
  setupMode,
  onConfigChange,
  onCredentialChange,
}: {
  config: SshHostWizardConfig
  credentialDraft: SshCredentialDraft
  authMode: SshAuthMode
  setupMode: SshSetupMode
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
  onCredentialChange: (patch: Partial<SshCredentialDraft>) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <section sx={sx}>
      <SshEndpointCard
        sx={nativeChildSpacingNext.s3}
        config={config}
        onConfigChange={onConfigChange}
      />
      <SshCredentialCard
        sx={nativeChildSpacingNext.s3}
        authMode={authMode}
        credentialDraft={credentialDraft}
        onConfigChange={onConfigChange}
        onCredentialChange={onCredentialChange}
      />
      <SshHostKeyTrustCard
        sx={nativeChildSpacingNext.s3}
        config={config}
        onConfigChange={onConfigChange}
      />
      <SshSetupModeCard
        sx={nativeChildSpacingNext.s3}
        setupMode={setupMode}
        onConfigChange={onConfigChange}
      />
      <SshReviewCard
        sx={nativeChildSpacingNext.s3}
        config={config}
        authMode={authMode}
      />
    </section>
  )
}

/** SshEndpointCard edits the host, port, and user of the SSH endpoint. */
function SshEndpointCard({
  sx,
  config,
  onConfigChange,
}: {
  config: SshHostWizardConfig
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const port = normalizeSshPort(config.port)
  const hostError = !config.host?.trim() ? 'Host is required.' : ''
  const userError = !config.username?.trim() ? 'User is required.' : ''
  const portError =
    port < 1 || port > 65535 ? 'Port must be between 1 and 65535.' : ''

  return (
    <div sx={[stylexStyles4.rule5, sx]}>
      <div sx={stylexStyles4.rule23}>
        <LuServer sx={stylexStyles2.rule1} />
        SSH Endpoint
      </div>
      <div sx={stylexStyles4.rule24}>
        <WizardField
          label="Host"
          value={config.host ?? ''}
          onChange={(e) => onConfigChange({ host: e.target.value })}
          placeholder="host.example.com"
          help={fieldError(hostError)}
        />
        <WizardField
          label="Port"
          type="number"
          min={1}
          max={65535}
          value={String(port)}
          onChange={(e) =>
            onConfigChange({ port: parsePortInput(e.target.value) })
          }
          aria-label="SSH port"
          help={fieldError(portError)}
        />
        <WizardField
          label="User"
          value={config.username ?? ''}
          onChange={(e) => onConfigChange({ username: e.target.value })}
          placeholder="user"
          help={fieldError(userError)}
        />
      </div>
    </div>
  )
}

/** SshCredentialCard picks the auth mode and edits its password or private key. */
function SshCredentialCard({
  sx,
  authMode,
  credentialDraft,
  onConfigChange,
  onCredentialChange,
}: {
  authMode: SshAuthMode
  credentialDraft: SshCredentialDraft
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
  onCredentialChange: (patch: Partial<SshCredentialDraft>) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const credentialError =
    authMode === 'private-key' && !credentialDraft.privateKey.trim()
      ? 'Private key is required.'
      : ''

  return (
    <div sx={[stylexStyles4.rule5, sx]}>
      <div sx={stylexStyles4.rule23}>
        <LuKeyRound sx={stylexStyles2.rule1} />
        SSH Credential
      </div>
      <div sx={stylexStyles4.rule25}>
        <AuthModeButton
          active={authMode === 'password'}
          label="Password"
          onClick={() => onConfigChange({ authMode: 'password' })}
        />
        <AuthModeButton
          active={authMode === 'private-key'}
          label="Private Key"
          onClick={() => onConfigChange({ authMode: 'private-key' })}
        />
      </div>
      {authMode === 'password' ? (
        <WizardField
          label="Password"
          type="password"
          value={credentialDraft.password}
          onChange={(e) => onCredentialChange({ password: e.target.value })}
          placeholder="SSH password"
          help="Leave blank if the host accepts an empty password or requires no authentication."
        />
      ) : (
        <div>
          <WizardTextareaField
            label="Private key"
            value={credentialDraft.privateKey}
            onChange={(e) => onCredentialChange({ privateKey: e.target.value })}
            placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
            fieldSx={nativeChildSpacingNext.s2}
            sx={stylexStyles3.rule1}
            help={fieldError(credentialError)}
          />
          <WizardField
            fieldSx={nativeChildSpacingNext.s2}
            label="Passphrase"
            type="password"
            value={credentialDraft.passphrase}
            onChange={(e) => onCredentialChange({ passphrase: e.target.value })}
            placeholder="Private key passphrase (optional)"
            help="Optional for unencrypted private keys."
          />
        </div>
      )}
    </div>
  )
}

/** SshHostKeyTrustCard edits the optional pinned host key. */
function SshHostKeyTrustCard({
  sx,
  config,
  onConfigChange,
}: {
  config: SshHostWizardConfig
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <details sx={[stylexStyles4.rule5, sx]}>
      <summary sx={stylexStyles4.rule26}>
        <span>Host-key trust (advanced)</span>
        <span sx={stylexStyles4.rule27}>optional</span>
      </summary>
      <p sx={stylexStyles4.rule28}>
        Trust mode:{' '}
        <span sx={stylexStyles4.rule29}>{hostKeyTrustMode(config)}</span>. Leave
        the fields empty to ask on the first connection.
      </p>
      <div sx={stylexStyles4.rule30}>
        <WizardField
          label="Algorithm"
          value={config.hostKeyAlgorithm ?? 'ssh-ed25519'}
          onChange={(e) => onConfigChange({ hostKeyAlgorithm: e.target.value })}
          placeholder="ssh-ed25519"
          variant="compactMono"
        />
        <WizardField
          label="Fingerprint"
          value={config.hostKeyFingerprint ?? ''}
          onChange={(e) =>
            onConfigChange({ hostKeyFingerprint: e.target.value })
          }
          placeholder="SHA256:..."
          variant="compactMono"
        />
      </div>
      <WizardTextareaField
        label="Public key"
        value={config.hostKeyPublicKey ?? ''}
        onChange={(e) => onConfigChange({ hostKeyPublicKey: e.target.value })}
        placeholder="[host]:22 ssh-ed25519 AAAA… or ssh-ed25519 AAAA…"
        sx={stylexStyles3.rule2}
        fieldSx={deviceWizardFieldStyles.publicKey}
      />
    </details>
  )
}

/** SshSetupModeCard shows the setup actions; installing the agent is unavailable. */
function SshSetupModeCard({
  sx,
  setupMode,
  onConfigChange,
}: {
  setupMode: SshSetupMode
  onConfigChange: (patch: Partial<SshHostWizardConfig>) => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles4.rule5, sx]}>
      <div sx={stylexStyles4.rule31}>What do you want to do?</div>
      <div sx={stylexStyles4.rule1}>
        <AuthModeButton
          active={setupMode === 'host'}
          label="Add SSH Host"
          onClick={() => onConfigChange({ setupMode: 'host' })}
        />
        <AuthModeButton
          active={setupMode === 'install-agent'}
          label="Install Agent"
          onClick={() => undefined}
          disabled
        />
      </div>
    </div>
  )
}

/** SshReviewCard summarizes the SSH host before it is created. */
function SshReviewCard({
  sx,
  config,
  authMode,
}: {
  config: SshHostWizardConfig
  authMode: SshAuthMode
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const host = config.host?.trim()
  const endpoint = host
    ? `${host}:${normalizeSshPort(config.port)}`
    : 'Host not set'

  return (
    <div sx={[stylexStyles4.rule5, sx]}>
      <div sx={stylexStyles4.rule31}>Review</div>
      <dl sx={stylexStyles4.rule32}>
        <ReviewRow label="Host label" value={host || 'Not set'} />
        <ReviewRow label="SSH endpoint" value={endpoint} mono />
        <ReviewRow
          label="Username"
          value={config.username?.trim() || 'Not set'}
        />
        <ReviewRow
          label="Credential"
          value={authMode === 'private-key' ? 'Private key' : 'Password'}
        />
        <ReviewRow label="Trust mode" value={hostKeyTrustMode(config)} />
      </dl>
    </div>
  )
}

function ReviewRow({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div sx={stylexStyles4.rule19}>
      <dt sx={stylexStyles4.rule33}>{label}</dt>
      <dd sx={[stylexStyles4.rule34, !!mono && stylexStyles4.rule35]}>
        {value}
      </dd>
    </div>
  )
}

function AuthModeButton({
  active,
  label,
  onClick,
  disabled = false,
}: {
  active: boolean
  label: string
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      disabled={disabled}
      onClick={onClick}
      sx={[
        stylexStyles4.rule36,
        !!disabled && stylexStyles4.rule37,
        active ? stylexStyles4.rule16 : stylexStyles4.rule38,
      ]}
    >
      {label}
    </button>
  )
}

function CommandPanel({
  sx,
  title,
  command,
  icon,
}: {
  title: string
  command: string
  icon: ReactNode
} & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles4.rule5, sx]}>
      <div sx={stylexStyles4.rule39}>
        <div sx={stylexStyles4.rule40}>
          {icon}
          <span>{title}</span>
        </div>
        <CopyButton text={command} label={`Copy ${title}`} />
      </div>
      <pre sx={stylexStyles4.rule41}>{command}</pre>
    </div>
  )
}

function SpaceLinkApprovalPanel({
  sx,
  config,
  ticket,
  setupCommand,
  installSetupCommand,
  busy,
  error,
  supported,
  loading,
  onTicketChange,
  onApprove,
  onSignIn,
}: {
  config: AddDeviceWizardConfig
  ticket: string
  setupCommand: string
  installSetupCommand: string
  busy: boolean
  error?: string
  supported: boolean
  loading: boolean
  onTicketChange: (value: string) => void
  onApprove: () => void
  onSignIn: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const ticketId = useId()
  if (loading) {
    return (
      <LoadingCard
        sx={sx}
        view={{
          state: 'active',
          title: 'Checking device linking',
          detail: 'Checking whether this session can approve device links.',
        }}
      />
    )
  }

  if (!supported) {
    return (
      <div sx={[stylexStyles4.rule5, sx]}>
        <div sx={stylexStyles4.rule6}>Spacewave Cloud required</div>
        <p sx={stylexStyles4.rule42}>{DEVICE_APPROVAL_CLOUD_REQUIRED}</p>
        <Button
          variant="brandOutline"
          size="toolbarWide"
          onClick={onSignIn}
          sx={stylexStyles.rule1}
        >
          <LuLogIn sx={stylexStyles2.rule1} />
          Sign in or create account
        </Button>
      </div>
    )
  }

  const ticketReady = ticket.trim().length > 0

  return (
    <div sx={sx}>
      {!ticketReady && (
        <div sx={nativeChildSpacing.s3}>
          <h3 sx={stylexStyles4.rule6}>1. Run setup on the Device</h3>
          <p sx={stylexStyles4.rule4}>
            Choose the command that matches the Device. Both print a ticket.
          </p>
          <div sx={stylexStyles5.rule1}>
            <CommandPanel
              sx={nativeChildSpacingNext.s2}
              title="Spacewave already installed"
              command={setupCommand}
              icon={<LuTerminal sx={stylexStyles2.rule1} />}
            />
            <CommandPanel
              sx={nativeChildSpacingNext.s2}
              title="Install and run"
              command={installSetupCommand}
              icon={<LuTerminal sx={stylexStyles2.rule1} />}
            />
          </div>
        </div>
      )}
      <div sx={[stylexStyles4.rule5, nativeChildSpacing.s3]}>
        <label htmlFor={ticketId} sx={stylexStyles4.rule43}>
          {ticketReady ? 'Approve Device ticket' : '2. Paste the Device ticket'}
        </label>
        <p sx={stylexStyles4.rule4}>
          {ticketReady
            ? 'The ticket is ready. Approve it to create the completion command.'
            : 'Paste the base64 ticket printed by the Device setup command.'}
        </p>
        <textarea
          id={ticketId}
          aria-label="Device ticket"
          value={ticket}
          onChange={(e) => onTicketChange(e.target.value)}
          placeholder="Paste the base64 ticket from spacewave device setup"
          disabled={busy}
          sx={stylexStyles4.rule44}
        />
        {config.preview && (
          <div sx={stylexStyles4.rule45}>
            <span>{config.preview.label || 'Device'}</span>
            {config.preview.agentPeerId && (
              <span sx={stylexStyles4.rule46}>
                {config.preview.agentPeerId}
              </span>
            )}
          </div>
        )}
        {busy && (
          <p sx={stylexStyles4.rule47}>
            Checking ticket and preparing enrollment…
          </p>
        )}
        {error && (
          <div sx={stylexStyles4.rule48} role="alert">
            <div sx={stylexStyles4.rule29}>Ticket could not be approved</div>
            <div sx={stylexStyles4.rule49}>{error}</div>
          </div>
        )}
        {ticketReady && (
          <Button
            onClick={onApprove}
            disabled={busy}
            variant="brandOutline"
            size="toolbarWide"
            sx={stylexStyles.rule1}
          >
            <LuClipboardCheck sx={stylexStyles2.rule1} />
            {busy ? 'Checking ticket…' : 'Approve'}
          </Button>
        )}
      </div>
    </div>
  )
}

async function approveTicket({
  ws,
  sharedObjectId,
  ticket,
  config,
  session,
  setBusy,
  setError,
  persist,
  nextStep = 2,
}: {
  ws: UseWizardStateResult
  sharedObjectId: string
  ticket: string
  config: AddDeviceWizardConfig
  session: Session | undefined
  setBusy: (busy: boolean) => void
  setError?: (message: string) => void
  persist: (config: AddDeviceWizardConfig, step?: number) => Promise<void>
  nextStep?: number
}) {
  if (!sharedObjectId) {
    toast.error('Open this wizard from a mounted Space before approval')
    return
  }
  if (!session) {
    toast.error('Spacewave session is not ready')
    return
  }
  setBusy(true)
  try {
    await ws.persistDraftState()
    const ticketBytes = base64ToBytes(ticket)
    const preview = await session.spacewave.previewSpaceLink({
      ticket: ticketBytes,
    })
    if (
      preview.completionMode !==
      SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI
    ) {
      throw new Error('Device setup requires CLI SpaceLink completion')
    }
    const approval = await session.spacewave.approveSpaceLink({
      ticket: ticketBytes,
      resourceId: new TextEncoder().encode(sharedObjectId),
    })
    if (!approval.completion) {
      throw new Error('SpaceLink approval did not return completion data')
    }
    if (
      approval.completionMode !==
      SpaceLinkCompletionMode.SpaceLinkCompletionMode_CLI
    ) {
      throw new Error('Device setup requires CLI SpaceLink completion')
    }
    await persist(
      {
        ...config,
        ticket,
        completion: bytesToBase64(
          SpaceLinkCallback.toBinary(approval.completion),
        ),
        preview: {
          label: preview.label,
          agentPeerId: bytesToDisplay(preview.agentPeerId),
          targetHint: decodeUTF8(preview.targetHint),
          expiresAt: preview.expiresAt?.toString(),
        },
      },
      nextStep,
    )
    toast.success('Device approved')
  } catch (err) {
    const message = getApprovalErrorMessage(err)
    setError?.(message)
    toast.error(
      message === DEVICE_APPROVAL_CLOUD_REQUIRED
        ? message
        : `Ticket could not be approved. ${message}`,
    )
  } finally {
    setBusy(false)
  }
}

export function getApprovalErrorMessage(error: unknown): string {
  if (isUnimplementedRpcError(error)) {
    return DEVICE_APPROVAL_CLOUD_REQUIRED
  }
  return 'Check the ticket and try again.'
}

function isUnimplementedRpcError(error: unknown): boolean {
  const message =
    error instanceof Error
      ? error.message
      : typeof error === 'string'
        ? error
        : ''
  if (/\bunimplemented\b|\bmethod not implemented\b/i.test(message)) {
    return true
  }
  if (typeof error !== 'object' || error === null || !('rpcError' in error)) {
    return false
  }
  return (
    typeof error.rpcError === 'string' &&
    /\bunimplemented\b|\bmethod not implemented\b/i.test(error.rpcError)
  )
}

async function replaceSpaceIndexIfWizardIsCurrent(
  ws: UseWizardStateResult,
  dashboardKey: string,
) {
  if (
    parseObjectUri(ws.spaceSettings?.indexPath ?? '').objectKey !== ws.objectKey
  ) {
    return
  }
  await applySpaceIndexPath(ws.spaceWorld, dashboardKey, {
    sender: ws.sessionPeerId,
    expectedIndexPath: ws.spaceSettings?.indexPath ?? '',
  })
}

async function createSshCredentialSecrets({
  space,
  label,
  config,
  draft,
  world,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  config: SshHostWizardConfig
  draft: SshCredentialDraft
  world: IWorldState
  readerPublicKeyPem: string
}): Promise<SshHostCredentialRefs> {
  const authMode = config.authMode ?? 'password'
  const credentials: SshHostCredentialRefs = {}
  if (authMode === 'private-key') {
    credentials.privateKeySecretObjectKey = await createSshCredentialSecret({
      space,
      label,
      suffix: 'private key',
      kind: SecretKindSSHPrivateKey,
      contentType: SSHPrivateKeyContentType,
      value: draft.privateKey,
      world,
      readerPublicKeyPem,
    })
    if (draft.passphrase.trim()) {
      credentials.passphraseSecretObjectKey = await createSshCredentialSecret({
        space,
        label,
        suffix: 'passphrase',
        kind: SecretKindSSHPassphrase,
        contentType: SSHTextCredentialContentType,
        value: draft.passphrase,
        world,
        readerPublicKeyPem,
      })
    }
    return credentials
  }

  credentials.passwordSecretObjectKey = await createSshCredentialSecret({
    space,
    label,
    suffix: 'password',
    kind: SecretKindSSHPassword,
    contentType: SSHTextCredentialContentType,
    value: draft.password,
    world,
    readerPublicKeyPem,
  })
  return credentials
}

async function createSshCredentialSecret({
  space,
  label,
  suffix,
  kind,
  contentType,
  value,
  world,
  readerPublicKeyPem,
}: {
  space: Space
  label: string
  suffix: string
  kind: string
  contentType: string
  value: string
  world: IWorldState
  readerPublicKeyPem: string
}): Promise<string> {
  const objectKey = await buildObjectKey(
    world,
    'secret/',
    `${label} SSH ${suffix}`,
  )
  await space.createSecret({
    objectKey,
    displayName: `${label} SSH ${suffix}`,
    kind,
    contentType,
    value: new TextEncoder().encode(value),
    readerPublicKeyPem: new TextEncoder().encode(readerPublicKeyPem),
  })
  return objectKey
}

function buildSshHostKeyPins(
  config: SshHostWizardConfig,
  acceptedByPeerId: string,
  acceptedAt: Date,
): SshHostKeyPin[] {
  const publicKey = normalizeSshHostPublicKey(config.hostKeyPublicKey)
  const fingerprint = config.hostKeyFingerprint?.trim()
  if (!publicKey && !fingerprint) return []
  const publicKeyAlgorithm = getSshPublicKeyAlgorithm(publicKey)
  return [
    {
      algorithm:
        publicKeyAlgorithm || (config.hostKeyAlgorithm ?? 'ssh-ed25519').trim(),
      publicKey,
      sha256Fingerprint: fingerprint,
      acceptedByPeerId,
      acceptedAt,
    },
  ]
}

function getSshCreateError(
  config: SshHostWizardConfig,
  draft: SshCredentialDraft,
): string {
  if (!config.host?.trim()) return 'SSH host is required'
  if (!config.username?.trim()) return 'SSH username is required'
  const port = normalizeSshPort(config.port)
  if (port < 1 || port > 65535) return 'SSH port is invalid'
  const authMode = config.authMode ?? 'password'
  if (authMode === 'private-key' && !draft.privateKey.trim()) {
    return 'SSH private key is required'
  }
  const publicKey = normalizeSshHostPublicKey(config.hostKeyPublicKey)
  const hasManualTrust = !!publicKey || !!config.hostKeyFingerprint?.trim()
  const publicKeyAlgorithm = getSshPublicKeyAlgorithm(publicKey)
  if (
    hasManualTrust &&
    !publicKeyAlgorithm &&
    !(config.hostKeyAlgorithm ?? 'ssh-ed25519').trim()
  ) {
    return 'SSH host key algorithm is required'
  }
  return ''
}

function normalizeSshHostPublicKey(value: string | undefined): string {
  const raw = value?.trim()
  if (!raw) return ''
  const fields = raw.split(/\s+/)
  const keyIndex = fields.findIndex(isSshPublicKeyAlgorithm)
  if (keyIndex >= 0 && fields[keyIndex + 1]) {
    return `${fields[keyIndex]} ${fields[keyIndex + 1]}`
  }
  return raw
}

function getSshPublicKeyAlgorithm(publicKey: string): string {
  const fields = publicKey.trim().split(/\s+/)
  return isSshPublicKeyAlgorithm(fields[0] ?? '') ? fields[0] : ''
}

function isSshPublicKeyAlgorithm(value: string): boolean {
  return (
    value.startsWith('ssh-') ||
    value.startsWith('ecdsa-') ||
    value.startsWith('sk-ssh-') ||
    value.startsWith('sk-ecdsa-')
  )
}

function normalizeSshPort(port: number | undefined): number {
  if (!Number.isFinite(port) || !port) return DEFAULT_SSH_PORT
  return Math.trunc(port)
}

function parsePortInput(value: string): number | undefined {
  const parsed = Number.parseInt(value, 10)
  if (!Number.isFinite(parsed)) return undefined
  return parsed
}

function decodeConfig(data: Uint8Array | undefined): AddDeviceWizardConfig {
  if (!data?.length) return {}
  try {
    return JSON.parse(new TextDecoder().decode(data)) as AddDeviceWizardConfig
  } catch {
    return {}
  }
}

function encodeConfig(config: AddDeviceWizardConfig): Uint8Array {
  return new TextEncoder().encode(JSON.stringify(config))
}

function buildSetupCommand(label: string, sharedObjectId: string): string {
  return ['spacewave', buildDeviceSetupArgs(label, sharedObjectId)].join(' ')
}

function buildInstallSetupCommand(
  label: string,
  sharedObjectId: string,
): string {
  return [
    `tmp=$(mktemp) || exit; trap 'rm -f "$tmp"' 0 HUP INT TERM; curl -fsSL --output "$tmp" https://raw.githubusercontent.com/s4wave/spacewave/master/scripts/spacewave.sh || exit; sh "$tmp"`,
    buildDeviceSetupArgs(label, sharedObjectId),
  ].join(' ')
}

function buildDeviceSetupArgs(label: string, sharedObjectId: string): string {
  const parts = ['device', 'setup', '--label', quoteShellArg(label)]
  if (sharedObjectId) {
    parts.push('--target-hint', quoteShellArg(sharedObjectId))
  }
  return parts.join(' ')
}

function quoteShellArg(value: string): string {
  if (/^[a-zA-Z0-9_./:@%+=,-]+$/.test(value)) return value
  return `'${value.replaceAll("'", "'\"'\"'")}'`
}

function base64ToBytes(value: string): Uint8Array {
  const clean = value.trim()
  const binary = atob(clean)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}

function bytesToBase64(bytes: Uint8Array): string {
  return btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''))
}

function bytesToDisplay(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return bytesToBase64(bytes)
}

function decodeUTF8(bytes: Uint8Array | undefined): string {
  if (!bytes?.length) return ''
  return new TextDecoder().decode(bytes)
}

// deviceWizardFieldStyles separates the public-key field from the preceding control.
const deviceWizardFieldStyles = stylex.create({
  publicKey: { marginTop: 'calc(var(--spacing) * 2)' },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
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
const nativeChildSpacingNextNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
