import { LuShield } from 'react-icons/lu'

import policies from './customer-policies.json'
import { LegalPageLayout } from './LegalPageLayout.js'
import { LegalPolicyContent } from './LegalPolicyContent.js'

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
      icon={<LuShield className="size-10" />}
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
