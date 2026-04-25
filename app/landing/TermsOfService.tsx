import { LuFileText } from 'react-icons/lu'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'Terms of Service - Spacewave',
  description: 'Spacewave terms of service and usage agreement.',
  canonicalPath: '/tos',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// TermsOfService renders the terms of service page.
export function TermsOfService() {
  return (
    <LegalPageLayout
      icon={<LuFileText className="h-10 w-10" />}
      title="Terms of Service"
      subtitle="Please review the terms and conditions for Spacewave."
      lastUpdated="Last updated: March 2026"
    >
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-14 @lg:px-8 @lg:pb-16">
        <div className="space-y-6">
          {/* Auto-renewal notice */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <div className="space-y-3">
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                PLEASE NOTE: IF YOU SUBSCRIBE TO THE SERVICES FOR A SUBSCRIPTION
                TERM, YOUR SUBSCRIPTION WILL AUTOMATICALLY RENEW FOR SUCCESSIVE
                BILLING PERIODS AT THE THEN-CURRENT PRICING UNLESS YOU CANCEL
                BEFORE THE RENEWAL DATE. SEE SECTION 4.3 BELOW.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                PLEASE NOTE: SECTION 14 CONTAINS AN ARBITRATION AGREEMENT THAT
                REQUIRES DISPUTES TO BE RESOLVED THROUGH BINDING ARBITRATION ON
                AN INDIVIDUAL BASIS. YOU MAY OPT OUT WITHIN 30 DAYS. SEE SECTION
                14 FOR DETAILS.
              </p>
            </div>
          </div>

          {/* 1. Agreement to Terms */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              1. Agreement to Terms
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                By creating an account, accessing, or using Spacewave
                (&ldquo;Service&rdquo;), operated by Aperture Robotics, LLC, a
                Delaware limited liability company (&ldquo;Company&rdquo;,
                &ldquo;we&rdquo;, &ldquo;us&rdquo;), you (&ldquo;you&rdquo; or
                &ldquo;User&rdquo;) acknowledge that you have read, understood,
                and agree to be bound by these Terms of Service
                (&ldquo;Terms&rdquo;). If you do not agree to these Terms, you
                must not access or use the Service.
              </p>
              <p>
                You must be at least 16 years old to use Spacewave. If you are
                under 18, you represent that your parent or legal guardian has
                read, understood, and agreed to be bound by these Terms on your
                behalf.
              </p>
              <p>
                Each party represents and warrants that it has the legal power
                and authority to enter into these Terms and to perform its
                obligations hereunder.
              </p>
            </div>
          </div>

          {/* 2. Description of Service */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              2. Description of Service
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Spacewave is a client-side application platform that enables
                users to self-host applications in the browser. The Service
                includes:
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  Open-source client-side application runtime (runs in your
                  browser or desktop)
                </li>
                <li>Direct device-to-device networking via web technologies</li>
                <li>Optional cloud storage and data relay (paid tier)</li>
              </ul>
              <p>
                The free tier operates entirely locally: no cloud infrastructure
                is used for free-tier users.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.1 Beta Services
              </h3>
              <p>
                The Service is currently in beta. Beta features may be changed,
                suspended, or discontinued at any time without prior notice.
                Beta features may not be as reliable or available as generally
                available services and have not been subjected to the same level
                of testing.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                BETA SERVICES ARE PROVIDED &ldquo;AS IS&rdquo; WITHOUT WARRANTY
                OF ANY KIND, EXPRESS OR IMPLIED. NOTWITHSTANDING ANY OTHER
                PROVISION OF THESE TERMS, THE COMPANY SHALL HAVE NO LIABILITY OF
                ANY KIND WITH RESPECT TO BETA SERVICES TO THE MAXIMUM EXTENT
                PERMITTED BY APPLICABLE LAW. IN JURISDICTIONS THAT DO NOT PERMIT
                A COMPLETE EXCLUSION OF LIABILITY, THE COMPANY&rsquo;S TOTAL
                LIABILITY FOR BETA SERVICES SHALL NOT EXCEED TEN U.S. DOLLARS
                (US $10).
              </p>
              <p>
                We will use commercially reasonable efforts to notify you of
                material changes to beta features, but failure to provide such
                notice shall not give rise to any liability.
              </p>
              <p>
                When the Service exits beta and becomes generally available
                (&ldquo;GA Transition&rdquo;), we will provide written notice to
                the email address associated with your account. Upon GA
                Transition: a) pricing, plan baselines, and feature availability
                may change; b) these Terms may be updated in accordance with
                Section 18.1; and c) your continued use of the Service after the
                GA Transition date constitutes acceptance of the then-current
                Terms and pricing. We will use commercially reasonable efforts
                to migrate your existing data and account configuration to the
                generally available Service, but we do not guarantee that all
                beta-period data, configurations, or customizations will be
                preserved. You are encouraged to export your data before the GA
                Transition date.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.2 Software
              </h3>
              <p>
                The Company may make client-side software applications available
                for download or installation as part of the Service
                (&ldquo;Software&rdquo;). Subject to the terms and conditions of
                these Terms, the Company grants you a limited, non-exclusive,
                non-transferable, non-sublicensable, revocable license to
                download and install the Software solely to the extent necessary
                to access and use the Service. Software may update
                automatically. This license automatically terminates upon
                expiration or termination of these Terms or closure of your
                account. To the extent a component of the Software contains
                open-source software, the applicable open-source license for
                that component shall govern with respect to that component.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.3 Usage Data
              </h3>
              <p>
                We may collect diagnostic and usage-related data from the use,
                performance, and operation of the Service (&ldquo;Usage
                Data&rdquo;), including usage patterns, traffic logs, and
                engagement metrics. We use Usage Data solely for system
                quality-of-service monitoring, billing, and maintaining the
                operation of the Service. We will not disclose Usage Data to any
                third party in a manner that identifies you or any individual
                User, except as required by applicable law.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.4 Data Storage and Lifecycle
              </h3>
              <p>
                Your cloud-stored data may be stored across multiple storage
                tiers to optimize cost and performance. Data that has not been
                accessed for an extended period (currently thirty (30) or more
                days) may be migrated to cold storage. Data in cold storage
                remains fully accessible but may experience higher retrieval
                latency compared to frequently accessed data. We will use
                commercially reasonable efforts to minimize any impact on your
                experience, and the migration between storage tiers is
                transparent to your use of the Service. Cold storage migration
                does not affect the durability or availability of your data. The
                specific inactivity threshold for cold storage migration may be
                adjusted at our discretion.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                2.5 Artificial Intelligence Features
              </h3>
              <p>
                The Service may include features that use or leverage artificial
                intelligence or machine learning technology, including large
                language models (&ldquo;AI Features&rdquo;). You acknowledge and
                agree that any text, information, analyses, results, content,
                recommendations, or other materials generated by AI Features
                (&ldquo;Output&rdquo;) are provided &ldquo;AS IS&rdquo; and
                &ldquo;WITH ALL FAULTS.&rdquo; The Company makes no
                representations, warranties, or guarantees of any kind with
                respect to AI Features or any Output, including with respect to
                accuracy, completeness, reliability, timeliness, or suitability.
                Given the probabilistic nature of artificial intelligence
                technology, Output may be inaccurate, incomplete, or
                inappropriate. You are solely responsible for evaluating and
                making all decisions based on any Output, and the Company shall
                have no responsibility or liability arising from your reliance
                on any Output.
              </p>
            </div>
          </div>

          {/* 3. Accounts and Authentication */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              3. Accounts and Authentication
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                3.1 Account Creation
              </h3>
              <p>
                You may create an account using email/password, passkeys, or
                third-party OAuth providers. You are responsible for maintaining
                the confidentiality and security of your authentication
                credentials. You must not share your account credentials with
                any other person. You are solely responsible for all activities
                that occur under your account, whether or not authorized by you.
              </p>
              <p>
                For email/password authentication, your password is processed
                client-side (scrypt key derivation) to generate a cryptographic
                keypair. We do not store your password.
              </p>
              <p>
                If you become aware of or reasonably suspect any unauthorized
                use of your account or any other breach of security, you must
                promptly notify us at legal@aperture.us.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                3.2 Email Verification
              </h3>
              <p>
                A verified email address is required before subscribing to a
                paid plan or making any payment.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                3.3 Billing Accounts and Organizations
              </h3>
              <p>
                You may create one or more billing accounts (&ldquo;Billing
                Account&rdquo;), each with its own subscription. Each Billing
                Account has a primary user (owner) and may be attached to one or
                more organizations (&ldquo;Organization&rdquo;) to fund that
                Organization&rsquo;s resources. You are solely responsible for
                all charges incurred under your Billing Account(s) and for the
                actions of all users within any Organization funded by your
                Billing Account(s).
              </p>
            </div>
          </div>

          {/* 4. Subscriptions and Payment */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              4. Subscriptions and Payment
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                4.1 Free Tier
              </h3>
              <p>
                The free tier provides full client-side functionality with
                direct device-to-device networking at no cost and with no cloud
                storage. We may modify or discontinue the free tier at any time
                without prior notice or liability.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                NOTWITHSTANDING ANY OTHER PROVISION OF THESE TERMS, THE FREE
                SERVICES ARE PROVIDED &ldquo;AS IS&rdquo; AND &ldquo;AS
                AVAILABLE&rdquo; WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
                IMPLIED, INCLUDING BUT NOT LIMITED TO THE IMPLIED WARRANTIES OF
                MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, TITLE, AND
                NON-INFRINGEMENT. IN NO EVENT SHALL THE COMPANY, ITS MEMBERS,
                MANAGERS, EMPLOYEES, OR AGENTS BE LIABLE FOR ANY DIRECT,
                INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
                DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
                SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR
                BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF
                LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
                (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
                THE USE OF THE FREE SERVICES, EVEN IF ADVISED OF THE POSSIBILITY
                OF SUCH DAMAGE.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.2 Paid Tier
              </h3>
              <p>
                Service plans, features, and pricing are described on our
                pricing page at{' '}
                <a
                  href="#/pricing"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  spacewave.app/pricing
                </a>
                . We reserve the right to change pricing at any time, effective
                upon the next renewal term. We will notify you by email of
                pricing changes before your next renewal.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.3 Auto-Renewal
              </h3>
              <p>
                Paid subscriptions automatically renew at the end of each
                monthly billing period at the then-current pricing unless you
                cancel before the renewal date. By subscribing to a paid plan,
                you authorize us to charge your payment method on file for each
                renewal period. Your failure to cancel before the renewal date
                constitutes your authorization to charge the applicable renewal
                fees.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.4 Baseline Allocation and Overage
              </h3>
              <p>
                Each paid plan includes a baseline allocation of storage, write
                operations, and read operations per billing period, as specified
                on the pricing page at{' '}
                <a
                  href="#/pricing"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  spacewave.app/pricing
                </a>
                . As of the effective date of these Terms, the standard paid
                plan baseline allocation is 100 GB of cloud storage, 1,000,000
                write operations, and 10,000,000 read operations per month. We
                reserve the right to adjust baseline allocations at any time,
                effective upon the next renewal term.
              </p>
              <p>
                Usage exceeding your plan&rsquo;s baseline allocation will be
                billed at the overage rates published on the pricing page at the
                time the overage is incurred. Overage charges are billed in
                arrears and added to your next invoice. Exceeding your baseline
                allocation does not result in service suspension or termination;
                overage usage is accommodated and billed accordingly. We will
                provide reasonable tools for you to monitor your usage,
                including notifications when you approach or exceed your
                baseline allocation thresholds.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.5 Taxes
              </h3>
              <p>
                All fees are exclusive of taxes. You are responsible for all
                applicable sales, use, excise, value-added, and other similar
                taxes, duties, levies, and charges of any kind imposed by any
                federal, state, local, or foreign governmental authority on any
                amounts payable under these Terms, other than taxes imposed on
                the Company&rsquo;s net income.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.6 Billing and Late Payment
              </h3>
              <p>
                Subscriptions are billed in advance. All fees are
                non-refundable, including fees for partially used subscription
                periods, except as expressly provided in these Terms or as
                required by applicable law. You must keep your billing
                information current and accurate.
              </p>
              <p>
                If payment fails, we will notify you at the email address
                associated with your account and may suspend access to paid
                features after five (5) days following such notification. If
                such failure to pay continues, we may charge interest on all
                past due amounts at the rate of one and a half percent (1.5%)
                per month, or the maximum rate permitted by applicable law,
                whichever is lower, calculated from the date payment was
                originally due. Suspension does not relieve you of the
                obligation to pay all outstanding amounts and accrued interest.
                During any period of suspension, your cloud-stored data will be
                preserved for thirty (30) days from the date of suspension. If
                the suspension is not resolved within that period, we may
                permanently delete your cloud-stored data.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.7 Cancellation and Refunds
              </h3>
              <p>
                You may cancel your subscription at any time through your
                account settings. Two cancellation options are available:
              </p>

              <h4 className="text-foreground mt-3 mb-1 text-sm font-semibold">
                4.7.1 Standard Cancellation (End of Period)
              </h4>
              <p>
                Cancellation takes effect at the end of the current billing
                period. You retain full access to paid features and your
                cloud-stored data until the end of the period, and you will not
                be charged for subsequent periods. Upon cancellation, your
                cloud-stored data will be available for export for thirty (30)
                days after the end of the billing period, after which it will be
                permanently deleted.
              </p>

              <h4 className="text-foreground mt-3 mb-1 text-sm font-semibold">
                4.7.2 Immediate Cancellation with Account Deletion
              </h4>
              <p>
                You may request immediate cancellation with full account
                deletion. A final billing reconciliation determines whether a
                refund is issued or an outstanding balance remains. To request
                immediate cancellation with account deletion:
              </p>
              <ol className="list-decimal space-y-1 pl-5">
                <li>
                  You must verify your identity via a confirmation link sent to
                  the email address associated with your account.
                </li>
                <li>
                  Upon verification, a twenty-four (24) hour waiting period
                  begins. During this period, your account becomes read-only and
                  you may undo the deletion request at any time through your
                  account settings. We will notify you by email when the waiting
                  period begins and again before deletion is executed.
                </li>
                <li>
                  After the waiting period expires, your account and all
                  cloud-stored data will be permanently and irreversibly
                  deleted. Local data on your devices is not affected.
                </li>
                <li>
                  Refund calculation: the unused prepaid portion of the current
                  billing period (calculated on a daily basis) is offset against
                  accrued overage charges and the Stripe fixed processing fee
                  described below. If a net credit remains, we issue an
                  immediate refund to the original payment method. If no net
                  credit remains, no refund is issued and you remain responsible
                  for any outstanding balance.
                </li>
                <li>
                  The Stripe processing fee ($0.30 fixed fee per original
                  transaction) is not refundable, as Stripe retains this amount
                  regardless of refund.
                </li>
              </ol>
              <p>
                Once account deletion is executed, the action is irreversible.
                We cannot recover deleted data.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.8 Billing Disputes
              </h3>
              <p>
                If you reasonably and in good faith believe that you have been
                billed incorrectly, you must notify us at legal@aperture.us no
                later than thirty (30) days after the date of the charge in
                question. Your notice must describe the nature of the dispute in
                reasonable detail. All undisputed amounts remain due and payable
                in accordance with these Terms. The parties will work together
                in good faith to resolve the dispute within fifteen (15) days.
                If no resolution is agreed upon, each party may pursue any
                remedies available under these Terms or applicable law.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.9 Suspension and Termination for Cause
              </h3>
              <p>
                We may suspend or terminate your access to the Service, in whole
                or in part, immediately and without prior notice if you: a)
                materially breach these Terms, including the restrictions in
                Section 5.4 (Prohibited Content) or Section 6 (Acceptable Use);
                b) use the Service in a manner that threatens the security,
                integrity, or availability of the Service; or c) are required to
                be suspended or terminated by applicable law. Upon termination
                for cause, we may in our sole discretion immediately and
                permanently delete your User Content and account data without
                any obligation to provide an export period. Any fees accrued
                prior to termination remain due and payable.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.10 Termination for Convenience
              </h3>
              <p>
                We may terminate your account or any subscription for any reason
                or no reason upon thirty (30) days&rsquo; prior written notice
                to the email address associated with your account. If we
                terminate a paid subscription under this section, we will
                provide a pro-rata refund of any prepaid fees for the remainder
                of the then-current billing period. Upon termination under this
                section, your cloud-stored data will be available for export for
                thirty (30) days from the effective date of termination, after
                which it will be permanently deleted.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                4.11 Trial and Promotional Periods
              </h3>
              <p>
                From time to time we may offer a free trial or other promotional
                billing period in connection with a paid subscription
                (&ldquo;Trial Period&rdquo;). A Trial Period is a temporary
                offer, may be changed or withdrawn at any time, and is limited
                to one Trial Period per Billing Account regardless of the number
                of accounts, email addresses, or payment methods you use. We do
                not guarantee availability of any Trial Period to any specific
                user.
              </p>
              <p>
                To start a Trial Period you must provide a valid payment method.
                Your payment method will not be charged during the Trial Period,
                except for any taxes or fees required by applicable law. When
                the Trial Period ends, your subscription will automatically
                convert to a paid subscription at the then-current pricing for
                the selected plan, and your payment method will be charged for
                the first billing period without further action on your part.
                Auto-renewal thereafter is governed by Section 4.3.
              </p>
              <p>
                You may cancel at any time during the Trial Period through your
                account settings. If you cancel before the Trial Period ends,
                your subscription will not convert to paid and your payment
                method will not be charged for the first billing period. Access
                to paid features ends at the earlier of the end of the Trial
                Period or the effective date of your cancellation, in accordance
                with Section 4.7.
              </p>
              <p>
                Participation in a Trial Period is not a refund trigger. No
                refund, credit, or other compensation is due if you do not
                cancel before the Trial Period ends, if you do not use the
                Service during the Trial Period, or if you are dissatisfied with
                the Service during the Trial Period. Refund eligibility after
                the Trial Period converts to paid is governed by Section 4.7 and
                applicable law. If a charge at the end of the Trial Period
                fails, Section 4.6 (Billing and Late Payment) applies.
              </p>
            </div>
          </div>

          {/* 5. User Content */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              5. User Content
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                5.1 Ownership
              </h3>
              <p>
                You retain all right, title, and interest in and to content you
                create, upload, or store using the Service (&ldquo;User
                Content&rdquo;). We do not claim any ownership rights in your
                User Content. We do not use your User Content to train
                artificial intelligence or machine learning models.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                5.2 License Grant
              </h3>
              <p>
                By using the Service&rsquo;s cloud storage and sharing features,
                you grant us a limited, worldwide, non-exclusive, royalty-free,
                sublicensable (solely to our infrastructure and service
                providers as necessary to operate the Service) license to store,
                display, transmit, and cache your User Content solely as
                necessary to provide and maintain the Service. This license
                terminates when you delete the content or close your account,
                except that a) content previously shared with other users may
                persist in those users&rsquo; accounts, and b) residual copies
                in backup or archival systems may be retained for a commercially
                reasonable period.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                5.3 Sharing and Collaboration
              </h3>
              <p>
                When you share content with other users via Spacewave&rsquo;s
                direct networking or cloud features, you are responsible for
                ensuring you have the right to share that content. We are not
                responsible for how recipients use shared content.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                5.4 Prohibited Content
              </h3>
              <p>You may not use the Service to store or transmit:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>Content that violates any applicable law</li>
                <li>Child sexual abuse material (CSAM)</li>
                <li>Malware, viruses, or other harmful code</li>
                <li>
                  Content that infringes intellectual property rights of others
                </li>
                <li>Spam or unsolicited bulk communications</li>
              </ul>
              <p>
                We have no obligation to monitor User Content or account
                activity but reserve the right to monitor account activity and
                metadata to the extent technically feasible. Because User
                Content is end-to-end encrypted, we are unable to access or
                review the plaintext content of your data. However, we may:
              </p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  Remove or disable access to encrypted data stored on our
                  infrastructure;
                </li>
                <li>
                  Suspend or terminate accounts based on metadata patterns,
                  resource consumption anomalies, or other non-content signals
                  that we reasonably believe indicate a violation of this
                  section or Section 6 (Acceptable Use);
                </li>
                <li>
                  Enforce resource consumption limits (storage, operations,
                  bandwidth) regardless of the nature or content of the data
                  stored;
                </li>
                <li>
                  Cooperate with law enforcement and respond to valid legal
                  process (such as subpoenas, court orders, or search warrants)
                  by providing account metadata, usage records, and other
                  non-content information within our possession, even where the
                  underlying User Content is encrypted and inaccessible to us;
                </li>
              </ul>
              <p>in each case without prior notice or liability.</p>
            </div>
          </div>

          {/* 6. Acceptable Use */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              6. Acceptable Use
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>You agree not to:</p>
              <ul className="list-disc space-y-1 pl-5">
                <li>
                  Use the Service as a general-purpose CDN, file hosting
                  service, or bulk data dump unrelated to active Spacewave
                  application usage
                </li>
                <li>
                  Use the Service to proxy, relay, mirror, or redistribute data
                  for non-Spacewave purposes or for the benefit of third-party
                  services
                </li>
                <li>
                  Engage in automated sync flooding, API abuse, or any pattern
                  of programmatic access designed to generate excessive write or
                  read operations disproportionate to genuine application use
                </li>
                <li>
                  Create excessive SharedObjects or collaborative sessions for
                  the purpose of consuming platform resources rather than
                  genuine collaboration
                </li>
                <li>
                  Circumvent usage limits, rate limits, access controls, or
                  resource quotas
                </li>
                <li>
                  Interfere with or disrupt the Service or its infrastructure
                </li>
                <li>
                  Attempt to gain unauthorized access to other users&rsquo;
                  accounts or data
                </li>
                <li>
                  Use the Service in any manner that could damage, disable, or
                  impair the Service
                </li>
                <li>
                  Reverse engineer, decompile, disassemble, or otherwise attempt
                  to discover the source code or underlying algorithms of the
                  Service&rsquo;s server-side components (except to the extent
                  that applicable law prohibits this restriction)
                </li>
                <li>
                  Use automated means to create accounts or access the Service
                  in a manner that exceeds reasonable use
                </li>
                <li>
                  Use the Service in violation of any applicable law or
                  regulation, including export control and sanctions laws
                </li>
                <li>
                  Access or use the Service from a country subject to
                  comprehensive U.S. economic sanctions or if you are designated
                  on any U.S. government restricted party list
                </li>
              </ul>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                6.1 Rate Limiting and Resource Quotas
              </h3>
              <p>
                We reserve the right to implement and enforce rate limiting,
                throttling, and resource quotas on any aspect of the Service,
                including but not limited to sync operations, API requests,
                SharedObject operations, WebSocket connections, and account
                registrations. These measures are designed to protect service
                quality and ensure fair access for all users. Rate limits and
                quotas may be adjusted at any time without prior notice. If your
                usage is throttled or rate-limited, we will use commercially
                reasonable efforts to notify you, but failure to provide such
                notice shall not give rise to any liability.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                6.2 SharedObject Limits
              </h3>
              <p>
                SharedObjects are a collaborative feature of the Service with
                associated infrastructure costs. We reserve the right to impose
                limits on: a) the number of concurrent SharedObjects per
                account; b) the rate of operations per SharedObject; and c) the
                number of concurrent participants per SharedObject. Current
                limits, if any, are published on the pricing page. You are
                responsible for all content shared via SharedObjects that you
                create or administer, and for ensuring that all participants in
                your SharedObjects comply with these Terms.
              </p>
            </div>
          </div>

          {/* 7. Intellectual Property */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              7. Intellectual Property
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                7.1 Company Ownership
              </h3>
              <p>
                The Service, including the platform, its proprietary server-side
                software, design, documentation, and all enhancements,
                derivatives, and improvements thereto, is the exclusive property
                of Aperture Robotics, LLC and is protected by applicable
                intellectual property laws. Except for the limited rights
                expressly granted in these Terms, we reserve all right, title,
                and interest in and to the Service, including all intellectual
                property rights therein. Any customizations, configurations, or
                modifications to the platform made by or on behalf of the
                Company remain the exclusive property of the Company. These
                Terms do not grant you any rights to our trademarks, logos, or
                branding.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                7.2 Customer Ownership
              </h3>
              <p>
                You retain all right, title, and interest in and to your User
                Content (as set forth in Section 5.1) and any applications,
                code, or other materials that you independently create, develop,
                or author using the Service (&ldquo;Customer Materials&rdquo;).
                Nothing in these Terms transfers ownership of Customer Materials
                to the Company.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                7.3 Open-Source Software
              </h3>
              <p>
                Open-source components of Spacewave are licensed under their
                respective open-source licenses, which are identified in the
                source code repositories. Nothing in these Terms restricts or
                limits your rights under any applicable open-source license. To
                the extent there is a conflict between these Terms and an
                applicable open-source license with respect to a specific
                open-source component, the open-source license shall govern with
                respect to that component.
              </p>
            </div>
          </div>

          {/* 8. Third-Party Services */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              8. Third-Party Services
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                The Service relies on third-party infrastructure providers,
                including Cloudflare (storage, compute, data relay, and bot
                management) and Stripe (payment processing). These providers are
                subject to contractual obligations consistent with these Terms.
                We remain responsible for the Service as described in these
                Terms but are not liable for the acts or omissions of
                third-party providers beyond our reasonable control. We make no
                warranty or representation regarding any third-party services,
                and your use of such third-party services may be subject to the
                terms and conditions of those third-party providers.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                8.1 Infrastructure and Data Processing
              </h3>
              <p>
                The Service uses Cloudflare&rsquo;s global edge network for
                request processing, data storage, and real-time communication.
                As a result: a) your requests may be processed at Cloudflare
                edge locations nearest to you, which may be in different
                jurisdictions; b) cached copies of your encrypted data may
                temporarily exist at multiple Cloudflare edge locations; and c)
                the availability and performance of the Service depend in part
                on the availability of Cloudflare&rsquo;s infrastructure. We do
                not currently offer user-selectable data residency regions but
                may do so in the future. All data stored on our infrastructure
                is encrypted as described in Section 5.4.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                8.2 Bot Management
              </h3>
              <p>
                The Service uses Cloudflare Turnstile for bot detection and
                abuse prevention. Turnstile may collect interaction data to
                distinguish human users from automated access. Your use of the
                Service is subject to the{' '}
                <a
                  href="https://www.cloudflare.com/turnstile-privacy-policy/"
                  className="text-brand hover:text-brand-highlight underline"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Cloudflare Turnstile Privacy Policy
                </a>
                .
              </p>
            </div>
          </div>

          {/* 9. DMCA and Copyright */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              9. DMCA and Copyright
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                We respect intellectual property rights and comply with the
                Digital Millennium Copyright Act (DMCA). If you believe content
                on Spacewave infringes your copyright, please see our{' '}
                <a
                  href="#/dmca"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  DMCA Policy
                </a>{' '}
                for designated agent contact information and takedown
                procedures.
              </p>
              <p>
                We will respond to valid DMCA takedown notices and may remove or
                disable access to infringing content. Repeat infringers may have
                their accounts terminated.
              </p>
            </div>
          </div>

          {/* 10. Privacy */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              10. Privacy
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Your use of the Service is also governed by our{' '}
                <a
                  href="#/privacy"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  Privacy Policy
                </a>
                , which is incorporated into these Terms by reference.
              </p>
            </div>
          </div>

          {/* 11. Disclaimers */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              11. Disclaimers
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                THE SERVICE IS PROVIDED &ldquo;AS IS&rdquo; AND &ldquo;AS
                AVAILABLE&rdquo; WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS
                OR IMPLIED, INCLUDING BUT NOT LIMITED TO IMPLIED WARRANTIES OF
                MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, TITLE, AND
                NON-INFRINGEMENT. NO ADVICE OR INFORMATION, WHETHER ORAL OR
                WRITTEN, OBTAINED FROM THE COMPANY OR THROUGH THE SERVICE SHALL
                CREATE ANY WARRANTY NOT EXPRESSLY STATED HEREIN.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                WE DO NOT WARRANT THAT THE SERVICE WILL BE UNINTERRUPTED,
                TIMELY, ERROR-FREE, OR SECURE. YOUR USE OF THE SERVICE IS AT
                YOUR SOLE RISK.
              </p>
              <p>
                For self-hosted and local features, data availability depends on
                connected devices. We do not guarantee availability of locally
                hosted or self-hosted content.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                THESE DISCLAIMERS APPLY TO THE MAXIMUM EXTENT PERMITTED BY
                APPLICABLE LAW.
              </p>
            </div>
          </div>

          {/* 12. Limitation of Liability */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              12. Limitation of Liability
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, APERTURE
                ROBOTICS, LLC SHALL NOT BE LIABLE FOR ANY INDIRECT, INCIDENTAL,
                SPECIAL, CONSEQUENTIAL, EXEMPLARY, OR PUNITIVE DAMAGES, OR ANY
                LOSS OF PROFITS, REVENUE, GOODWILL, DATA, OR USE, ARISING OUT OF
                OR IN CONNECTION WITH THESE TERMS, THE SERVICE, OR YOUR USE OR
                INABILITY TO USE THE SERVICE, WHETHER BASED IN TORT (INCLUDING
                NEGLIGENCE), CONTRACT, BREACH OF STATUTORY DUTY, OR OTHERWISE,
                EVEN IF THE COMPANY HAS BEEN ADVISED OF THE POSSIBILITY OF SUCH
                DAMAGES.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                OUR TOTAL CUMULATIVE LIABILITY FOR ALL CLAIMS ARISING OUT OF OR
                RELATING TO THESE TERMS OR THE SERVICE SHALL NOT EXCEED THE
                AMOUNT YOU PAID US IN THE TWELVE (12) MONTHS IMMEDIATELY
                PRECEDING THE EVENT GIVING RISE TO THE CLAIM. FOR CLARITY, IF
                YOU ARE USING ONLY THE FREE SERVICES, YOU HAVE PAID US NOTHING
                AND OUR LIABILITY IS ZERO. WHERE A MORE SPECIFIC LIABILITY
                EXCLUSION OR CAP IS SET FORTH ELSEWHERE IN THESE TERMS
                (INCLUDING SECTION 2.1 FOR BETA SERVICES AND SECTION 4.1 FOR
                FREE SERVICES), THE MORE PROTECTIVE PROVISION SHALL CONTROL.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                THESE LIMITATIONS AND EXCLUSIONS APPLY TO THE MAXIMUM EXTENT
                PERMITTED BY APPLICABLE LAW.
              </p>
            </div>
          </div>

          {/* 13. Indemnification */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              13. Indemnification
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                You agree to indemnify, defend, and hold harmless Aperture
                Robotics, LLC and its members, managers, employees, and agents
                from and against any and all claims, demands, losses, damages,
                liabilities, costs, and expenses (including reasonable
                attorneys&rsquo; fees and court costs) arising out of or
                relating to a) your use of the Service, b) your User Content, c)
                your violation of these Terms, or d) your violation of any
                applicable law or the rights of any third party. We will provide
                you with prompt written notice of any such claim and reasonably
                cooperate with your defense at your sole cost and expense. We
                reserve the right to assume the exclusive defense and control of
                any matter subject to indemnification by you, in which case you
                agree to cooperate with our defense of such claim.
              </p>
            </div>
          </div>

          {/* 14. Dispute Resolution */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              14. Dispute Resolution
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                14.1 Good-Faith Negotiation
              </h3>
              <p>
                Before initiating any arbitration or legal proceeding, the
                parties shall use commercially reasonable efforts to resolve any
                dispute, claim, or disagreement arising out of or relating to
                these Terms or the Service through good-faith negotiation. The
                complaining party shall send written notice to the other party
                describing the dispute in reasonable detail. The parties shall
                have thirty (30) days from the date of such notice to attempt to
                resolve the dispute. Good-faith negotiation is a precondition to
                initiating arbitration under Section 14.2.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                14.2 Arbitration
              </h3>
              <p>
                If a dispute is not resolved through good-faith negotiation
                under Section 14.1, it shall be finally settled by binding
                arbitration administered by the American Arbitration Association
                (AAA) under its Consumer Arbitration Rules in effect at the time
                the arbitration is initiated. Arbitration will be conducted in
                Wilmington, Delaware, or, at the election of the claimant, by
                telephone or videoconference as permitted by the AAA Rules.
              </p>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                BY AGREEING TO THESE TERMS, EACH PARTY WAIVES ANY CONSTITUTIONAL
                AND STATUTORY RIGHTS TO GO TO COURT AND HAVE A TRIAL BEFORE A
                JUDGE OR JURY. ALL CLAIMS AND DISPUTES WITHIN THE SCOPE OF THIS
                ARBITRATION AGREEMENT MUST BE ARBITRATED ON AN INDIVIDUAL BASIS
                AND NOT ON A CLASS BASIS. THE ARBITRATOR&rsquo;S DECISION SHALL
                BE FINAL AND BINDING AND MAY BE ENTERED AS A JUDGMENT IN ANY
                COURT OF COMPETENT JURISDICTION.
              </p>
              <p>
                To the extent permitted by applicable law and the AAA Rules, all
                aspects of the arbitration proceeding, including any ruling,
                decision, or award by the arbitrator, shall be strictly
                confidential. Neither party shall disclose the existence,
                content, or results of any arbitration without the prior written
                consent of the other party, except as may be required by
                applicable law or to enforce the arbitration award.
              </p>
              <p>
                Notwithstanding the foregoing, either party may bring an
                individual action in small claims court for disputes within the
                jurisdictional limits of that court.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                14.3 Opt-Out
              </h3>
              <p>
                You may opt out of the arbitration agreement by sending written
                notice to legal@aperture.us within thirty (30) days of the date
                you first agree to these Terms. Your notice must include your
                name, the email address associated with your account, and a
                clear statement that you wish to opt out of the arbitration
                agreement. If you opt out, disputes will be resolved in the
                state or federal courts located in the State of Delaware, and
                you irrevocably submit to the exclusive jurisdiction of such
                courts.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                14.4 Class Action Waiver
              </h3>
              <p className="text-foreground-alt/80 text-xs leading-relaxed font-medium tracking-wide uppercase">
                YOU AGREE TO RESOLVE ALL DISPUTES INDIVIDUALLY AND WAIVE ANY
                RIGHT TO PARTICIPATE IN A CLASS ACTION, CLASS ARBITRATION,
                CONSOLIDATED ACTION, OR REPRESENTATIVE PROCEEDING OF ANY KIND.
              </p>
            </div>
          </div>

          {/* 15. Governing Law */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              15. Governing Law
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                These Terms shall be governed by and construed in accordance
                with the laws of the State of Delaware, without regard to its
                conflict of law provisions. The United Nations Convention on
                Contracts for the International Sale of Goods does not apply to
                these Terms.
              </p>
            </div>
          </div>

          {/* 16. Force Majeure */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              16. Force Majeure
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Neither party shall be liable for any delay or failure to
                perform its obligations under these Terms (other than payment
                obligations) to the extent caused by circumstances beyond its
                reasonable control, including acts of God, natural disasters,
                epidemics, pandemics, fire, flood, war, terrorism, riots, labor
                disputes, government action, power outages, telecommunications
                failures, Internet or third-party hosting service failures, or
                denial of service attacks, provided that the affected party uses
                commercially reasonable efforts to mitigate the effects of such
                event and provides prompt notice to the other party. If a force
                majeure event prevents the Service from materially operating for
                sixty (60) or more consecutive days, either party may terminate
                the affected subscription upon written notice. In such case, we
                will provide a pro-rata refund of any prepaid fees for the
                remainder of the then-current billing period.
              </p>
            </div>
          </div>

          {/* 17. Export Control */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              17. Export Control
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                The Service is subject to United States export control laws and
                regulations, including the Export Administration Regulations
                (EAR) and sanctions programs administered by the Office of
                Foreign Assets Control (OFAC). You represent and warrant that
                you are not a) located in, or a resident or national of, any
                country subject to comprehensive U.S. economic sanctions; b)
                designated on any U.S. government restricted party list,
                including the Specially Designated Nationals and Blocked Persons
                List (SDN List), the Entity List, or the Denied Persons List;
                nor c) fifty percent (50%) or more owned by any party designated
                on any such list. You shall not export, re-export, or transfer
                the Service or any related technical data in violation of any
                applicable export control law or regulation.
              </p>
            </div>
          </div>

          {/* 18. General */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              18. General
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <h3 className="text-foreground mt-1 mb-2 text-base font-semibold">
                18.1 Modifications to Terms
              </h3>
              <p>
                We reserve the right to modify these Terms at any time. If we
                make material changes, we will notify you by sending an email to
                the address associated with your account when the updated Terms
                take effect. Your continued use of the Service after the
                effective date of the modified Terms constitutes your acceptance
                of the changes. If you do not agree to the modified Terms, you
                must stop using the Service and may cancel your subscription in
                accordance with Section 4.7.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.2 Notices
              </h3>
              <p>
                All notices under these Terms shall be in writing and delivered
                by email. Notices to the Company shall be sent to
                legal@aperture.us. Notices to you shall be sent to the email
                address associated with your account. Notices shall be deemed
                given upon confirmed delivery. You are responsible for keeping
                your account email address current.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.3 Severability
              </h3>
              <p>
                If any provision of these Terms is held to be invalid or
                unenforceable, such provision shall be modified to the minimum
                extent necessary to make it valid and enforceable, and the
                remaining provisions shall remain in full force and effect.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.4 Waiver
              </h3>
              <p>
                No waiver of any term or condition of these Terms shall be
                deemed a further or continuing waiver of such term or condition
                or any other term or condition. Our failure to assert any right
                or provision under these Terms shall not constitute a waiver of
                such right or provision.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.5 Entire Agreement
              </h3>
              <p>
                These Terms constitute the entire agreement between you and
                Aperture Robotics, LLC regarding the Service and supersede all
                prior or contemporaneous agreements, representations, and
                understandings, whether written or oral.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.6 Assignment
              </h3>
              <p>
                You may not assign or transfer these Terms, in whole or in part,
                without our prior written consent (not to be unreasonably
                withheld), and any attempted assignment without such consent
                shall be null and void. We may assign these Terms without your
                consent upon notice to an affiliate or in connection with a
                merger, acquisition, reorganization, or sale of all or
                substantially all of our assets.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.7 Independent Contractors
              </h3>
              <p>
                The parties are independent contractors. Nothing in these Terms
                creates a partnership, joint venture, employment, or agency
                relationship between the parties.
              </p>

              <h3 className="text-foreground mt-4 mb-2 text-base font-semibold">
                18.8 No Third-Party Beneficiaries
              </h3>
              <p>
                These Terms do not confer any rights or remedies upon any third
                party.
              </p>
            </div>
          </div>

          {/* 19. Contact */}
          <div className="border-foreground/8 bg-background-card/50 rounded-lg border p-6 backdrop-blur-sm @lg:p-8">
            <h2 className="text-foreground mb-4 text-lg font-semibold">
              19. Contact
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <div className="bg-background/50 border-foreground/8 rounded border p-4 font-mono text-xs leading-relaxed">
                Aperture Robotics, LLC
                <br />
                Email:{' '}
                <a
                  href="mailto:legal@aperture.us"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  legal@aperture.us
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
