import { LuFileText } from 'react-icons/lu'

import policies from './customer-policies.json'
import { LegalPageLayout } from './LegalPageLayout.js'
import { LegalPolicyContent } from './LegalPolicyContent.js'

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
      icon={<LuFileText className="size-10" />}
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
