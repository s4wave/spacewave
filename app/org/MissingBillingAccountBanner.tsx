import { LuArrowRight, LuTriangleAlert } from '@s4wave/web/images/icons.js'

import * as stylex from '@stylexjs/stylex'
import {
  layout as themeLayout,
  colors as themeColors,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'
import { groupMarker } from '../../web/style/relations.stylex.js'

import { groupMarker as groupMarker2 } from '../../web/style/relations.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    display: 'flex',
    flexShrink: 0,
    alignItems: 'center',
    gap: themeLayout['--spacing'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 1.5)`,
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

const stylexStyles = stylex.create({
  rule1: {
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    flexShrink: 0,
    color: themeColors['--color-warning'],
  },
  rule2: {
    width: `calc(${themeLayout['--spacing']} * 3)`,
    height: `calc(${themeLayout['--spacing']} * 3)`,
    flexShrink: 0,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        [stylex.when.ancestor(':is(:hover)', groupMarker)]:
          themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
})

// MissingBillingAccountBanner prompts when an org has no billing account.
// Owners get a "Configure billing" CTA that opens the org details billing
// section so they can pick or detach a managed BA. Non-owners see a
// read-only message asking them to contact an org admin.
export function MissingBillingAccountBanner(props: {
  isOwner: boolean
  onConfigureBilling: () => void
}) {
  const message =
    'This organization has no billing account. Spaces in this org cannot be created until billing is configured.'

  if (!props.isOwner) {
    return (
      <div className="border-warning/20 bg-warning/5 relative z-10 flex w-full items-center border-b">
        <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
          <LuTriangleAlert sx={stylexStyles.rule1} />
          <div className="min-w-0">
            <p className="text-foreground/80 text-xs font-medium">{message}</p>
            <p className="text-foreground-alt/60 mt-0.5 text-xs">
              Ask an organization admin to assign a billing account.
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={props.onConfigureBilling}
      className="border-warning/20 bg-warning/5 hover:bg-warning/10 relative z-10 flex w-full items-center border-b text-left transition-colors"
    >
      <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
        <LuTriangleAlert sx={stylexStyles.rule1} />
        <div className="min-w-0">
          <p className="text-foreground/80 text-xs font-medium">{message}</p>
          <p className="text-foreground-alt/60 mt-0.5 text-xs">
            Assign one of your managed billing accounts to keep cloud features
            available.
          </p>
        </div>
      </div>
      <div sx={[stylexStyles2.rule1, groupMarker2]}>
        <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
          Configure billing
        </span>
        <LuArrowRight sx={stylexStyles.rule2} />
      </div>
    </button>
  )
}
