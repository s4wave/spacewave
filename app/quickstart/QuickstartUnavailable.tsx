import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { Button } from '@s4wave/web/ui/button.js'

import * as stylex from '@stylexjs/stylex'
import {
  colors as themeColors,
  layout as themeLayout,
  utilityState as themeUtilityState,
} from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    flex: 1,
    alignItems: 'center',
    backgroundColor: themeColors['--color-background-landing'],
    display: 'flex',
    justifyContent: 'center',
    minHeight: '100vh',
    width: '100%',
  },
  rule2: {
    '--tw-shadow':
      '0 1px 3px 0 var(--tw-shadow-color, #0000001a), 0 1px 2px -1px var(--tw-shadow-color, #0000001a)',
    padding: `calc(${themeLayout['--spacing']} * 6)`,
    borderRadius: 'var(--radius-2xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 90%, transparent)`,
    boxShadow:
      'var(--tw-inset-shadow), var(--tw-inset-ring-shadow), var(--tw-ring-offset-shadow), var(--tw-ring-shadow), var(--tw-shadow)',
    maxWidth: 'var(--container-md)',
    width: '100%',
  },
  rule3: {
    display: 'flex',
    justifyContent: 'center',
    marginTop: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

interface QuickstartUnavailableProps {
  quickstartId?: string
  homeHref?: string
  onBack?: () => void
}

export function QuickstartUnavailable({
  quickstartId,
  homeHref = '/',
  onBack,
}: QuickstartUnavailableProps) {
  const description = quickstartId
    ? `The "${quickstartId}" quickstart is not part of the current public Spacewave catalog. Choose an available quickstart from the home page.`
    : 'This quickstart route is not part of the current public Spacewave catalog. Choose an available quickstart from the home page.'

  return (
    <div sx={stylexStyles.rule1}>
      <div sx={stylexStyles.rule2}>
        <EmptyState
          title="Quickstart not available"
          description={description}
          variant="compact"
        />
        <div sx={stylexStyles.rule3}>
          {onBack ? (
            <Button type="button" variant="outline" onClick={onBack}>
              Back to home
            </Button>
          ) : (
            <Button asChild variant="outline">
              <a href={homeHref}>Back to home</a>
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
