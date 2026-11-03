import { useCallback, type FocusEvent, type ReactNode } from 'react'
import { LuArrowLeft, LuCheck, LuTrash2 } from '@s4wave/web/images/icons.js'

import { Button } from '@s4wave/web/ui/button.js'
import { DashboardButton } from '@s4wave/web/ui/DashboardButton.js'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@s4wave/web/ui/tooltip.js'

import { WizardField } from './WizardField.js'

import * as stylex from '@stylexjs/stylex'
import { layout as themeLayout } from '../../web/style/tokens.stylex.js'

import {
  utilityState as themeUtilityState,
  colors as themeColors,
  fonts as themeFonts,
  radii as themeRadii,
} from '../../web/style/tokens.stylex.js'

const stylexStyles2 = stylex.create({
  rule1: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    flexShrink: 0,
    justifyContent: 'space-between',
    borderBottomStyle: themeUtilityState['--tw-border-style'],
    borderBottomWidth: '1px',
    minHeight: `calc(${themeLayout['--spacing']} * 9)`,
  },
  rule2: {
    '--tw-font-weight': 'var(--font-weight-semibold)',
    '--tw-tracking': 'var(--tracking-tight)',
    WebkitUserSelect: 'none',
    alignItems: 'center',
    color: themeColors['--color-foreground'],
    display: 'flex',
    fontSize: themeFonts['--text-sm'],
    fontWeight: 'var(--font-weight-semibold)',
    letterSpacing: 'var(--tracking-tight)',
    lineHeight: `var(--tw-leading, ${themeFonts['--text-sm--line-height']})`,
    userSelect: 'none',
    minWidth: 0,
  },
  rule3: {
    padding: `calc(${themeLayout['--spacing']} * 3.5)`,
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: themeRadii['--radius-lg'],
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
  },
  rule4: {
    color: themeColors['--color-destructive'],
    fontSize: themeFonts['--text-xs'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
    marginTop: `calc(${themeLayout['--spacing']} * 1.5)`,
  },
  rule5: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 8%, transparent)`,
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'center',
    display: 'flex',
    justifyContent: 'space-between',
    borderTopStyle: themeUtilityState['--tw-border-style'],
    borderTopWidth: '1px',
  },
  rule6: { gap: `calc(${themeLayout['--spacing']} * 2)`, display: 'flex' },
  rule7: {
    paddingBlock: {
      default: `calc(${themeLayout['--spacing']} * 10)`,
      '@media (max-height: 470px)': `calc(${themeLayout['--spacing']} * 2)`,
    },
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    alignItems: 'flex-start',
    display: 'flex',
    justifyContent: 'center',
    height: '100%',
    width: '100%',
  },
  rule8: {
    borderColor: `color-mix(in oklab, ${themeColors['--color-foreground']} 6%, transparent)`,
    borderRadius: 'var(--radius-xl)',
    borderStyle: themeUtilityState['--tw-border-style'],
    borderWidth: '1px',
    overflow: 'hidden',
    backgroundColor: `color-mix(in oklab, ${themeColors['--color-background-card']} 30%, transparent)`,
    display: 'flex',
    flexDirection: 'column',
    maxHeight: '100%',
    width: '100%',
  },
  rule9: { maxWidth: 'var(--container-2xl)' },
  rule10: { maxWidth: 'var(--container-lg)' },
  rule11: {
    flex: 1,
    paddingBlock: `calc(${themeLayout['--spacing']} * 3)`,
    paddingInline: `calc(${themeLayout['--spacing']} * 4)`,
    minHeight: 0,
    overflowY: 'auto',
  },
  rule12: {
    WebkitUserSelect: 'none',
    gap: `calc(${themeLayout['--spacing']} * 2)`,
    alignItems: 'center',
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 50%, transparent)`,
    display: 'flex',
    userSelect: 'none',
  },
  rule13: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    '--tw-tracking': 'var(--tracking-widest)',
    fontSize: '.6rem',
    fontWeight: themeFonts['--font-weight-medium'],
    letterSpacing: 'var(--tracking-widest)',
    lineHeight: 1.5,
    textTransform: 'uppercase',
  },
  rule14: {
    '--tw-font-weight': themeFonts['--font-weight-medium'],
    color: `color-mix(in oklab, ${themeColors['--color-foreground-alt']} 80%, transparent)`,
    fontSize: themeFonts['--text-xs'],
    fontWeight: themeFonts['--font-weight-medium'],
    lineHeight: `var(--tw-leading, ${themeFonts['--text-xs--line-height']})`,
  },
})

