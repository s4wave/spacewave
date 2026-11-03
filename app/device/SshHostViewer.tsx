import { useCallback } from 'react'
import {
  LuCircleAlert,
  LuCircleCheck,
  LuHardDrive,
  LuServer,
  LuShield,
  LuTerminal,
  LuCircleX,
} from '@s4wave/web/images/icons.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SshHostHandle, SshHostTypeID } from '@s4wave/sdk/sshhost/sshhost.js'
import {
  SshHostProbeState,
  type SshHost,
  type SshHostCredentialRefs,
  type SshHostKeyPin,
} from '@s4wave/sdk/sshhost/sshhost.pb.js'
import { CREATE_TERMINAL_OP_ID } from '@s4wave/sdk/terminal/create-terminal.js'
import { SpaceContainerContext } from '@s4wave/web/contexts/SpaceContainerContext.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'

import { buildCreateSshHostTerminalOpData } from './terminal-action.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles5 = stylex.create({
  rule1: {
    display: 'grid',
    gap: themeLayout['--spacing'],
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'minmax(8rem, .35fr) minmax(0, 1fr)',
    },
  },
})

const stylexStyles4 = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule2: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    display: 'flex',
    minWidth: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule4: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule6: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    display: 'flex',
    flexDirection: { default: 'column', '@media (min-width: 40rem)': 'row' },
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: { default: null, '@media (min-width: 40rem)': 'flex-start' },
    justifyContent: {
      default: null,
      '@media (min-width: 40rem)': 'space-between',
    },
  },
  rule8: { minWidth: 0 },
  rule9: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule11: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(4, minmax(0, 1fr))',
    },
  },
  rule12: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule13: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule14: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule15: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule16: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
  rule17: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule18: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 20%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule19: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
  },
  rule20: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule21: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule22: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule23: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule24: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule25: {
    cursor: 'pointer',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 70%, transparent)`,
    minHeight: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 11)`,
    },
    paddingBlock: {
      default: null,
      '@media (pointer: coarse)': `calc(${themeLayout['--spacing']} * 3.5)`,
    },
  },
  rule26: {
    minWidth: 0,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    wordBreak: 'break-all',
    color: themeColors['--color-foreground'],
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: { color: themeColors['--color-success'] },
  rule3: { color: themeColors['--color-destructive'] },
  rule4: { color: themeColors['--color-warning'] },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
})

export { SshHostTypeID }

export interface CanonicalSshConnectionStatus {
  state: 'connecting' | 'ready' | 'failed' | 'unknown'
  label: 'Connecting' | 'Ready' | 'Failed' | 'Unknown'
  detail: string
  observedAt?: Date
}

export function projectSshConnectionStatus(
  lastStatus: SshHost['lastStatus'] | undefined,
  loading = false,
): CanonicalSshConnectionStatus {
  const error = lastStatus?.error?.trim() ?? ''
  const message = lastStatus?.message?.trim() ?? ''
  const hasTimeout = /timeout|timed out|deadline/i.test(`${error} ${message}`)
  if (loading && !lastStatus) {
    return {
      state: 'connecting',
      label: 'Connecting',
      detail: 'Checking the SSH endpoint and host-key trust.',
    }
  }
  if (error || hasTimeout || lastStatus?.state === SshHostProbeState.FAILED) {
    const detail = hasTimeout
      ? 'The last connection attempt timed out. Check the host and network, then try again.'
      : /auth|credential|permission|denied/i.test(`${error} ${message}`)
        ? 'The last connection attempt was not accepted. Check the username and credential.'
        : 'The last connection attempt failed. Check the connection details and try again.'
    return {
      state: 'failed',
      label: 'Failed',
      detail,
      observedAt: lastStatus?.observedAt,
    }
  }
  if (lastStatus?.state === SshHostProbeState.READY) {
    return {
      state: 'ready',
      label: 'Ready',
      detail: 'Connected and host key verified.',
      observedAt: lastStatus.observedAt,
    }
  }
  return {
    state: 'unknown',
    label: 'Unknown',
    detail: 'No connection check has completed yet.',
    observedAt: lastStatus?.observedAt,
  }
}

