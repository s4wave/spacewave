import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { LuSiren } from '@s4wave/web/images/icons.js'

import type { Alarm, AlarmOption } from '@s4wave/sdk/alarm/alarm.pb.js'

import { Button } from '@s4wave/web/ui/button.js'

import { alarmStatus, holdText, type AlarmTone } from './alarm-view.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
} from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    borderRadius: '.25rem',
    paddingInline: `calc(${themeLayout['--spacing']} * 1.5)`,
    paddingBlock: '1px',
    fontSize: themeLayout['--text-metadata'],
    '--tw-leading': themeLayout['--text-metadata--line-height'],
    lineHeight: themeLayout['--text-metadata--line-height'],
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
  },
  rule2: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
    color: themeColors['--color-destructive'],
  },
  rule3: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-warning']} 15%, transparent)`,
    color: themeColors['--color-warning'],
  },
  rule4: {
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    minHeight: 0,
    flex: 1,
    overflow: 'auto',
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: { maxWidth: 'var(--container-xl)' },
})

const stylexStyles2 = stylex.create({
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
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule3: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule4: { color: themeColors['--color-destructive'] },
  rule5: { color: themeColors['--color-foreground'] },
  rule6: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule7: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-warning'],
  },
  rule10: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-destructive'],
  },
  rule12: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground-alt'],
  },
  rule13: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
  },
  rule14: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 20%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 5%, transparent)`,
  },
  rule15: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
  },
  rule16: {
    display: 'flex',
    alignItems: 'center',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground'],
  },
  rule17: {
    fontSize: themeLayout['--text-metadata'],
    lineHeight: `var(--tw-leading, ${themeLayout['--text-metadata--line-height']})`,
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
  rule2: { color: themeColors['--color-destructive'] },
  rule3: { color: themeColors['--color-foreground-alt'] },
})

export interface AlarmViewProps {
  alarm: Alarm
  nowMs: number
  // who names the member of a peer ID.
  who: (peerId: string) => string
  // canAcknowledge is set for a writer.
  canAcknowledge: boolean
  saving: boolean
  // ackError is why the last acknowledgement failed.
  ackError: string | null
  onAcknowledge: () => void
}

// AlarmView shows an Alarm: its state, the condition, the actions that would
// change it, and a writer's Acknowledge.
export function AlarmView({
  alarm,
  nowMs,
  who,
  canAcknowledge,
  saving,
  ackError,
  onAcknowledge,
}: AlarmViewProps) {
  const status = alarmStatus(alarm, nowMs, who)
  return (
    <div sx={stylexStyles2.rule1}>
      <header sx={stylexStyles2.rule2}>
        <LuSiren
          sx={[
            stylexStyles.rule1,
            status.tone === 'alert' ? stylexStyles.rule2 : stylexStyles.rule3,
          ]}
          aria-hidden
        />
        <span
          sx={[
            stylexStyles2.rule3,
            status.tone === 'alert' ? stylexStyles2.rule4 : stylexStyles2.rule5,
          ]}
        >
          {alarm.title || 'Alarm'}
        </span>
        <AlarmBadge word={status.word} tone={status.tone} />
      </header>

      <div sx={stylexStyles3.rule1}>
        <div sx={[nativeChildSpacingNext.s4, false]}>
          <p sx={[nativeChildSpacing.s1, stylexStyles2.rule6]}>{status.line}</p>
          {alarm.summary ? (
            <p sx={[nativeChildSpacing.s1, stylexStyles2.rule7]}>
              {alarm.summary}
            </p>
          ) : null}
          {!status.sounding ? (
            <p sx={[nativeChildSpacing.s1, stylexStyles2.rule8]}>
              {holdText(alarm)}
            </p>
          ) : null}
        </div>

        {alarm.error ? (
          <p sx={[nativeChildSpacingNext.s4, stylexStyles2.rule9]}>
            The watch could not check this Alarm:{' '}
            {alarm.error.replace(/\.$/, '')}.
          </p>
        ) : null}

        {canAcknowledge || ackError ? (
          <AckRow
            sx={nativeChildSpacingNext.s4}
            firing={status.word === 'Firing'}
            canAcknowledge={canAcknowledge}
            saving={saving}
            ackError={ackError}
            onAcknowledge={onAcknowledge}
          />
        ) : null}

        {status.sounding ? (
          <AlarmOptions
            sx={nativeChildSpacingNext.s4}
            options={alarm.options ?? []}
          />
        ) : null}
      </div>
    </div>
  )
}

// AckRow is a writer's Acknowledge while the Alarm fires, with why the last
// acknowledgement failed.
function AckRow({
  sx,
  firing,
  canAcknowledge,
  saving,
  ackError,
  onAcknowledge,
}: (Pick<
  AlarmViewProps,
  'canAcknowledge' | 'saving' | 'ackError' | 'onAcknowledge'
> & { firing: boolean }) & { sx?: stylex.StyleXStyles; className?: never }) {
  if (!firing) {
    return ackError ? <p sx={[stylexStyles2.rule8, sx]}>{ackError}</p> : null
  }
  if (!canAcknowledge) return null
  return (
    <div sx={[stylexStyles2.rule10, sx]}>
      <Button size="sm" disabled={saving} onClick={onAcknowledge}>
        {ackError ? 'Retry' : 'Acknowledge'}
      </Button>
      {ackError ? <span sx={stylexStyles2.rule11}>{ackError}</span> : null}
    </div>
  )
}

// AlarmOptions lists the actions that would change the condition, the
// recommended one first and marked.
function AlarmOptions({
  sx,
  options,
}: { options: readonly AlarmOption[] } & {
  sx?: stylex.StyleXStyles
  className?: never
}) {
  if (options.length === 0) return null
  const sorted = [...options].sort(
    (a, b) => Number(b.recommended ?? false) - Number(a.recommended ?? false),
  )
  return (
    <section sx={[stylexStyles3.rule2, sx]}>
      <h3 sx={[nativeChildSpacingNext.s1_5, stylexStyles2.rule12]}>
        What would change this
      </h3>
      <ul sx={[nativeChildSpacingNext.s1_5, false]}>
        {sorted.map((o) => (
          <li
            key={o.label}
            sx={[
              nativeChildSpacing.s1_5,
              [
                stylexStyles2.rule13,
                o.recommended ? stylexStyles2.rule14 : stylexStyles2.rule15,
              ],
            ]}
          >
            <div sx={stylexStyles2.rule16}>
              {o.label}
              {o.recommended ? (
                <span sx={stylexStyles2.rule17}>Recommended</span>
              ) : null}
            </div>
            {o.description ? (
              <div sx={stylexStyles2.rule8}>{o.description}</div>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  )
}

// AlarmBadge labels an Alarm's state with its word and a status color.
export function AlarmBadge({
  word,
  tone,
  sx,
}: Omit<
  {
    word: string
    tone: AlarmTone
    className?: string
  },
  'className'
> & { sx?: ComponentStyles; className?: never }) {
  return (
    <span
      // Keep the metadata font role separate from conditional tone utilities.
      sx={[
        stylexStyles4.rule1,
        tone === 'alert' && stylexStyles4.rule2,
        tone === 'holding' && stylexStyles4.rule3,
        tone === 'quiet' && stylexStyles4.rule4,
        sx,
      ]}
    >
      {word}
    </span>
  )
}

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
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
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
  s1_5: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1.5 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