const stylexStyles = stylex.create({
  rule1: {
    height: `calc(${themeLayout['--spacing']} * 3.5)`,
    width: `calc(${themeLayout['--spacing']} * 3.5)`,
  },
})

export interface WizardShellProps {
  title: ReactNode
  step: number
  totalSteps?: number
  stepName?: string
  localName: string
  onUpdateName: (name: string) => void
  onBack: () => void
  canBack?: boolean
  onCancel: () => void
  // Content slot rendered between header and buttons.
  children?: (childSx: stylex.StyleXStyles) => React.ReactNode
  // Name input configuration.
  nameLabel?: string
  namePlaceholder?: string
  nameHelp?: string
  nameError?: string
  // Which step shows the name input. Defaults to 0.
  nameStep?: number
  // Selects the existing default name when the name input receives focus.
  selectNameOnFocus?: boolean
  // Primary action button.
  creating: boolean
  createLabel?: string
  creatingLabel?: string
  onFinalize: () => void
  canFinalize?: boolean
  // Optional next button for multi-step wizards where step 0 is config.
  onNext?: () => void
  canNext?: boolean
  nextLabel?: string
  nextBusyLabel?: string
  nextBusy?: boolean
  // Which step shows the finalize button. Defaults to 0 (single-step).
  finalizeStep?: number
  // Long-form steps can opt into the established wide wizard width.
  width?: 'default' | 'wide'
}

/** WizardHeader renders the title bar with the delete action. */
function WizardHeader({
  title,
  onCancel,
}: Pick<WizardShellProps, 'title' | 'onCancel'>) {
  return (
    <div sx={stylexStyles2.rule1}>
      <h2 sx={stylexStyles2.rule2}>{title}</h2>
      <Tooltip>
        <TooltipTrigger asChild>
          <DashboardButton
            icon={<LuTrash2 sx={stylexStyles.rule1} />}
            onClick={onCancel}
            aria-label="Delete wizard"
            variant="destructive"
          />
        </TooltipTrigger>
        <TooltipContent side="bottom">Delete wizard</TooltipContent>
      </Tooltip>
    </div>
  )
}

type WizardNameFieldProps = Pick<
  WizardShellProps,
  | 'localName'
  | 'onUpdateName'
  | 'nameLabel'
  | 'namePlaceholder'
  | 'nameHelp'
  | 'nameError'
  | 'selectNameOnFocus'
>

/** WizardNameField renders the name input with its optional error message. */
function WizardNameField({
  sx,
  localName,
  onUpdateName,
  nameLabel = 'Name',
  namePlaceholder = 'Enter a name...',
  nameHelp,
  nameError,
  selectNameOnFocus = false,
}: WizardNameFieldProps & { sx?: stylex.StyleXStyles; className?: never }) {
  const handleNameInputRef = useCallback(
    (node: HTMLInputElement | null) => {
      node?.focus()
      if (selectNameOnFocus) node?.select()
    },
    [selectNameOnFocus],
  )
  const handleNameFocus = useCallback(
    (event: FocusEvent<HTMLInputElement>) => {
      if (selectNameOnFocus) event.currentTarget.select()
    },
    [selectNameOnFocus],
  )

  return (
    <section sx={sx}>
      <div sx={stylexStyles2.rule3}>
        <WizardField
          inputRef={handleNameInputRef}
          label={nameLabel}
          value={localName}
          onChange={(e) => onUpdateName(e.target.value)}
          placeholder={namePlaceholder}
          help={nameHelp}
          aria-invalid={nameError ? true : undefined}
          aria-describedby={nameError ? 'wizard-name-error' : undefined}
          variant={nameError ? 'error' : 'default'}
          onFocus={handleNameFocus}
        />
        {nameError && (
          <p id="wizard-name-error" sx={stylexStyles2.rule4}>
            {nameError}
          </p>
        )}
      </div>
    </section>
  )
}

