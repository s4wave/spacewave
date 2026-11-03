import { type ComponentProps, type ReactNode, type Ref } from 'react'

import { Input } from '@s4wave/web/ui/input.js'

import { WizardFieldFrame } from './WizardFieldFrame.js'

export interface WizardFieldProps extends ComponentProps<'input'> {
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
  className: _className,
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
