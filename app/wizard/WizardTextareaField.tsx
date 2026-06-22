import { type ComponentProps, type ReactNode, type Ref } from 'react'

import { cn } from '@s4wave/web/style/utils.js'

import { wizardTextareaClassName } from './wizard-field-styles.js'
import { WizardFieldFrame } from './WizardFieldFrame.js'

export interface WizardTextareaFieldProps extends ComponentProps<'textarea'> {
  label: ReactNode
  help?: ReactNode
  fieldClassName?: string
  labelClassName?: string
  textareaRef?: Ref<HTMLTextAreaElement>
}

export function WizardTextareaField({
  label,
  help,
  fieldClassName,
  labelClassName,
  className,
  textareaRef,
  ...props
}: WizardTextareaFieldProps) {
  return (
    <WizardFieldFrame
      label={label}
      help={help}
      fieldClassName={fieldClassName}
      labelClassName={labelClassName}
    >
      <textarea
        ref={textareaRef}
        className={cn(wizardTextareaClassName, className)}
        {...props}
      />
    </WizardFieldFrame>
  )
}
