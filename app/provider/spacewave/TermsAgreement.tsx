import { useStaticHref } from '@s4wave/app/prerender/StaticContext.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  fonts as themeFonts,
} from '../../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    display: 'flex',
    cursor: 'pointer',
    alignItems: 'flex-start',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    WebkitUserSelect: 'none',
    userSelect: 'none',
  },
  rule2: {
    marginTop: `calc(${themeLayout['--spacing']} * .5)`,
    width: `calc(${themeLayout['--spacing']} * 4)`,
    height: `calc(${themeLayout['--spacing']} * 4)`,
    flexShrink: 0,
    borderRadius: '.25rem',
    accentColor: themeColors['--color-brand'],
  },
  rule3: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule4: {
    color: themeColors['--color-brand'],
    textDecorationLine: {
      default: null,
      '@media (hover: hover)': { default: null, ':hover': 'underline' },
    },
  },
})

export interface TermsAgreementProps {
  agreed: boolean
  onChange: (agreed: boolean) => void
  disabled: boolean
}

// TermsAgreement renders the checkbox a new Spacewave account must tick to
// accept the Terms of Service and Privacy Policy. The links open in a new tab
// so the pending sign-up survives reading them.
export function TermsAgreement({
  agreed,
  onChange,
  disabled,
}: TermsAgreementProps) {
  const tosHref = useStaticHref('/tos')
  const privacyHref = useStaticHref('/privacy')

  return (
    <label sx={stylexStyles.rule1}>
      <input
        type="checkbox"
        checked={agreed}
        onChange={(e) => onChange(e.target.checked)}
        disabled={disabled}
        sx={stylexStyles.rule2}
      />
      <span sx={stylexStyles.rule3}>
        I agree to the{' '}
        <a
          href={tosHref}
          target="_blank"
          rel="noopener noreferrer"
          sx={stylexStyles.rule4}
        >
          Terms of Service
        </a>{' '}
        and{' '}
        <a
          href={privacyHref}
          target="_blank"
          rel="noopener noreferrer"
          sx={stylexStyles.rule4}
        >
          Privacy Policy
        </a>
      </span>
    </label>
  )
}
