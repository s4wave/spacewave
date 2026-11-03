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
    gap: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 4)`,
  },
  rule2: {
    '--tw-outline-style': 'none',
    borderColor: {
      default: themeColors['--color-border'],
      ':focus': themeColors['--color-brand'],
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background'],
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule3: {
    borderRadius: themeRadii['--radius-md'],
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
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
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule4: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-brand']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-brand']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
  },
  rule5: {
    gap: themeLayout['--spacing'],
    color: themeColors['--color-foreground'],
    display: 'flex',
    flexDirection: 'column',
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
  rule6: {
    '--tw-outline-style': 'none',
    MozOsxFontSmoothing: 'auto',
    WebkitFontSmoothing: 'subpixel-antialiased',
    borderColor: {
      default: themeColors['--color-border'],
      ':focus': themeColors['--color-brand'],
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 2)`,
    backgroundColor: themeColors['--color-background'],
    color: themeColors['--color-foreground'],
    fontFamily: themeFonts['--font-mono'],
    fontSize: themeFonts['--text-sm'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    outlineStyle: 'none',
    textRendering: 'auto',
    textShadow: '0 0 0.25px currentColor',
    height: `calc(${themeLayout['--spacing']} * 8)`,
  },
  rule7: {
    '--tw-duration': '.15s',
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    borderColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 30%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 50%, transparent)`,
      },
    },
    borderRadius: themeRadii['--radius-md'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    paddingInline: `calc(${themeLayout['--spacing']} * 3)`,
    backgroundColor: {
      default: `color-mix(in oklab, ${themeColors['--color-destructive']} 10%, transparent)`,
      '@media (hover: hover)': {
        default: null,
        ':hover': `color-mix(in oklab, ${themeColors['--color-destructive']} 15%, transparent)`,
      },
    },
    color: themeColors['--color-foreground'],
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    transitionDuration: '.15s',
    transitionProperty:
      'color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, --tw-gradient-from, --tw-gradient-via, --tw-gradient-to, opacity, box-shadow, transform, translate, scale, rotate, filter, -webkit-backdrop-filter, backdrop-filter, display, content-visibility, overlay, pointer-events',
    transitionTimingFunction: `var(--tw-ease, ${themeMotion['--default-transition-timing-function']})`,
    height: `calc(${themeLayout['--spacing']} * 7)`,
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
    marginBlockEnd: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))`,
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)': `calc(var(--spacing) * 3 * var(--tw-space-y-reverse))`,
    },
  },
})
