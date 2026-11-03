import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'
import { DISCORD_INVITE_URL, GITHUB_REPO_URL } from '@s4wave/app/github.js'
import { useAppBuildInfo } from '@s4wave/app/build-info.js'
import { AppLogo } from '@s4wave/web/images/AppLogo.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import * as stylex from '@stylexjs/stylex'

import { layout as themeLayout } from '../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../web/style/tokens.stylex.js'

const stylexStyles3 = stylex.create({
  rule1: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles2 = stylex.create({
  rule1: {
    position: 'absolute',
    top: `calc(${themeLayout['--spacing']} * 5)`,
    right: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'right',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
  },
  rule2: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 8)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
  },
  rule3: {
    fontSize: 'var(--text-xl)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
    '--tw-font-weight': 'var(--font-weight-semibold)',
    fontWeight: 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    letterSpacing: 'var(--tracking-tight)',
  },
  rule4: {
    marginTop: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
  },
  rule5: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
    textAlign: 'center',
  },
  rule6: {
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
    '--tw-leading': 'var(--leading-relaxed)',
    color: themeColors['--color-foreground-alt'],
  },
  rule7: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: themeColors['--color-border'],
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
  },
  rule8: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
  },
  rule9: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
  },
  rule10: {
    display: 'block',
    cursor: 'pointer',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
    borderColor: themeColors['--color-border'],
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 30%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    textAlign: 'center',
  },
  rule11: {
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 20)`,
    height: `calc(${themeLayout['--spacing']} * 20)`,
  },
})

const dialogStyles = stylex.create({
  content: {
    overflow: 'hidden',
    maxWidth: {
      default: 'calc(100% - 2rem)',
      '@media (min-width: 40rem)': 'var(--container-sm)',
    },
  },
  accessibleOnly: {
    clipPath: 'inset(50%)',
    whiteSpace: 'nowrap',
    borderWidth: 0,
    width: '1px',
    height: '1px',
    margin: '-1px',
    padding: 0,
    position: 'absolute',
    overflow: 'hidden',
  },
})

interface AboutDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

// AboutDialog renders a Photoshop-style about modal with app branding.
export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
  const buildInfo = useAppBuildInfo()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        variant="compact"
        sx={dialogStyles.content}
      >
        <DialogTitle sx={dialogStyles.accessibleOnly}>
          About Spacewave
        </DialogTitle>
        <DialogDescription sx={dialogStyles.accessibleOnly}>
          Free, open-source, local-first platform for file sync, communication,
          device management, and community plugins.
        </DialogDescription>
        <p sx={stylexStyles2.rule1}>{buildInfo.version}</p>

        {/* Header with logo */}
        <div sx={stylexStyles2.rule2}>
          <AppLogo sx={stylexStyles.rule1} alt="Spacewave" />
          <h2 sx={stylexStyles2.rule3}>Spacewave</h2>
          {buildInfo.runtimeLabel && (
            <p sx={stylexStyles2.rule4}>{buildInfo.runtimeLabel}</p>
          )}
        </div>

        {/* Description */}
        <div sx={stylexStyles2.rule5}>
          <p sx={stylexStyles2.rule6}>
            Free, open-source, local-first platform for file sync,
            communication, device management, and more with community plugins.
            End-to-end encrypted. Runs directly in the web browser.
          </p>
        </div>

        {/* Links */}
        <div sx={stylexStyles2.rule7}>
          <a
            href={GITHUB_REPO_URL}
            target="_blank"
            rel="noopener noreferrer"
            sx={stylexStyles2.rule8}
          >
            GitHub
          </a>
          <span sx={stylexStyles2.rule9}>|</span>
          <a
            href={DISCORD_INVITE_URL}
            target="_blank"
            rel="noopener noreferrer"
            sx={stylexStyles2.rule8}
          >
            Discord
          </a>
          <span sx={stylexStyles2.rule9}>|</span>
          <a href="#/docs" sx={stylexStyles2.rule8}>
            Documentation
          </a>
        </div>

        {/* Footer */}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={() =>
                window.open('https://cjs.zip', '_blank', 'noopener,noreferrer')
              }
              sx={stylexStyles2.rule10}
            >
              <p sx={stylexStyles2.rule11}>Created by Christian Stewart</p>
            </button>
          </TooltipTrigger>
          <TooltipContent side="bottom">
            <div sx={stylexStyles3.rule1}>
              <p sx={nativeChildSpacing.s1}>
                Dedicated to my father, Jim, and my cat, May.
              </p>
              <p sx={nativeChildSpacing.s1}>
                May they rest in peace ❤ ~ CJS, 2024
              </p>
            </div>
          </TooltipContent>
        </Tooltip>
      </DialogContent>
    </Dialog>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
  s1: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
  },
})
