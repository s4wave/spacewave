import { LuShield } from '@s4wave/web/images/icons.js'
import { LegalPageLayout } from './LegalPageLayout.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  fonts as themeFonts,
} from '../../web/style/tokens.stylex.js'

const stylexStyles4 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
})

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule2: {
    listStyleType: 'decimal',
    paddingLeft: `calc(${themeLayout['--spacing']} * 6)`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'relative',
    zIndex: 10,
    marginInline: 'auto',
    width: '100%',
    maxWidth: 'var(--container-4xl)',
    paddingInline: {
      default: `calc(${themeLayout['--spacing']} * 4)`,
      '@container (min-width: 32rem)': `calc(${themeLayout['--spacing']} * 8)`,
    },
    paddingBottom: `calc(${themeLayout['--spacing']} * 16)`,
  },
  rule2: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    color: themeColors['--color-foreground'],
  },
  rule3: {
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: `color-mix(in oklab, ${themeColors['--color-border']} 50%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background']} 50%, transparent)`,
    padding: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule4: {
    marginBottom: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
  },
  rule5: { marginTop: `calc(${themeLayout['--spacing']} * 2)` },
  rule6: {
    color: {
      default: themeColors['--color-brand'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-brand-highlight'],
      },
    },
    textDecorationLine: 'underline',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 10)`,
    height: `calc(${themeLayout['--spacing']} * 10)`,
  },
})

export const metadata = {
  title: 'DMCA Policy - Spacewave',
  description:
    'Find Spacewave DMCA policy details, designated agent contact information, takedown notice requirements, and counter-notification steps.',
  canonicalPath: '/dmca',
}

// DMCA renders the DMCA compliance page.
export function DMCA() {
  return (
    <LegalPageLayout
      icon={<LuShield sx={stylexStyles.rule1} />}
      title="DMCA Policy"
    >
      <section sx={stylexStyles2.rule1}>
        <div sx={false}>
          {/* Designated Agent */}
          <div sx={nativeChildSpacing.s10}>
            <h2 sx={stylexStyles2.rule2}>1. Designated Agent</h2>
            <div sx={stylexStyles3.rule1}>
              <p sx={nativeChildSpacing.s3}>
                Aperture Robotics, LLC. has registered a designated agent with
                the U.S. Copyright Office in accordance with 17 U.S.C. &sect;
                512(c)(2) of the Digital Millennium Copyright Act.
              </p>
              <p sx={nativeChildSpacing.s3}>
                Registration number: DMCA-1070193
              </p>
              <div sx={[nativeChildSpacing.s3, stylexStyles2.rule3]}>
                <p sx={stylexStyles2.rule4}>
                  Christian Stewart, Designated Agent
                </p>
                <p>Aperture Robotics, LLC.</p>
                <p>PO Box 692</p>
                <p>Mercer Island, WA 98040</p>
                <p sx={stylexStyles2.rule5}>
                  Email:{' '}
                  <a href="mailto:dmca@aperture.us" sx={stylexStyles2.rule6}>
                    dmca@aperture.us
                  </a>{' '}
                  (preferred)
                </p>
                <p>Phone: (818) 308-4570</p>
              </div>
            </div>
          </div>

          {/* Filing a Notice */}
          <div sx={nativeChildSpacing.s10}>
            <h2 sx={stylexStyles2.rule2}>2. Filing a DMCA Takedown Notice</h2>
            <div sx={stylexStyles4.rule1}>
              <p sx={nativeChildSpacingNext.s3}>
                If you believe that content hosted on Spacewave infringes your
                copyright, you may submit a written notification to our
                designated agent. Your notice must include:
              </p>
              <ol sx={[nativeChildSpacingNext.s3, stylexStyles3.rule2]}>
                <li sx={nativeChildSpacing.s2}>
                  A physical or electronic signature of the copyright owner or a
                  person authorized to act on their behalf.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  Identification of the copyrighted work claimed to have been
                  infringed.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  Identification of the material that is claimed to be
                  infringing, with information reasonably sufficient to permit
                  us to locate the material.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  Your contact information, including address, telephone number,
                  and email address.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  A statement that you have a good faith belief that use of the
                  material in the manner complained of is not authorized by the
                  copyright owner, its agent, or the law.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  A statement, made under penalty of perjury, that the
                  information in the notification is accurate and that you are
                  authorized to act on behalf of the copyright owner.
                </li>
              </ol>
              <p sx={nativeChildSpacingNext.s3}>
                Send your notice by email to{' '}
                <a href="mailto:dmca@aperture.us" sx={stylexStyles2.rule6}>
                  dmca@aperture.us
                </a>{' '}
                or by mail to the designated agent address above.
              </p>
            </div>
          </div>

          {/* Counter-Notification */}
          <div sx={nativeChildSpacing.s10}>
            <h2 sx={stylexStyles2.rule2}>3. Counter-Notification</h2>
            <div sx={stylexStyles4.rule1}>
              <p sx={nativeChildSpacingNext.s3}>
                If you believe that material you posted was removed or disabled
                as a result of mistake or misidentification, you may file a
                counter-notification with our designated agent. Your
                counter-notification must include:
              </p>
              <ol sx={[nativeChildSpacingNext.s3, stylexStyles3.rule2]}>
                <li sx={nativeChildSpacing.s2}>
                  Your physical or electronic signature.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  Identification of the material that has been removed or to
                  which access has been disabled, and the location at which the
                  material appeared before it was removed or disabled.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  A statement under penalty of perjury that you have a good
                  faith belief that the material was removed or disabled as a
                  result of mistake or misidentification.
                </li>
                <li sx={nativeChildSpacing.s2}>
                  Your name, address, and telephone number, and a statement that
                  you consent to the jurisdiction of the Federal District Court
                  for the judicial district in which your address is located, or
                  if your address is outside the United States, for any judicial
                  district in which Aperture Robotics, LLC. may be found, and
                  that you will accept service of process from the person who
                  provided the original notification or an agent of such person.
                </li>
              </ol>
              <p sx={nativeChildSpacingNext.s3}>
                We promptly forward a valid counter-notification to the
                complaining party and inform them that we intend to restore the
                material after ten business days. We restore it no earlier than
                ten and no later than fourteen business days after we receive
                the valid counter-notification, unless our designated agent
                receives notice of a court action seeking an order restraining
                the subscriber from the alleged infringement.
              </p>
              <p sx={nativeChildSpacingNext.s3}>
                Our operational calendar uses America/Los_Angeles dates and
                excludes weekends and observed U.S. federal holidays. Later
                administrative entry, review, or forwarding does not restart the
                original receipt period. Other unresolved notices may keep the
                material disabled. A withdrawal or acknowledged mistake may be
                resolved sooner.
              </p>
            </div>
          </div>

          {/* Repeat Infringers */}
          <div sx={nativeChildSpacing.s10}>
            <h2 sx={stylexStyles2.rule2}>4. Repeat Infringers</h2>
            <div sx={stylexStyles3.rule1}>
              <p sx={nativeChildSpacing.s3}>
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
          <div sx={nativeChildSpacing.s10}>
            <h2 sx={stylexStyles2.rule2}>5. Misrepresentation Warning</h2>
            <div sx={stylexStyles3.rule1}>
              <p sx={nativeChildSpacing.s3}>
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

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s10: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 10 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 10 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
  s2: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 2 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacingNext = stylex.create({
  s3: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
