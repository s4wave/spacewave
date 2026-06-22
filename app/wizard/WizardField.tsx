import { type ComponentProps, type ReactNode, type Ref } from 'react'

import { cn } from '@s4wave/web/style/utils.js'
import { Input } from '@s4wave/web/ui/input.js'

import { wizardInputClassName } from './wizard-field-styles.js'
import { WizardFieldFrame } from './WizardFieldFrame.js'

export interface WizardFieldProps extends ComponentProps<'input'> {
  label: ReactNode
  help?: ReactNode
  fieldClassName?: string
  labelClassName?: string
  inputRef?: Ref<HTMLInputElement>
}

export function WizardField({
  label,
  help,
  fieldClassName,
  labelClassName,
  className,
  inputRef,
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
        className={cn(wizardInputClassName, className)}
        {...props}
      />
    </WizardFieldFrame>
  )
}
