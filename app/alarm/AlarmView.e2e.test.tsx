import { afterEach, describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { cleanup, render } from 'vitest-browser-react'

import '@s4wave/web/style/app.js'

import { AlarmState, type Alarm } from '@s4wave/sdk/alarm/alarm.pb.js'

import { AlarmChips } from './AlarmStrip.js'
import { AlarmView } from './AlarmView.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 105)`,
    width: `calc(${themeLayout['--spacing']} * 180)`,
  },
  rule2: {
    backgroundColor: themeColors['--color-background-primary'],
    height: `calc(${themeLayout['--spacing']} * 40)`,
    width: `calc(${themeLayout['--spacing']} * 180)`,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 4)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

// at is a time today at hh:mm local.
function at(hh: number, mm: number): bigint {
  const d = new Date()
  d.setHours(hh, mm, 0, 0)
  return BigInt(d.getTime())
}

const nowMs = Number(at(14, 9))

const base: Alarm = {
  title: 'Forge down',
  summary: 'forge:9100 has not answered a scrape.',
  holdMs: 120_000n,
  options: [
    {
      label: 'Restart node_exporter on forge',
      description: 'systemctl restart prometheus-node-exporter',
      recommended: false,
    },
    {
      label: 'Check that forge is up',
      description: 'Open the forge Device and look at its status.',
      recommended: true,
    },
  ],
}

// states are the Alarm viewer states of the sheet, by capture name.
const states: [
  string,
  Alarm,
  { canAcknowledge?: boolean; ackError?: string },
][] = [
  ['clear-never-fired', { ...base, state: AlarmState.CLEAR }, {}],
  [
    'clear-after-episode',
    { ...base, state: AlarmState.CLEAR, clearedMs: at(14, 2) },
    {},
  ],
  ['holding', { ...base, state: AlarmState.HOLDING, sinceMs: at(14, 8) }, {}],
  [
    'firing-writer',
    { ...base, state: AlarmState.FIRING, firedMs: at(14, 5) },
    { canAcknowledge: true },
  ],
  [
    'firing-reader',
    { ...base, state: AlarmState.FIRING, firedMs: at(14, 5) },
    {},
  ],
  [
    'acknowledged',
    {
      ...base,
      state: AlarmState.ACKNOWLEDGED,
      firedMs: at(14, 5),
      acknowledgedBy: 'peer-cjs',
      acknowledgedMs: at(14, 7),
    },
    {},
  ],
  [
    'error',
    {
      ...base,
      state: AlarmState.FIRING,
      firedMs: at(14, 5),
      error: 'function/pure forge-down: budget exceeded',
    },
    {},
  ],
  [
    'ack-failed',
    { ...base, state: AlarmState.FIRING, firedMs: at(14, 5) },
    {
      canAcknowledge: true,
      ackError: 'Could not acknowledge: world op rejected.',
    },
  ],
  [
    'ack-after-clear',
    { ...base, state: AlarmState.CLEAR, clearedMs: at(14, 8) },
    {
      canAcknowledge: true,
      ackError: 'This Alarm stopped firing before the acknowledgement.',
    },
  ],
]

describe('Alarm states', () => {
  afterEach(() => {
    void cleanup()
  })

  it('captures each Alarm viewer state', async () => {
    // Render each state in a pane and capture it.
    await page.viewport(720, 420)
    for (const [name, alarm, opts] of states) {
      const screen = await render(
        <div sx={stylexStyles.rule1}>
          <AlarmView
            alarm={alarm}
            nowMs={nowMs}
            who={(p) => (p === 'peer-cjs' ? 'cjs' : p)}
            canAcknowledge={opts.canAcknowledge ?? false}
            saving={false}
            ackError={opts.ackError ?? null}
            onAcknowledge={() => {}}
          />
        </div>,
      )
      await expect.element(page.getByText('Forge down')).toBeVisible()
      await page.screenshot({ path: `__screenshots__/alarm/${name}.png` })
      await screen.unmount()
    }
  })

  it('captures the Dashboard strip', async () => {
    // Render one chip, then several in sound order, above a panel area.
    await page.viewport(720, 160)
    const firing = { ...base, state: AlarmState.FIRING, firedMs: at(14, 5) }
    const sets: [string, { key: string; alarm: Alarm }[]][] = [
      ['strip-one', [{ key: 'a', alarm: firing }]],
      [
        'strip-several',
        [
          { key: 'a', alarm: firing },
          {
            key: 'b',
            alarm: {
              ...base,
              title: 'Thumper disk 90%',
              state: AlarmState.ACKNOWLEDGED,
              firedMs: at(13, 40),
            },
          },
          {
            key: 'c',
            alarm: {
              ...base,
              title: 'Scrape slow',
              state: AlarmState.HOLDING,
              sinceMs: at(14, 8),
            },
          },
        ],
      ],
    ]
    for (const [name, alarms] of sets) {
      const screen = await render(
        <div sx={stylexStyles.rule2}>
          <AlarmChips alarms={alarms} nowMs={nowMs} onOpen={() => {}} />
          <div sx={stylexStyles.rule3}>Panels</div>
        </div>,
      )
      await expect.element(page.getByText('Forge down')).toBeVisible()
      await page.screenshot({ path: `__screenshots__/alarm/${name}.png` })
      await screen.unmount()
    }
  })
})