type WizardFooterProps = Pick<
  WizardShellProps,
  | 'step'
  | 'onBack'
  | 'canBack'
  | 'creating'
  | 'createLabel'
  | 'creatingLabel'
  | 'onFinalize'
  | 'canFinalize'
  | 'onNext'
  | 'canNext'
  | 'nextLabel'
  | 'nextBusyLabel'
  | 'nextBusy'
  | 'finalizeStep'
> & { canSubmit: boolean }

/** WizardFooter renders the back, next, and finalize buttons for the current step. */
function WizardFooter({
  step,
  onBack,
  canBack = true,
  creating,
  createLabel = 'Create',
  creatingLabel = 'Creating...',
  onFinalize,
  canFinalize = true,
  onNext,
  canNext = true,
  nextLabel = 'Next',
  nextBusyLabel = 'Opening…',
  nextBusy = false,
  finalizeStep,
  canSubmit,
}: WizardFooterProps) {
  const showFinalize = finalizeStep === undefined || step === finalizeStep

  return (
    <div sx={stylexStyles2.rule5}>
      <div>
        {step > 0 && (
          <DashboardButton
            icon={<LuArrowLeft sx={stylexStyles.rule1} />}
            onClick={onBack}
            disabled={!canBack}
          >
            Back
          </DashboardButton>
        )}
      </div>
      <div sx={stylexStyles2.rule6}>
        {onNext && step < (finalizeStep ?? 0) && (
          <Button
            onClick={onNext}
            disabled={nextBusy || !canNext}
            variant="brandOutline"
            size="toolbarWide"
          >
            {nextBusy ? nextBusyLabel : nextLabel}
          </Button>
        )}
        {showFinalize && (
          <Button
            onClick={onFinalize}
            disabled={creating || !canSubmit || !canFinalize}
            variant="brandOutline"
            size="toolbarWide"
          >
            <LuCheck sx={stylexStyles.rule1} />
            {creating ? creatingLabel : createLabel}
          </Button>
        )}
      </div>
    </div>
  )
}

// WizardShell renders the shared wizard layout: header, step indicator,
// content slot, name input, and button grid. The content scrolls between the
// header and the buttons, so a short window keeps the buttons in reach.
export function WizardShell({
  title,
  step,
  totalSteps,
  stepName,
  onCancel,
  children,
  nameStep = 0,
  width = 'default',
  ...rest
}: WizardShellProps) {
  const stepLabel =
    totalSteps !== undefined
      ? `Step ${step + 1} of ${totalSteps}`
      : `Step ${step + 1}`

  return (
    <div sx={stylexStyles2.rule7}>
      <div
        sx={[
          stylexStyles2.rule8,
          width === 'wide' ? stylexStyles2.rule9 : stylexStyles2.rule10,
        ]}
      >
        <WizardHeader title={title} onCancel={onCancel} />

        <div sx={stylexStyles2.rule11}>
          <div sx={false}>
            <div sx={[setupChildren.child, stylexStyles2.rule12]}>
              <span sx={stylexStyles2.rule13}>{stepLabel}</span>
              {stepName && <span sx={stylexStyles2.rule14}>{stepName}</span>}
            </div>

            {step === nameStep && (
              <WizardNameField sx={setupChildren.child} {...rest} />
            )}

            {children?.(setupChildren.child)}
          </div>
        </div>

        <WizardFooter
          step={step}
          canSubmit={!!rest.localName.trim()}
          {...rest}
        />
      </div>
    </div>
  )
}

// setupChildren preserves the original spacing through the content slot.
const setupChildren = stylex.create({
  child: {
    '--tw-space-y-reverse': { default: null, ':not(:last-child)': 0 },
    marginBlockEnd: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * calc(1 - var(--tw-space-y-reverse)))',
    },
    marginBlockStart: {
      default: null,
      ':not(:last-child)':
        'calc(var(--spacing) * 3 * var(--tw-space-y-reverse))',
    },
  },
})
