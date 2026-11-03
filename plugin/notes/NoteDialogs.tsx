import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  fonts as themeFonts,
  colors as themeColors,
  radii as themeRadii,
  utilityState as themeUtilityState,
  motion as themeMotion,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

const stylexStyles = stylex.create({
  rule1: {
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule2: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-border'],
      ':focus': themeColors['--color-brand'],
    },
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule3: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
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
  rule4: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
  rule5: {
    display: 'flex',
    flexDirection: 'column',
    gap: themeLayout['--spacing'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    color: themeColors['--color-foreground'],
  },
  rule6: {
    height: `calc(${themeLayout['--spacing']} * 8)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: themeColors['--color-border'],
      ':focus': themeColors['--color-brand'],
    },
    backgroundColor: themeColors['--color-background'],
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    fontFamily: themeFonts['--font-mono'],
    WebkitFontSmoothing: 'subpixel-antialiased',
    MozOsxFontSmoothing: 'auto',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    color: themeColors['--color-foreground'],
    '--tw-outline-style': 'none',
    outlineStyle: 'none',
  },
  rule7: {
    height: `calc(${themeLayout['--spacing']} * 7)`,
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      },
    },
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    fontWeight: themeFonts['--font-weight-medium'],
    color: themeColors['--color-foreground'],
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    transitionDuration: '.15s',
    '--tw-duration': '.15s',
  },
})

const dialogStyles = stylex.create({
  footer: { marginTop: `calc(${themeLayout['--spacing']} * 4)` },
})

function getFormString(form: FormData, name: string): string {
  const value = form.get(name)
  return typeof value === 'string' ? value.trim() : ''
}

interface TextInputDialogProps {
  open: boolean
  title: string
  description?: string
  label: string
  defaultValue?: string
  placeholder?: string
  confirmLabel: string
  requireValue?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (value: string) => void
}

export function TextInputDialog({
  open,
  title,
  description,
  label,
  defaultValue = '',
  placeholder,
  confirmLabel,
  requireValue,
  onOpenChange,
  onConfirm,
}: TextInputDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* eslint-disable-next-line react-doctor/no-prevent-default -- plugin dialogs submit to in-memory callbacks, not no-JS document actions. */}
        <form
          onSubmit={(event) => {
            // Read the submitted value before invoking the in-memory confirmation.
            event.preventDefault()
            const form = new FormData(event.currentTarget)
            const value = getFormString(form, 'value')
            if (requireValue && !value) return
            onConfirm(value)
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : null}
          </DialogHeader>
          <label sx={stylexStyles.rule1}>
            {label}
            <input
              key={`${open ? 'open' : 'closed'}:${defaultValue}`}
              name="value"
              defaultValue={defaultValue}
              placeholder={placeholder}
              required={requireValue}
              sx={stylexStyles.rule2}
            />
          </label>
          <DialogFooter sx={dialogStyles.footer}>
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              sx={stylexStyles.rule3}
            >
              Cancel
            </button>
            <button type="submit" sx={stylexStyles.rule4}>
              {confirmLabel}
            </button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface SourceInputDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (source: { name: string; ref: string }) => void
}

export function SourceInputDialog({
  open,
  onOpenChange,
  onConfirm,
}: SourceInputDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* eslint-disable-next-line react-doctor/no-prevent-default -- plugin dialogs submit to in-memory callbacks, not no-JS document actions. */}
        <form
          onSubmit={(event) => {
            // Read the source fields before invoking the in-memory confirmation.
            event.preventDefault()
            const form = new FormData(event.currentTarget)
            const name = getFormString(form, 'name')
            const ref = getFormString(form, 'ref')
            if (!ref) return
            onConfirm({ name, ref })
          }}
        >
          <DialogHeader>
            <DialogTitle>Add source</DialogTitle>
            <DialogDescription>
              Connect this notebook to a folder or document source.
            </DialogDescription>
          </DialogHeader>
          <div sx={stylexStyles2.rule1}>
            <label sx={[nativeChildSpacing.s3, stylexStyles.rule5]}>
              Source name
              <input
                key={open ? 'source-name-open' : 'source-name-closed'}
                name="name"
                placeholder="Docs"
                sx={stylexStyles.rule2}
              />
            </label>
            <label sx={[nativeChildSpacing.s3, stylexStyles.rule5]}>
              Source ref
              <input
                key={open ? 'source-ref-open' : 'source-ref-closed'}
                name="ref"
                placeholder="object-key/-/path"
                required
                sx={stylexStyles.rule6}
              />
            </label>
          </div>
          <DialogFooter sx={dialogStyles.footer}>
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              sx={stylexStyles.rule3}
            >
              Cancel
            </button>
            <button type="submit" sx={stylexStyles.rule4}>
              Add source
            </button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface ConfirmActionDialogProps {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export function ConfirmActionDialog({
  open,
  title,
  description,
  confirmLabel,
  onOpenChange,
  onConfirm,
}: ConfirmActionDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            sx={stylexStyles.rule3}
          >
            Cancel
          </button>
          <button type="button" onClick={onConfirm} sx={stylexStyles.rule7}>
            {confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// nativeChildSpacing preserves the original margins at each rendered child.
const nativeChildSpacing = stylex.create({
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
