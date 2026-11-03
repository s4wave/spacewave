import { utilityState as dividerState } from '@s4wave/web/style/tokens.stylex.js'
import { useId, useState } from 'react'
import {
  SpacewaveApp,
  type SpacewaveAppProps,
} from '@s4wave/app/SpacewaveApp.js'
import { getDebugContext } from '@s4wave/sdk/debug/context.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { flex: 1, display: 'flex', minHeight: 0 },
})

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    flexDirection: 'column',
    height: '100%',
    minHeight: 0,
  },
  rule2: {
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
  },
  rule3: {
    borderRadius: '.25rem',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingBlock: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule4: {
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    display: 'flex',
  },
})

export interface AppScenario {
  name: string
  title: string
  setup: NonNullable<SpacewaveAppProps['setup']>
}

const ephemeralStorage = { ephemeral: true } as const

// ScenarioPage keeps the outer route stable while each app owns its World.
export function ScenarioPage({ scenario }: { scenario: AppScenario }) {
  const [compare, setCompare] = useState(false)
  const [resetError, setResetError] = useState('')
  const instanceId = useId()
  const appId = `scenario:${scenario.name}:${instanceId}`
  const reset = async () => {
    try {
      setResetError('')
      await getDebugContext<{ reset(): Promise<void> }>(appId).reset()
    } catch (error) {
      setResetError(error instanceof Error ? error.message : String(error))
    }
  }
  return (
    <div sx={stylexStyles.rule1} data-testid="app-scenario" data-app-id={appId}>
      <div sx={stylexStyles.rule2}>
        <strong>{scenario.title}</strong>
        <button
          type="button"
          sx={stylexStyles.rule3}
          onClick={() => {
            void reset()
          }}
        >
          Reset scenario
        </button>
        <label sx={stylexStyles.rule4}>
          <input
            type="checkbox"
            checked={compare}
            onChange={(event) => setCompare(event.target.checked)}
          />
          Compare two apps
        </label>
        {resetError && <span role="alert">{resetError}</span>}
      </div>
      <div sx={stylexStyles2.rule1}>
        <SpacewaveApp
          sx={nativeColumnDividers.tcurrent}
          key={appId}
          appId={appId}
          suppliedStorage={ephemeralStorage}
          setup={scenario.setup}
        />
        {compare && (
          <SpacewaveApp
            sx={nativeColumnDividers.tcurrent}
            key={`${appId}:comparison`}
            appId={`${appId}:comparison`}
            suppliedStorage={ephemeralStorage}
            setup={scenario.setup}
          />
        )}
      </div>
    </div>
  )
}

// nativeColumnDividers applies the list border only to its nonfinal physical columns.
const nativeColumnDividers = stylex.create({
  tcurrent: {
    '--tw-divide-x-reverse': { default: null, ':not(:last-child)': 0 },
    borderColor: { default: null, ':not(:last-child)': 'currentcolor' },
    borderInlineEndStyle: {
      default: null,
      ':not(:last-child)': dividerState['--tw-border-style'],
    },
    borderInlineEndWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * calc(1 - var(--tw-divide-x-reverse)))',
    },
    borderInlineStartStyle: {
      default: null,
      ':not(:last-child)': dividerState['--tw-border-style'],
    },
    borderInlineStartWidth: {
      default: null,
      ':not(:last-child)': 'calc(1px * var(--tw-divide-x-reverse))',
    },
  },
})
