import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { type ComponentProps, type ReactNode, type Ref } from 'react'

import { Input } from '@s4wave/web/ui/input.js'

import { WizardFieldFrame } from './WizardFieldFrame.js'

export interface WizardFieldProps extends Omit<
  ComponentProps<'input'>,
  'className'
> {
  className?: never
  sx?: ComponentStyles
  label: ReactNode
  help?: ReactNode
  fieldSx?: ComponentStyles
  labelSx?: ComponentStyles
  inputRef?: Ref<HTMLInputElement>
  variant?: 'default' | 'error' | 'compactMono'
}

export function WizardField({
  label,
  help,
  fieldSx,
  labelSx,
  sx,
  inputRef,
  variant = 'default',
  ...props
}: WizardFieldProps) {
  return (
    <WizardFieldFrame label={label} help={help} sx={fieldSx} labelSx={labelSx}>
      {(controlSx) => (
        <Input
          sx={[controlSx, sx]}
          ref={inputRef}
          variant={
            variant === 'error'
              ? 'wizardError'
              : variant === 'compactMono'
                ? 'wizardMono'
                : 'wizard'
          }
          {...props}
        />
      )}
    </WizardFieldFrame>
  )
}
