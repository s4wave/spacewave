import { useState } from 'react'
import {
  LuKey,
  LuPlus,
  LuTrash2,
  LuDownload,
} from '@s4wave/web/images/icons.js'

import { downloadPemFile } from '@s4wave/web/download.js'
import { useStreamingResource } from '@aptre/bldr-sdk/hooks/useStreamingResource.js'
import { useResourceValue } from '@aptre/bldr-sdk/hooks/useResource.js'
import { InfoCard } from '@s4wave/web/ui/InfoCard.js'
import { SessionContext } from '@s4wave/web/contexts/contexts.js'

import { CredentialProofInput } from '@s4wave/web/ui/credential/CredentialProofInput.js'
import { useCredentialProof } from '@s4wave/web/ui/credential/useCredentialProof.js'
import { truncatePeerId } from '@s4wave/web/ui/credential/auth-utils.js'
import { useMountAccount } from '@s4wave/web/hooks/useMountAccount.js'
import { useSessionInfo } from '@s4wave/web/hooks/useSessionInfo.js'
import type { Account } from '@s4wave/sdk/account/account.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import type { EntityKeypair } from '@s4wave/core/session/session.pb.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  motion as themeMotion,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

import { radii as themeRadii } from '../../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 3)`,
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 3)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule2: {
    paddingBlock: themeLayout['--spacing'],
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule3: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule4: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 10%, transparent)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    paddingTop: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule5: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    gap: themeLayout['--spacing'],
    alignItems: 'center',
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 80%, transparent)`,
      },
    },
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule6: {
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule7: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    userSelect: 'none',
  },
  rule8: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule9: {
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule10: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderRadius: '.25rem',
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 90%, transparent)`,
      },
    },
    color: themeColors['--color-white'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule11: { cursor: 'not-allowed', opacity: 0.5 },
  rule12: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      },
    },
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule13: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule14: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule15: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
  },
  rule16: {
    flex: 1,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
    minWidth: 0,
  },
  rule17: { flex: 1, minWidth: 0 },
  rule18: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    overflow: 'hidden',
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  rule19: {
    borderRadius: '.25rem',
    gap: themeLayout['--spacing'],
    paddingBlock: `calc(${themeLayout['--spacing']} * .5)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    alignItems: 'center',
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-destructive'],
      },
    },
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3)`,
    width: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule3: {
    color: themeColors['--color-foreground-alt'],
    flexShrink: 0,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface EntityKeypairsSectionProps {
  sx?: stylex.StyleXStyles
  className?: never
  // embedded hides the section heading and outer wrapper when rendered inside
  // a parent CollapsibleSection.
  embedded?: boolean
}

// EntityKeypairsSection displays and manages entity keypairs for local sessions.
export function EntityKeypairsSection({
  sx,
  embedded,
}: EntityKeypairsSectionProps) {
  const sessionResource = SessionContext.useContext()
  const session = useResourceValue(sessionResource)

  const { providerId, accountId } = useSessionInfo(session)
  const accountResource = useMountAccount(providerId, accountId)
  const keypairsResource = useStreamingResource(
    accountResource,
    (account, signal) => account.watchEntityKeypairs({}, signal),
    [],
  )
  const keypairs = (keypairsResource.value?.keypairs ?? [])
    .map((state) => state.keypair)
    .filter((keypair): keypair is EntityKeypair => keypair != null)
  const loading = keypairsResource.loading

  const [showAdd, setShowAdd] = useState(false)
  const [removing, setRemoving] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const handleRemove = async (peerId: string) => {
    if (!session || !peerId) return
    setRemoving(peerId)
    setError(null)
    try {
      await session.localProvider.removeEntityKeypair({ peerId })
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to remove keypair')
    }
    setRemoving(null)
  }

  const content = (
    <>
      {loading && (
        <p sx={[stylexStyles2.rule1, embedded && sx]}>Loading keypairs…</p>
      )}
      {!loading && keypairs.length === 0 && !showAdd && (
        <div sx={[stylexStyles2.rule2, embedded && sx]}>
          <p sx={stylexStyles2.rule1}>No entity keypairs yet.</p>
          <button
            type="button"
            onClick={() => setShowAdd(true)}
            sx={stylexStyles2.rule3}
          >
            Add keypair
          </button>
        </div>
      )}
      {!loading && keypairs.length > 0 && (
        <div sx={embedded && sx}>
          {keypairs.map((kp) => (
            <KeypairRow
              sx={nativeChildSpacingNext.s2}
              key={kp.peerId}
              keypair={kp}
              removing={removing === (kp.peerId ?? '')}
              onRemove={handleRemove}
              canRemove={keypairs.length > 1}
            />
          ))}
          {!showAdd && (
            <div sx={[nativeChildSpacingNext.s2, stylexStyles2.rule4]}>
              <button
                type="button"
                onClick={() => setShowAdd(true)}
                sx={stylexStyles2.rule5}
              >
                <LuPlus sx={stylexStyles.rule1} />
                Add keypair
              </button>
            </div>
          )}
        </div>
      )}

      {showAdd && (
        <AddKeypairPanel
          sx={embedded && sx}
          session={session}
          account={accountResource.value}
          error={error}
          onError={setError}
          onClose={() => setShowAdd(false)}
        />
      )}
    </>
  )

  if (embedded) return content

  return (
    <section sx={sx}>
      <div sx={stylexStyles2.rule6}>
        <h2 sx={stylexStyles2.rule7}>
          <LuKey sx={stylexStyles.rule2} />
          Entity Keypairs
        </h2>
      </div>
      <InfoCard>{content}</InfoCard>
    </section>
  )
}

// AddKeypairPanel adds a password key or a downloaded backup key. It owns the
// credential draft, and reports failures through onError.
function AddKeypairPanel({
  sx,
  session,
  account,
  error,
  onError,
  onClose,
}: {
  session: Session | null | undefined
  account: Account | null | undefined
  error: string | null
  onError: (error: string | null) => void
  onClose: () => void
} & { sx?: stylex.StyleXStyles; className?: never }) {
  const [adding, setAdding] = useState(false)
  const [exporting, setExporting] = useState(false)
  const cred = useCredentialProof()
  const busy = adding || exporting

  const handleAddPassword = async () => {
    if (!session || !cred.credential) return
    setAdding(true)
    onError(null)
    try {
      await session.localProvider.addEntityKeypair({
        credential: cred.credential,
      })
      cred.reset()
      onClose()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : 'Failed to add keypair')
    }
    setAdding(false)
  }

  const handleAddBackup = async () => {
    if (!account) return
    setExporting(true)
    onError(null)
    try {
      const resp = await account.generateBackupKey({})
      if (resp.pemData) {
        const filename = `backup-key-${resp.peerId?.slice(0, 8) ?? 'key'}.pem`
        downloadPemFile(resp.pemData, filename)
      }
      cred.reset()
      onClose()
    } catch (e: unknown) {
      onError(e instanceof Error ? e.message : 'Failed to add backup key')
    }
    setExporting(false)
  }

  return (
    <div sx={[stylexStyles4.rule1, sx]}>
      <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule1]}>
        <div sx={nativeChildSpacing.s3}>
          <p sx={stylexStyles2.rule8}>Password key</p>
          <p sx={stylexStyles2.rule9}>
            Adds a key derived from this password. Use the password to recover
            this local session if it becomes locked.
          </p>
        </div>
        <CredentialProofInput
          sx={nativeChildSpacing.s3}
          password={cred.password}
          onPasswordChange={cred.setPassword}
          showPem={false}
          passwordLabel="Password"
          passwordPlaceholder="Enter password for entity key"
          disabled={busy}
          focusOnMount
        />
        <button
          type="button"
          onClick={() => void handleAddPassword()}
          disabled={busy || !cred.hasCredential}
          sx={[
            nativeChildSpacing.s3,
            [
              stylexStyles2.rule10,
              !!(busy || !cred.hasCredential) && stylexStyles2.rule11,
            ],
          ]}
        >
          {adding ? 'Adding…' : 'Add password key'}
        </button>
      </div>
      <div sx={[nativeChildSpacingNext.s3, stylexStyles3.rule1]}>
        <div sx={nativeChildSpacing.s3}>
          <p sx={stylexStyles2.rule8}>Backup key</p>
          <p sx={stylexStyles2.rule9}>
            Creates and adds a separate backup key, then downloads its PEM file.
            Use the file to recover this local session if the password is
            unavailable. Store it somewhere safe.
          </p>
        </div>
        <button
          type="button"
          onClick={() => void handleAddBackup()}
          disabled={busy}
          sx={[
            nativeChildSpacing.s3,
            [stylexStyles2.rule12, !!busy && stylexStyles2.rule11],
          ]}
        >
          <LuDownload sx={stylexStyles.rule1} />
          {exporting ? 'Adding…' : 'Add backup key'}
        </button>
      </div>
      {error && (
        <p sx={[nativeChildSpacingNext.s3, stylexStyles2.rule13]}>{error}</p>
      )}
      <button
        type="button"
        onClick={() => {
          onClose()
          onError(null)
          cred.reset()
        }}
        disabled={busy}
        sx={[nativeChildSpacingNext.s3, stylexStyles2.rule14]}
      >
        Cancel
      </button>
    </div>
  )
}

interface KeypairRowProps {
  keypair: EntityKeypair
  removing: boolean
  onRemove: (peerId: string) => Promise<void>
  canRemove: boolean
}

function KeypairRow({
  sx,
  keypair,
  removing,
  onRemove,
  canRemove,
}: KeypairRowProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const peerId = keypair.peerId ?? ''
  const method = keypair.authMethod === 'pem' ? 'Backup PEM' : 'Password'

  return (
    <div sx={[stylexStyles2.rule15, sx]}>
      <div sx={stylexStyles2.rule16}>
        <LuKey sx={stylexStyles.rule3} />
        <div sx={stylexStyles2.rule17}>
          <p sx={stylexStyles2.rule18}>{truncatePeerId(peerId)}</p>
          <p sx={stylexStyles2.rule1}>{method}</p>
        </div>
      </div>
      {canRemove && (
        <button
          type="button"
          onClick={() => void onRemove(peerId)}
          disabled={removing}
          sx={[stylexStyles2.rule19, !!removing && stylexStyles2.rule11]}
          title="Remove keypair"
        >
          <LuTrash2 sx={stylexStyles.rule1} />
        </button>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
