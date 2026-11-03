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
  fieldClassName?: string
  labelClassName?: string
  inputRef?: Ref<HTMLInputElement>
  variant?: 'default' | 'error' | 'compactMono'
}

export function WizardField({
  label,
  help,
  fieldClassName,
  labelClassName,
  sx,
  inputRef,
  variant = 'default',
  ...props
}: WizardFieldProps) {
  return (
    <WizardFieldFrame
      label={label}
      help={help}
      fieldClassName={fieldClassName}
      labelClassName={labelClassName}
    >
      <Input
        sx={sx}
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
    </WizardFieldFrame>
  )
}
