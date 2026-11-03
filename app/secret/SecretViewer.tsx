import { useCallback } from 'react'
import {
  LuKeyRound,
  LuLockKeyhole,
  LuShieldCheck,
} from '@s4wave/web/images/icons.js'

import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { SecretHandle, SecretTypeID } from '@s4wave/sdk/secret/secret.js'
import type { SecretState } from '@s4wave/sdk/secret/secret.pb.js'
import { SharedObjectHealthStatus } from '@s4wave/core/sobject/sobject.pb.js'
import { useAccessTypedHandle } from '@s4wave/web/hooks/useAccessTypedHandle.js'
import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import { getObjectKey } from '@s4wave/web/object/object.js'
import { useObjectName } from '@s4wave/web/object/ObjectViewerContext.js'

import { CopyableField } from '@s4wave/web/ui/CopyableField.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    backgroundColor: themeColors['--color-background-dark'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    letterSpacing: '.2em',
    color: themeColors['--color-foreground'],
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule3: {
    display: 'grid',
    columnGap: `calc(${themeLayout['--spacing']} * 6)`,
    rowGap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(2, minmax(0, 1fr))',
    },
  },
  rule4: {
    display: 'grid',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    gridTemplateColumns: {
      default: null,
      '@media (min-width: 40rem)': 'repeat(3, minmax(0, 1fr))',
    },
  },
  rule5: {
    marginTop: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule6: {
    marginInline: 'auto',
    display: 'flex',
    width: '100%',
    maxWidth: 'var(--container-3xl)',
    flexDirection: 'column',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule7: {
    display: 'flex',
    height: '100%',
    width: '100%',
    flexDirection: 'column',
    backgroundColor: themeColors['--color-background-primary'],
  },
  rule8: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 9)`,
    flexShrink: 0,
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule9: {
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
    color: themeColors['--color-foreground'],
  },
  rule10: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule11: { minWidth: 0 },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule13: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    overflowWrap: 'break-word',
    color: themeColors['--color-foreground'],
  },
  rule14: {
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
  },
  rule15: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-brand'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
})

export { SecretTypeID }

type SecretGrant = SecretState['grantStatus']

/** grantLabel describes a boolean grant flag, or Unknown before the grant arrives. */
function grantLabel(
  grant: SecretGrant,
  flag: boolean | undefined,
  yes: string,
  no: string,
): string {
  if (!grant) return 'Unknown'
  return flag ? yes : no
}

/** ProtectedValueCard explains why the secret value is never shown. */
function ProtectedValueCard() {
  return (
    <InfoCard
      icon={<LuLockKeyhole sx={stylexStyles.rule1} />}
      title="Protected value"
    >
      <div sx={stylexStyles2.rule1}>••••••••••••</div>
      <p sx={stylexStyles2.rule2}>
        Secret values stay hidden in the object viewer. Use the feature that
        consumes this credential to verify or replace it.
      </p>
    </InfoCard>
  )
}

/** SecretMetadataCard lists the redacted secret fields. */
function SecretMetadataCard({
  secret,
  objectKey,
}: {
  secret: SecretState['secret']
  objectKey: string
}) {
  return (
    <InfoCard title="Metadata">
      <div sx={stylexStyles2.rule3}>
        <Metadata
          label="Name"
          value={secret?.displayName || 'Unnamed secret'}
        />
        <Metadata label="Kind" value={secret?.kind || 'Unspecified'} mono />
        <Metadata label="Object key" value={objectKey || 'Unknown'} mono />
        {secret?.nestedSharedObjectId ? (
          <CopyableField
            label="Nested SharedObject ID"
            value={secret.nestedSharedObjectId}
          />
        ) : (
          <Metadata label="Nested SharedObject ID" value="Unavailable" />
        )}
        <Metadata label="Created" value={formatTimestamp(secret?.createdAt)} />
        <Metadata label="Updated" value={formatTimestamp(secret?.updatedAt)} />
      </div>
    </InfoCard>
  )
}

/** SecretAccessCard reports nested SharedObject access once its health is ready. */
function SecretAccessCard({
  grant,
  healthStatus,
}: {
  grant: SecretGrant
  healthStatus: SharedObjectHealthStatus
}) {
  return (
    <InfoCard
      icon={<LuShieldCheck sx={stylexStyles.rule2} />}
      title="Access status"
    >
      {healthStatus === SharedObjectHealthStatus.READY ? (
        <div sx={stylexStyles2.rule4}>
          <Metadata
            label="Nested SharedObject readability"
            value={grantLabel(
              grant,
              grant?.readable,
              'Readable',
              'Not readable',
            )}
          />
          <Metadata
            label="Participant"
            value={grantLabel(
              grant,
              grant?.participant,
              'Granted',
              'Not granted',
            )}
          />
          <Metadata
            label="Active grants"
            value={grant ? String(grant.grantCount ?? 0) : 'Unknown'}
          />
        </div>
      ) : (
        <Metadata
          label="Nested SharedObject status"
          value={formatHealthStatus(healthStatus)}
        />
      )}
      <p sx={stylexStyles2.rule5}>
        This status describes nested SharedObject access. It does not reveal or
        copy the protected value.
      </p>
    </InfoCard>
  )
}

/** SecretDetails shows the protected value, metadata, and access cards. */
function SecretDetails({
  state,
  objectKey,
}: {
  state: SecretState
  objectKey: string
}) {
  const healthStatus = state.health?.status ?? SharedObjectHealthStatus.UNKNOWN

  return (
    <div sx={stylexStyles2.rule6}>
      <ProtectedValueCard />
      <SecretMetadataCard secret={state.secret} objectKey={objectKey} />
      <SecretAccessCard grant={state.grantStatus} healthStatus={healthStatus} />
    </div>
  )
}

/** SecretBody renders the state stream as an error, loading card, or the details. */
function SecretBody({
  error,
  loading,
  state,
  retry,
  objectKey,
}: {
  error: Error | null | undefined
  loading: boolean
  state: SecretState | undefined
  retry: () => void
  objectKey: string
}) {
  if (error) {
    return (
      <LoadingCard
        view={{
          state: 'error',
          title: 'Secret metadata unavailable',
          detail: 'The redacted Secret state stream stopped unexpectedly.',
          onRetry: retry,
        }}
      />
    )
  }
  if (!state) {
    return loading ? (
      <LoadingCard
        view={{
          state: 'active',
          title: 'Loading secret metadata',
          detail: 'Reading the authorized, redacted Secret state.',
        }}
      />
    ) : (
      <LoadingCard
        view={{
          state: 'error',
          title: 'Secret metadata unavailable',
          detail: 'The Secret resource returned no state.',
          onRetry: retry,
        }}
      />
    )
  }
  return <SecretDetails state={state} objectKey={objectKey} />
}

// SecretViewer displays only the browser-safe metadata stream. Payload reads
// require a separate peer-authenticated challenge and are deliberately absent.
export function SecretViewer({
  objectInfo,
  worldState,
}: ObjectViewerComponentProps) {
  const objectKey = getObjectKey(objectInfo)
  const handle = useAccessTypedHandle(
    worldState,
    objectKey,
    SecretHandle,
    SecretTypeID,
  )
  const streamFactory = useCallback(
    (secret: SecretHandle, signal: AbortSignal) => secret.watchState(signal),
    [],
  )
  const stateResource = useStreamingResource(handle, streamFactory, [])
  const state: SecretState | undefined = stateResource.value ?? undefined
  useObjectName(state?.secret?.displayName)
  const title = stateResource.error ? 'Secret' : state?.secret?.displayName

  return (
    <div sx={stylexStyles2.rule7}>
      <div sx={stylexStyles2.rule8}>
        <LuKeyRound sx={stylexStyles.rule2} aria-hidden="true" />
        <span sx={stylexStyles2.rule9}>{title || 'Secret'}</span>
      </div>
      <div sx={stylexStyles2.rule10}>
        <SecretBody
          error={stateResource.error}
          loading={stateResource.loading}
          state={state}
          retry={stateResource.retry}
          objectKey={objectKey}
        />
      </div>
    </div>
  )
}

function Metadata({
  label,
  value,
  mono = false,
}: {
  label: string
  value: string
  mono?: boolean
}) {
  return (
    <div sx={stylexStyles2.rule11}>
      <p sx={stylexStyles2.rule12}>{label}</p>
      <p
        sx={[
          stylexStyles2.rule13,
          mono ? stylexStyles2.rule14 : stylexStyles2.rule15,
        ]}
      >
        {value}
      </p>
    </div>
  )
}

function formatHealthStatus(status: SharedObjectHealthStatus): string {
  switch (status) {
    case SharedObjectHealthStatus.LOADING:
      return 'Loading'
    case SharedObjectHealthStatus.DEGRADED:
      return 'Degraded'
    case SharedObjectHealthStatus.CLOSED:
      return 'Unavailable'
    default:
      return 'Unknown'
  }
}

function formatTimestamp(value: Date | undefined): string {
  if (!value) return 'Unavailable'
  return value.toLocaleString()
}
