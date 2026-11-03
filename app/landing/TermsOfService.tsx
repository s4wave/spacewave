import { LuFileText } from '@s4wave/web/images/icons.js'

import policies from './customer-policies.json'
import { LegalPageLayout } from './LegalPageLayout.js'
import { LegalPolicyContent } from './LegalPolicyContent.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 10)`,
    width: `calc(${themeLayout['--spacing']} * 10)`,
  },
})

export const metadata = {
  title: 'Terms of Service - Spacewave',
  description:
    'Spacewave service terms, monthly cloud subscriptions, cancellation, content rights, dispute resolution, and business data processing.',
  canonicalPath: '/tos',
}

// TermsOfService renders the canonical service terms and processing addendum.
export function TermsOfService() {
  return (
    <LegalPageLayout
      icon={<LuFileText sx={stylexStyles.rule1} />}
      title="Terms of Service"
      subtitle="Your service, subscription, and content rights."
      lastUpdated={
        policies.terms.effectiveDate
          ? `Effective date: ${policies.terms.effectiveDate}`
          : `Policy version: ${policies.terms.version}`
      }
      draftBanner={policies.terms.draft}
    >
      <LegalPolicyContent sections={policies.terms.sections} />
    </LegalPageLayout>
  )
}
