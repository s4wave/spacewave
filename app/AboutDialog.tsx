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
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 60%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    position: 'absolute',
    textAlign: 'right',
    right: `calc(${themeLayout['--spacing']} * 6)`,
    top: `calc(${themeLayout['--spacing']} * 5)`,
  },
  rule2: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    flexDirection: 'column',
    paddingBottom: `calc(${themeLayout['--spacing']} * 2)`,
    paddingTop: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule3: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    fontSize: 'var(--text-xl)',
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: 'var(--tw-leading, var(--text-xl--line-height))',
  },
  rule4: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 40%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: themeLayout['--spacing'],
  },
  rule5: {
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    textAlign: 'center',
    paddingBottom: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule6: {
    '--tw-leading': 'var(--leading-relaxed)',
    paddingBlock: `calc(${themeLayout['--spacing']} * 2)`,
    color: themeColors['--color-foreground-alt'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: 'var(--leading-relaxed)',
  },
  rule7: {
    borderColor: themeColors['--color-border'],
    gap: `calc(${themeLayout['--spacing']} * 4)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'center',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule8: {
    color: {
      default: themeColors['--color-foreground-alt'],
      '@media (hover: hover)': {
        default: null,
        ':hover': themeColors['--color-foreground'],
      },
    },
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration:
      'var(--tw-duration, var(--default-transition-duration))',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
  },
  rule9: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 30%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule10: {
    borderColor: themeColors['--color-border'],
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 6)`,
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-muted']} 30%, transparent)`,
    cursor: 'pointer',
    display: 'block',
    textAlign: 'center',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule11: {
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 20)`,
    marginBottom: `calc(${themeLayout['--spacing']} * 4)`,
    width: `calc(${themeLayout['--spacing']} * 20)`,
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
    margin: '-1px',
    padding: 0,
    borderWidth: 0,
    overflow: 'hidden',
    clipPath: 'inset(50%)',
    position: 'absolute',
    whiteSpace: 'nowrap',
    height: '1px',
    width: '1px',
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 1 * var(--tw-space-y-reverse))`,
    },
  },
})