export function SshHostViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const { navigateToObjects, spaceWorld } = SpaceContainerContext.useContext()
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SshHostHandle,
    SshHostTypeID,
  )
  const streamFactory = useCallback(
    (h: SshHostHandle, signal: AbortSignal) => h.watchSshHostState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: SshHost | undefined = stateResource.value ?? undefined
  const status = projectSshConnectionStatus(
    state?.lastStatus,
    stateResource.loading,
  )
  const pins = state?.hostKeyPins ?? []

  const handleOpenTerminal = async () => {
    if (!state) return
    try {
      const terminalOp = await buildCreateSshHostTerminalOpData({
        host: state,
        hostObjectKey: objectKey,
        world: spaceWorld,
      })
      if (!terminalOp) return
      await spaceWorld.applyWorldOp(
        CREATE_TERMINAL_OP_ID,
        terminalOp.opData,
        '',
      )
      navigateToObjects([terminalOp.objectKey])
    } catch {
      toast.error('Terminal could not be opened. Try again.')
    }
  }

  return (
    <div sx={stylexStyles3.rule1}>
      <div sx={stylexStyles3.rule2}>
        <span sx={stylexStyles3.rule3}>
          <LuServer sx={stylexStyles.rule1} />
          {state?.label || 'SSH Host'}
        </span>
        <div sx={stylexStyles3.rule4}>
          {state && (
            <DashboardButton
              icon={<LuTerminal sx={stylexStyles.rule2} />}
              onClick={() => void handleOpenTerminal()}
            >
              Open Terminal
            </DashboardButton>
          )}
          <DashboardButton
            icon={<LuHardDrive sx={stylexStyles.rule2} />}
            disabled
          >
            Install Agent
          </DashboardButton>
        </div>
      </div>
      <div sx={stylexStyles3.rule5}>
        {stateResource.loading && !state && (
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading SSH Host',
              detail: 'Reading SSH Host state.',
            }}
          />
        )}
        {state && (
          <div sx={stylexStyles3.rule6}>
            <InfoCard>
              <div sx={stylexStyles3.rule7}>
                <div sx={stylexStyles3.rule8}>
                  <div sx={stylexStyles3.rule9}>{formatEndpoint(state)}</div>
                  <div sx={stylexStyles3.rule10}>SSH-only connection</div>
                </div>
                <CanonicalStatus status={status} />
              </div>
              <div sx={stylexStyles3.rule11}>
                <HostMeta label="Mode" value="SSH-only" />
                <HostMeta
                  label="User"
                  value={state.endpoint?.username || 'Unknown'}
                />
                <HostMeta
                  label="Credential"
                  value={credentialSummary(state.credentials)}
                />
                <HostMeta
                  label="Trust"
                  value={
                    pins.length ? 'Pinned host key' : 'Ask on first connection'
                  }
                />
              </div>
              <p sx={stylexStyles3.rule12}>
                Open Terminal requires a desktop/native SSH connector.
              </p>
            </InfoCard>

            <InfoCard
              icon={<LuCircleAlert sx={stylexStyles.rule3} />}
              title="Connection status"
            >
              <div sx={stylexStyles3.rule13}>
                <CanonicalStatusIcon state={status.state} />
                <div>
                  <div sx={stylexStyles3.rule14}>{status.label}</div>
                  <div sx={stylexStyles3.rule15}>{status.detail}</div>
                  {status.observedAt && (
                    <div sx={stylexStyles3.rule16}>
                      Last checked {formatObservedAt(status.observedAt)}
                    </div>
                  )}
                </div>
              </div>
            </InfoCard>

            <InfoCard
              icon={<LuShield sx={stylexStyles.rule3} />}
              title="Credentials"
            >
              <CredentialRows refs={state.credentials} />
            </InfoCard>

            <InfoCard
              icon={<LuShield sx={stylexStyles.rule3} />}
              title="Host-key trust"
            >
              {pins.length === 0 ? (
                <div sx={stylexStyles3.rule17}>Ask on first connection</div>
              ) : (
                <div>
                  {pins.map((pin) => (
                    <TrustPinRow
                      sx={nativeDividers.t8}
                      key={`${pin.algorithm}:${pin.sha256Fingerprint || pin.publicKey}`}
                      pin={pin}
                    />
                  ))}
                </div>
              )}
            </InfoCard>

            <div sx={stylexStyles3.rule18}>
              <div sx={stylexStyles3.rule19}>
                Agent installation is not available from this SSH Host until
                secure bootstrap is configured.
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function HostMeta({ label, value }: { label: string; value: string }) {
  return (
    <div sx={stylexStyles3.rule8}>
      <div sx={stylexStyles3.rule20}>{label}</div>
      <div sx={stylexStyles3.rule21}>{value}</div>
    </div>
  )
}

function CanonicalStatus({ status }: { status: CanonicalSshConnectionStatus }) {
  return (
    <div sx={stylexStyles3.rule22}>
      <CanonicalStatusIcon state={status.state} />
      <span sx={stylexStyles3.rule23}>{status.label}</span>
    </div>
  )
}

function CanonicalStatusIcon({
  state,
}: {
  state: CanonicalSshConnectionStatus['state']
}) {
  const Icon =
    state === 'ready'
      ? LuCircleCheck
      : state === 'failed'
        ? LuCircleX
        : LuCircleAlert
  return (
    <Icon
      sx={[
        stylexStyles2.rule1,
        state === 'ready'
          ? stylexStyles2.rule2
          : state === 'failed'
            ? stylexStyles2.rule3
            : stylexStyles2.rule4,
      ]}
    />
  )
}

function CredentialRows({ refs }: { refs?: SshHostCredentialRefs }) {
  const summary = credentialSummary(refs)
  return (
    <div sx={false}>
      <div sx={[nativeChildSpacing.s2, stylexStyles3.rule14]}>{summary}</div>
      <details sx={[nativeChildSpacing.s2, stylexStyles3.rule24]}>
        <summary sx={stylexStyles3.rule25}>
          Technical credential details
        </summary>
        <div sx={stylexStyles4.rule1}>
          <div sx={nativeChildSpacing.s1}>
            Private key: {refs?.privateKeySecretObjectKey || 'not linked'}
          </div>
          <div sx={nativeChildSpacing.s1}>
            Password: {refs?.passwordSecretObjectKey || 'not linked'}
          </div>
          <div sx={nativeChildSpacing.s1}>
            Passphrase: {refs?.passphraseSecretObjectKey || 'not linked'}
          </div>
        </div>
      </details>
    </div>
  )
}

function credentialSummary(refs?: SshHostCredentialRefs): string {
  if (refs?.privateKeySecretObjectKey) return 'Private key linked'
  if (refs?.passwordSecretObjectKey) return 'Password linked'
  return 'No credential linked'
}

function TrustPinRow({
  sx,
  pin,
}: { pin: SshHostKeyPin } & { sx?: stylex.StyleXStyles; className?: never }) {
  return (
    <div sx={[stylexStyles5.rule1, [trustRowStyles.insets, sx]]}>
      <div sx={stylexStyles3.rule17}>{pin.algorithm || 'unknown'}</div>
      <div sx={stylexStyles3.rule26}>
        {pin.sha256Fingerprint || pin.publicKey || 'unfingerprinted'}
      </div>
    </div>
  )
}

function formatEndpoint(state: SshHost): string {
  const endpoint = state.endpoint
  if (!endpoint?.host) return 'Unknown endpoint'
  return `${endpoint.host}:${endpoint.port || 22}`
}

function formatObservedAt(timestamp: Date): string {
  return timestamp.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
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
})

// nativeDividers applies the list border only to its nonfinal physical rows.
const nativeDividers = stylex.create({
  t8: {
    '--tw-divide-y-reverse': { default: null, ':not(:last-child)': 0 },
    borderBottomStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopStyle: {
      default: null,
      ':not(:last-child)': themeUtilityState['--tw-border-style'],
    },
    borderTopWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-y-reverse))',
    },
    borderBottomWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-y-reverse)))',
    },
    borderColor: {
      default: null,
      ':not(:last-child)':
        'color-mix(in oklab, var(--color-foreground) 8%, transparent)',
    },
  },
})

// trustRowStyles preserves first and final row insets at the actual trust row.
const trustRowStyles = stylex.create({
  insets: {
    paddingTop: { default: 'calc(var(--spacing) * 2)', ':first-child': 0 },
    paddingBottom: { default: 'calc(var(--spacing) * 2)', ':last-child': 0 },
  },
})
