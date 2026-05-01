import { LuShield } from 'react-icons/lu'
import { LegalPageLayout } from './LegalPageLayout.js'

export const metadata = {
  title: 'DMCA Policy - Spacewave',
  description:
    'Find Spacewave DMCA policy details, designated agent contact information, takedown notice requirements, and counter-notification steps.',
  canonicalPath: '/dmca',
  ogImage: 'https://cdn.spacewave.app/og-default.png',
}

// DMCA renders the DMCA compliance page.
export function DMCA() {
  return (
    <LegalPageLayout
      icon={<LuShield className="h-10 w-10" />}
      title="DMCA Policy"
    >
      <section className="relative z-10 mx-auto w-full max-w-4xl px-4 pb-16 @lg:px-8">
        <div className="space-y-10">
          {/* Designated Agent */}
          <div>
            <h2 className="text-foreground mb-4 text-xl font-semibold">
              1. Designated Agent
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Aperture Robotics, LLC. has registered a designated agent with
                the U.S. Copyright Office in accordance with 17 U.S.C. &sect;
                512(c)(2) of the Digital Millennium Copyright Act.
              </p>
              <p>Registration number: DMCA-1070193</p>
              <div className="bg-background/50 border-border/50 rounded-lg border p-4">
                <p className="text-foreground mb-2 text-sm font-medium">
                  Copyright Compliance Manager
                </p>
                <p>Aperture Robotics, LLC.</p>
                <p>PO Box 692</p>
                <p>Mercer Island, WA 98040</p>
                <p className="mt-2">
                  Email:{' '}
                  <a
                    href="mailto:dmca@aperture.us"
                    className="text-brand hover:text-brand-highlight underline"
                  >
                    dmca@aperture.us
                  </a>{' '}
                  (preferred)
                </p>
                <p>Phone: (818) 308-4570</p>
              </div>
            </div>
          </div>

          {/* Filing a Notice */}
          <div>
            <h2 className="text-foreground mb-4 text-xl font-semibold">
              2. Filing a DMCA Takedown Notice
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                If you believe that content hosted on Spacewave infringes your
                copyright, you may submit a written notification to our
                designated agent. Your notice must include:
              </p>
              <ol className="list-decimal space-y-2 pl-6">
                <li>
                  A physical or electronic signature of the copyright owner or a
                  person authorized to act on their behalf.
                </li>
                <li>
                  Identification of the copyrighted work claimed to have been
                  infringed.
                </li>
                <li>
                  Identification of the material that is claimed to be
                  infringing, with information reasonably sufficient to permit
                  us to locate the material.
                </li>
                <li>
                  Your contact information, including address, telephone number,
                  and email address.
                </li>
                <li>
                  A statement that you have a good faith belief that use of the
                  material in the manner complained of is not authorized by the
                  copyright owner, its agent, or the law.
                </li>
                <li>
                  A statement, made under penalty of perjury, that the
                  information in the notification is accurate and that you are
                  authorized to act on behalf of the copyright owner.
                </li>
              </ol>
              <p>
                Send your notice by email to{' '}
                <a
                  href="mailto:dmca@aperture.us"
                  className="text-brand hover:text-brand-highlight underline"
                >
                  dmca@aperture.us
                </a>{' '}
                or by mail to the designated agent address above.
              </p>
            </div>
          </div>

          {/* Counter-Notification */}
          <div>
            <h2 className="text-foreground mb-4 text-xl font-semibold">
              3. Counter-Notification
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                If you believe that material you posted was removed or disabled
                as a result of mistake or misidentification, you may file a
                counter-notification with our designated agent. Your
                counter-notification must include:
              </p>
              <ol className="list-decimal space-y-2 pl-6">
                <li>Your physical or electronic signature.</li>
                <li>
                  Identification of the material that has been removed or to
                  which access has been disabled, and the location at which the
                  material appeared before it was removed or disabled.
                </li>
                <li>
                  A statement under penalty of perjury that you have a good
                  faith belief that the material was removed or disabled as a
                  result of mistake or misidentification.
                </li>
                <li>
                  Your name, address, and telephone number, and a statement that
                  you consent to the jurisdiction of the Federal District Court
                  for the judicial district in which your address is located, or
                  if your address is outside the United States, for any judicial
                  district in which Aperture Robotics, LLC. may be found, and
                  that you will accept service of process from the person who
                  provided the original notification or an agent of such person.
                </li>
              </ol>
              <p>
                Upon receipt of a valid counter-notification, we will forward it
                to the complaining party. If the complaining party does not file
                a court action seeking to restrain the allegedly infringing
                activity within ten (10) business days, we will restore the
                removed material.
              </p>
            </div>
          </div>

          {/* Repeat Infringers */}
          <div>
            <h2 className="text-foreground mb-4 text-xl font-semibold">
              4. Repeat Infringers
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                In accordance with the DMCA and other applicable law, we have
                adopted a policy of terminating, in appropriate circumstances
                and at our sole discretion, accounts of users who are deemed to
                be repeat infringers. We may also limit access to the Service or
                terminate the accounts of any users who infringe the
                intellectual property rights of others, whether or not there is
                any repeat infringement.
              </p>
            </div>
          </div>

          {/* Good Faith */}
          <div>
            <h2 className="text-foreground mb-4 text-xl font-semibold">
              5. Misrepresentation Warning
            </h2>
            <div className="text-foreground-alt space-y-3 text-sm leading-relaxed">
              <p>
                Please be aware that under 17 U.S.C. &sect; 512(f), any person
                who knowingly materially misrepresents that material is
                infringing, or that material was removed or disabled by mistake
                or misidentification, may be subject to liability for damages,
                including costs and attorneys&rsquo; fees.
              </p>
            </div>
          </div>
        </div>
      </section>
    </LegalPageLayout>
  )
}
