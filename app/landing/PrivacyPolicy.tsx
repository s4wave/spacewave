import { LuShield } from 'react-icons/lu'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'Privacy Policy - Spacewave',
  description:
    'Learn how Spacewave handles account, billing, usage, and encrypted content data for its local-first platform and optional cloud services.',
  canonicalPath: '/privacy',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// PrivacyPolicy renders the privacy policy page.
export function PrivacyPolicy() {
  return (
    <LegalPageLayout
      icon={<LuShield className="h-10 w-10" />}
      title="Privacy Policy"
      subtitle="How Spacewave handles your data and protects your privacy."
      lastUpdated="Last updated: March 2026"
    >
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <div className="space-y-6">
          {/* 1. Introduction */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              1. Introduction
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Aperture Robotics, LLC, a Delaware limited liability company
                (&ldquo;Company&rdquo;, &ldquo;we&rdquo;, &ldquo;us&rdquo;),
                operates Spacewave (&ldquo;Service&rdquo;). This Privacy Policy
                describes how we collect, use, disclose, and protect your
                information when you use the Service. This Privacy Policy is
                incorporated into and subject to our{' '}
                <a
                  href="#/tos"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  Terms of Service
                </a>
                .
              </p>
              <p>
                Spacewave is a client-side-first platform. Most data processing
                happens in your browser, not on our servers. All content you
                create, upload, or store using the Service (&ldquo;User
                Content,&rdquo; as defined in the Terms of Service) - whether
                stored in cloud storage or transmitted directly between devices
                - is end-to-end encrypted using keys derived from your
                credentials on your device. The Company does not hold decryption
                keys and cannot access the plaintext content of your data. This
                policy explains what data we do handle server-side and why.
              </p>
            </div>
          </div>

          {/* 2. Information We Collect */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              2. Information We Collect
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                2.1 Account Information
              </h3>
              <p>When you create an account, we collect:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">Email address</strong> -
                  required for account verification, billing, and service
                  communications (billing receipts, security alerts, Terms
                  updates).
                </li>
                <li>
                  <strong className="text-foreground">
                    Authentication credentials
                  </strong>{' '}
                  - for passkey and OAuth users, we store the server-side
                  credential data necessary for authentication. For
                  email/password users, your password is processed client-side
                  via scrypt key derivation; we receive only the derived
                  cryptographic public key, never your password.
                </li>
                <li>
                  <strong className="text-foreground">
                    Billing information
                  </strong>{' '}
                  - payment details are processed directly by Stripe and are not
                  stored on our servers. We receive only transaction
                  confirmations and subscription status from Stripe.
                </li>
              </ul>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.2 Usage Data
              </h3>
              <p>
                We collect diagnostic and usage-related data (&ldquo;Usage
                Data&rdquo;) solely for system quality-of-service monitoring,
                billing, and maintaining the operation of the Service. For
                paid-tier users who use cloud storage, this includes:
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">Storage volume</strong> -
                  total bytes stored in your cloud allocation
                </li>
                <li>
                  <strong className="text-foreground">Operation counts</strong>{' '}
                  - number of read/write operations for billing purposes
                </li>
                <li>
                  <strong className="text-foreground">Timestamps</strong> - when
                  operations occur (for billing cycle calculation)
                </li>
              </ul>
              <p>
                We do not use Usage Data for advertising, profiling, or any
                purpose other than those stated above.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.3 Technical Data
              </h3>
              <p>When you connect to our services, we may collect:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">IP address</strong> - for
                  rate limiting and abuse prevention
                </li>
                <li>
                  <strong className="text-foreground">User agent</strong> -
                  browser type and version
                </li>
                <li>
                  <strong className="text-foreground">
                    Connection metadata
                  </strong>{' '}
                  - timestamps, request counts
                </li>
              </ul>
              <p>
                We do not use tracking cookies, analytics scripts, or
                third-party tracking pixels. We do not use your data for
                advertising. The Service may use strictly necessary cookies or
                local storage for authentication and session management only;
                these are essential for the Service to function and cannot be
                opted out of.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.4 Data We Do NOT Collect
              </h3>
              <p>
                Spacewave&rsquo;s client-side architecture means we do not
                collect:
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>Content of your locally stored data (free tier)</li>
                <li>Content of direct device-to-device communications</li>
                <li>Browsing history within Spacewave applications</li>
                <li>Keystroke or interaction telemetry</li>
                <li>Precise geolocation data</li>
                <li>Device identifiers beyond user agent</li>
              </ul>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.5 Artificial Intelligence Features
              </h3>
              <p>
                The Service may include features that use artificial
                intelligence or machine learning technology (&ldquo;AI
                Features&rdquo;) as described in the Terms of Service. When you
                use AI Features, the input you provide and any resulting output
                may be processed by the AI Feature to generate a response. We do
                not use your input to AI Features or any AI-generated output to
                train artificial intelligence or machine learning models. AI
                Feature processing is subject to the same data protections
                described in this Privacy Policy.
              </p>
            </div>
          </div>

          {/* 3. How We Use Your Information */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              3. How We Use Your Information
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>We use collected information solely to:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>Provide, maintain, and operate the Service</li>
                <li>Process payments and manage subscriptions</li>
                <li>
                  Send service communications (billing, security, Terms updates)
                </li>
                <li>
                  Prevent abuse, enforce our{' '}
                  <a
                    href="#/tos"
                    className="text-brand hover:text-brand-highlight underline"
                  >
                    Terms of Service
                  </a>
                  , and maintain security
                </li>
                <li>Comply with applicable legal obligations</li>
                <li>Monitor system quality of service and billing accuracy</li>
              </ul>
              <p>
                We do not sell, rent, or share your personal information for
                advertising or marketing purposes. We do not use your
                information for profiling or automated decision-making that
                produces legal or similarly significant effects.
              </p>
            </div>
          </div>

          {/* 4. Data Storage and Security */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              4. Data Storage and Security
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                4.1 Cloud Storage (Paid Tier)
              </h3>
              <p>
                Paid-tier cloud data is stored on Cloudflare R2 infrastructure.
                All User Content is end-to-end encrypted on your device before
                transmission; the Company does not hold decryption keys and
                cannot access the plaintext content of your cloud-stored data.
                Data is additionally encrypted in transit (TLS) and Cloudflare
                provides encryption at rest for R2 storage.
              </p>
              <p>
                Because User Content is end-to-end encrypted, we are technically
                unable to inspect, read, or review the plaintext content of your
                cloud-stored data. If we have reason to believe that your use of
                the Service violates our Terms of Service (including prohibited
                content restrictions), we may suspend or terminate your access
                to the Service and remove the encrypted data from our
                infrastructure. We may also disclose encrypted data in response
                to valid legal process, though we are unable to decrypt such
                data.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.2 Client-Side Data
              </h3>
              <p>
                Data stored locally in your browser (free tier or local mode) is
                under your control. We have no access to locally stored data.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.3 Direct Device-to-Device Data
              </h3>
              <p>
                Data transmitted via direct device-to-device connections is
                end-to-end encrypted and passes directly between devices. We do
                not relay, intercept, or store direct device-to-device traffic.
                When using the optional paid cloud relay feature, data passes
                through Cloudflare infrastructure in transit only, remains
                end-to-end encrypted, and is not stored. The Company cannot
                access the plaintext content of direct device-to-device
                communications in any case.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.4 Security Measures
              </h3>
              <p>
                We implement security measures including end-to-end encryption
                of all User Content, TLS encryption for all connections, secure
                authentication (scrypt key derivation, passkey support), and
                access controls. Encryption keys are derived on your device and
                are never transmitted to or stored on our servers. However, no
                method of transmission over the Internet or method of electronic
                storage is completely secure, and we cannot guarantee absolute
                security.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.5 Data Breach Notification
              </h3>
              <p>
                In the event of a data breach that affects your personal
                information, we will notify you by email to the address
                associated with your account and, where required by applicable
                law, the relevant supervisory authorities, within the timeframes
                required by applicable law. Such notification will describe the
                nature of the breach and the steps we are taking in response.
              </p>
            </div>
          </div>

          {/* 5. Data Sharing */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              5. Data Sharing
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>We share your information only with:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">Stripe</strong> - payment
                  processing (governed by{' '}
                  <a
                    href="https://stripe.com/privacy"
                    className="text-brand hover:text-brand-highlight underline"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Stripe&rsquo;s Privacy Policy
                  </a>
                  )
                </li>
                <li>
                  <strong className="text-foreground">Cloudflare</strong> -
                  infrastructure provider for cloud storage and data relay
                  (governed by{' '}
                  <a
                    href="https://www.cloudflare.com/privacypolicy/"
                    className="text-brand hover:text-brand-highlight underline"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Cloudflare&rsquo;s Privacy Policy
                  </a>
                  ). The Service also uses Cloudflare Turnstile for bot
                  detection and abuse prevention, which is subject to the{' '}
                  <a
                    href="https://www.cloudflare.com/turnstile-privacy-policy/"
                    className="text-brand hover:text-brand-highlight underline"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    Cloudflare Turnstile Privacy Policy
                  </a>
                </li>
                <li>
                  <strong className="text-foreground">
                    Law enforcement or government authorities
                  </strong>{' '}
                  - only when required by valid legal process (subpoena, court
                  order, or equivalent). We will notify you of such requests
                  unless prohibited by law or court order.
                </li>
              </ul>
              <p>
                We do not sell, rent, or share your personal information with
                third parties for marketing or advertising purposes. We do not
                share personal information with data brokers.
              </p>
            </div>
          </div>

          {/* 6. Data Retention */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              6. Data Retention
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <ul className="list-disc space-y-2 pl-5">
                <li>
                  <strong className="text-foreground">Account data</strong> -
                  retained while your account is active. Deleted within thirty
                  (30) days of account closure or voluntary cancellation.
                </li>
                <li>
                  <strong className="text-foreground">Cloud-stored data</strong>{' '}
                  - retained while your subscription is active. Upon voluntary
                  cancellation, available for export for thirty (30) days, then
                  permanently deleted. Upon termination by the Company for
                  convenience, available for export for thirty (30) days from
                  the effective date of termination, then permanently deleted.
                  Upon termination for cause, may be immediately and permanently
                  deleted without an export period. During suspension for
                  non-payment, data will be preserved for thirty (30) days from
                  the date of suspension, after which it may be permanently
                  deleted.
                </li>
                <li>
                  <strong className="text-foreground">Billing records</strong> -
                  retained as required by applicable tax and accounting law
                  (typically seven (7) years).
                </li>
                <li>
                  <strong className="text-foreground">Server logs</strong> -
                  retained for up to ninety (90) days for abuse prevention,
                  security, and debugging, then deleted.
                </li>
                <li>
                  <strong className="text-foreground">Backup copies</strong> -
                  residual copies in backup or archival systems may be retained
                  for a commercially reasonable period after deletion of the
                  primary data.
                </li>
              </ul>
            </div>
          </div>

          {/* 7. Your Rights */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              7. Your Rights
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>You have the right to:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">Access</strong> - request
                  a copy of personal data we hold about you
                </li>
                <li>
                  <strong className="text-foreground">Correction</strong> -
                  request correction of inaccurate personal data
                </li>
                <li>
                  <strong className="text-foreground">Deletion</strong> -
                  request deletion of your account and associated personal data
                </li>
                <li>
                  <strong className="text-foreground">Export</strong> - export
                  your cloud-stored data at any time through the Service
                </li>
                <li>
                  <strong className="text-foreground">
                    Opt-out of communications
                  </strong>{' '}
                  - unsubscribe from non-essential communications
                </li>
              </ul>
              <p>
                To exercise these rights, contact privacy@aperture.us. We will
                respond to verified requests within thirty (30) days or such
                shorter period as required by applicable law. We will not
                discriminate against you for exercising any of these rights.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                7.1 California Residents (CCPA/CPRA)
              </h3>
              <p>
                If you are a California resident, you have additional rights
                under the California Consumer Privacy Act as amended by the
                California Privacy Rights Act (collectively,
                &ldquo;CCPA&rdquo;):
              </p>
              <p>
                <strong className="text-foreground">
                  Categories of personal information we collect:
                </strong>{' '}
                identifiers (email address, IP address), commercial information
                (billing and subscription status), and internet or other
                electronic network activity information (usage data, user agent,
                connection metadata).
              </p>
              <p>
                <strong className="text-foreground">Sources:</strong> directly
                from you (account creation, payment) and automatically from your
                use of the Service (technical data, usage data).
              </p>
              <p>
                <strong className="text-foreground">
                  Business purpose for collection:
                </strong>{' '}
                providing and maintaining the Service, processing payments,
                security and abuse prevention, and legal compliance.
              </p>
              <p>
                <strong className="text-foreground">
                  Third parties with whom we share:
                </strong>{' '}
                Stripe (payment processing) and Cloudflare (infrastructure), as
                described in Section 5.
              </p>
              <p>
                <strong className="text-foreground">Your CCPA rights:</strong>
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  Right to know what personal information we collect, use, and
                  disclose
                </li>
                <li>Right to delete personal information</li>
                <li>Right to correct inaccurate personal information</li>
                <li>
                  Right to opt-out of the sale or sharing of personal
                  information - we do not sell or share (as defined by the CCPA)
                  your personal information
                </li>
                <li>
                  Right to non-discrimination for exercising your privacy rights
                </li>
                <li>
                  Right to limit use and disclosure of sensitive personal
                  information - we do not use sensitive personal information for
                  purposes beyond those permitted by the CCPA
                </li>
              </ul>
              <p>
                To exercise these rights, contact privacy@aperture.us or submit
                a request through your account settings.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                7.2 EU/EEA and UK Residents (GDPR/UK GDPR)
              </h3>
              <p>
                <strong className="text-foreground">Data controller:</strong>{' '}
                Aperture Robotics, LLC is the data controller for personal data
                processed in connection with the Service. For data protection
                inquiries, contact privacy@aperture.us.
              </p>
              <p>
                <strong className="text-foreground">
                  Legal bases for processing:
                </strong>
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  <strong className="text-foreground">
                    Contract performance
                  </strong>{' '}
                  (GDPR Art. 6(1)(b)) - processing of account information,
                  billing information, and cloud-stored data, as necessary to
                  provide the Service you requested
                </li>
                <li>
                  <strong className="text-foreground">
                    Legitimate interest
                  </strong>{' '}
                  (GDPR Art. 6(1)(f)) - processing of technical data (IP
                  address, user agent, connection metadata) for abuse
                  prevention, security, and system quality-of-service
                  monitoring. We have balanced our interests against your rights
                  and have determined that our processing is proportionate and
                  does not override your fundamental rights
                </li>
                <li>
                  <strong className="text-foreground">Legal obligation</strong>{' '}
                  (GDPR Art. 6(1)(c)) - retention of billing records as required
                  by tax and accounting law
                </li>
              </ul>
              <p>
                <strong className="text-foreground">
                  Your additional GDPR rights:
                </strong>
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>Right to data portability</li>
                <li>Right to restrict processing</li>
                <li>
                  Right to object to processing based on legitimate interests
                </li>
                <li>
                  Right to withdraw consent (where processing is based on
                  consent)
                </li>
                <li>
                  Right to lodge a complaint with your local data protection
                  authority
                </li>
              </ul>
              <p>
                We do not engage in automated decision-making or profiling that
                produces legal or similarly significant effects concerning you.
              </p>
            </div>
          </div>

          {/* 8. Do Not Track */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              8. Do Not Track
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Some browsers transmit &ldquo;Do Not Track&rdquo; (DNT) signals.
                We do not use tracking cookies, third-party analytics, or
                advertising trackers, so our practices are consistent with a DNT
                preference regardless of whether the signal is received.
              </p>
            </div>
          </div>

          {/* 9. Children's Privacy */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              9. Children&apos;s Privacy
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Spacewave is not directed at children under 16. We do not
                knowingly collect personal information from children under 16.
                If we learn that we have collected personal information from a
                child under 16, we will delete it promptly. If you believe a
                child under 16 has provided us with personal information,
                contact privacy@aperture.us.
              </p>
            </div>
          </div>

          {/* 10. International Data Transfers */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              10. International Data Transfers
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Your data may be processed in the United States and other
                countries where Cloudflare operates infrastructure. For
                transfers of personal data from the EU/EEA or United Kingdom to
                the United States or other countries that have not received an
                adequacy determination, we rely on Cloudflare&rsquo;s data
                processing agreements incorporating the European
                Commission&rsquo;s Standard Contractual Clauses (SCCs) and the
                UK International Data Transfer Addendum, as applicable. You may
                request a copy of the applicable transfer safeguards by
                contacting privacy@aperture.us.
              </p>
            </div>
          </div>

          {/* 11. Changes to This Policy */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              11. Changes to This Policy
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                We may update this Privacy Policy from time to time. If we make
                material changes, we will notify you by email to the address
                associated with your account when the updated policy takes
                effect. Non-material changes (such as clarifications or
                formatting updates) may be made without advance notice. The
                &ldquo;Last Updated&rdquo; date at the bottom of this policy
                indicates when it was last revised. Your continued use of the
                Service after the effective date of any changes constitutes your
                acceptance of the updated Privacy Policy.
              </p>
            </div>
          </div>

          {/* 12. Contact */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              12. Contact
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>For privacy-related questions or requests:</p>
              <div className="bg-background/50 border-foreground/8 rounded border p-4 font-mono text-xs leading-relaxed">
                Aperture Robotics, LLC
                <br />
                Email:{' '}
                <a
                  href="mailto:privacy@aperture.us"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  privacy@aperture.us
                </a>
                <br />
                Web:{' '}
                <a
                  href="#/"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  spacewave.app
                </a>
              </div>
            </div>
          </div>
        </div>
      </section>
    </LegalPageLayout>
  )
}
