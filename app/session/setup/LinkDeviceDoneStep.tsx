import { useResource } from '@aptre/bldr-sdk/hooks/useResource.js'
import {
  LuArrowRight,
  LuCircleCheck,
  LuLink,
  LuRefreshCw,
} from '@s4wave/web/images/icons.js'

import type { SessionListEntry } from '@s4wave/core/session/session.pb.js'
import type { Session } from '@s4wave/sdk/session/session.js'
import { Spinner } from '@s4wave/web/ui/loading/Spinner.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  radii as themeRadii,
  utilityState as themeUtilityState,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({ rule1: { textAlign: 'center' } })

const stylexStyles2 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule2: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: '100%',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
  },
  rule4: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule5: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule6: {
    display: 'flex',
    width: `calc(${themeLayout['--spacing']} * 12)`,
    height: `calc(${themeLayout['--spacing']} * 12)`,
    alignItems: 'center',
    justifyContent: 'center',
    borderRadius: '3.40282e38px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
  },
  rule7: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: { display: 'flex', gap: `calc(${themeLayout['--spacing']} * 2)` },
  rule9: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-foreground']} 40%, transparent)`,
      },
    },
  },
  rule10: {
    display: 'flex',
    height: `calc(${themeLayout['--spacing']} * 10)`,
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
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
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 6)`,
    height: `calc(${themeLayout['--spacing']} * 6)`,
    color: themeColors['--color-brand'],
  },
})

export interface LinkDeviceDoneStepProps {
  session: Session | null | undefined
  remotePeerId: string | null
  onDone: (entry?: SessionListEntry) => void
  onLinkMore: (entry?: SessionListEntry) => void
}

// LinkDeviceDoneStep reads the durable account attachment. File-copy progress is
// independent; a paired-device record never establishes either completion state.
export function LinkDeviceDoneStep({
  session,
  remotePeerId,
  onDone,
  onLinkMore,
}: LinkDeviceDoneStepProps) {
  const completion = useResource(
    async (signal) =>
      session && remotePeerId
        ? session.confirmPairing(remotePeerId, '', signal)
        : null,
    [session, remotePeerId],
  )
  const result = completion.loading ? null : completion.value
  const error = completion.error?.message

  if (error) {
    return (
      <div sx={stylexStyles3.rule1}>
        <h2 sx={[nativeChildSpacing.s4, stylexStyles2.rule1]}>
          Pairing failed
        </h2>
        <p sx={[nativeChildSpacing.s4, stylexStyles2.rule2]}>{error}</p>
        <button
          type="button"
          onClick={() => onLinkMore()}
          sx={[nativeChildSpacing.s4, stylexStyles2.rule3]}
        >
          <LuRefreshCw sx={stylexStyles.rule1} />
          <span sx={stylexStyles2.rule4}>Try again</span>
        </button>
      </div>
    )
  }

  return (
    <div>
      <div sx={[nativeChildSpacing.s4, stylexStyles2.rule5]}>
        <div sx={stylexStyles2.rule6}>
          {result ? (
            <LuCircleCheck sx={stylexStyles.rule2} />
          ) : (
            <Spinner size="lg" variant="brand" />
          )}
        </div>
        <h2 sx={stylexStyles2.rule1}>
          {result ? 'Account connected' : 'Opening paired account…'}
        </h2>
        {result && (
          <p sx={stylexStyles2.rule7}>Your Spaces are ready to open.</p>
        )}
      </div>
      {result && (
        <div sx={[nativeChildSpacing.s4, stylexStyles2.rule8]}>
          <button
            type="button"
            onClick={() => onLinkMore(result.sessionListEntry)}
            sx={stylexStyles2.rule9}
          >
            <LuLink sx={stylexStyles.rule1} />
            <span sx={stylexStyles2.rule4}>Link more</span>
          </button>
          <button
            type="button"
            onClick={() => onDone(result.sessionListEntry)}
            sx={stylexStyles2.rule10}
          >
            <span sx={stylexStyles2.rule4}>Open account</span>
            <LuArrowRight sx={stylexStyles.rule1} />
          </button>
        </div>
      )}
    </div>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s4: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 4 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
