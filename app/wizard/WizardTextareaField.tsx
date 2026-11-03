import type { StyleXStyles as ComponentStyles } from '@stylexjs/stylex'
import { type ComponentProps, type ReactNode, type Ref } from 'react'

import { wizardFieldStyles } from './wizard-field-styles.js'
import { WizardFieldFrame } from './WizardFieldFrame.js'

export interface WizardTextareaFieldProps extends Omit<
  ComponentProps<'textarea'>,
  'className'
> {
  sx?: ComponentStyles
  className?: never
  label: ReactNode
  help?: ReactNode
  fieldSx?: ComponentStyles
  labelSx?: ComponentStyles
  textareaRef?: Ref<HTMLTextAreaElement>
}

export function WizardTextareaField({
  label,
  help,
  fieldSx,
  labelSx,
  sx,
  textareaRef,
  ...props
}: WizardTextareaFieldProps) {
  return (
    <WizardFieldFrame label={label} help={help} sx={fieldSx} labelSx={labelSx}>
      {(controlSx) => (
        <textarea
          ref={textareaRef}
          sx={[controlSx, wizardFieldStyles.textarea, sx]}
          {...props}
        />
      )}
    </WizardFieldFrame>
  )
}
