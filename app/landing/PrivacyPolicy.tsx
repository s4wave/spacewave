import { LuShield } from '@s4wave/web/images/icons.js'

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
  title: 'Privacy Policy - Spacewave',
  description:
    'How Spacewave handles encrypted content, account and billing records, service providers, retention, deletion, and privacy requests.',
  canonicalPath: '/privacy',
}

// PrivacyPolicy renders the canonical privacy and retention account.
export function PrivacyPolicy() {
  return (
    <LegalPageLayout
      icon={<LuShield sx={stylexStyles.rule1} />}
      title="Privacy Policy"
      subtitle="What stays on your devices and what our cloud service handles."
      lastUpdated={
        policies.privacy.effectiveDate
          ? `Effective date: ${policies.privacy.effectiveDate}`
          : `Policy version: ${policies.privacy.version}`
      }
      draftBanner={policies.privacy.draft}
    >
      <LegalPolicyContent sections={policies.privacy.sections} />
    </LegalPageLayout>
  )
}
