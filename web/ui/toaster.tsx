import { Toaster as Sonner, toast } from 'sonner'
import type { StyleXStyles } from '@stylexjs/stylex'
import { stylexProps } from '@s4wave/web/style/props.js'

import { sonnerClassNames as skin } from '../style/sonner.js'

type ToasterProps = Omit<React.ComponentProps<typeof Sonner>, 'className'> & {
  sx?: StyleXStyles
  className?: never
}

/** Toaster styles the vendor-owned toast elements through the shared skin. */
function Toaster({ sx, ...props }: ToasterProps) {
  const attributes = stylexProps(sx, props)
  return (
    <Sonner
      theme="dark"
      position="bottom-right"
      offset={{ bottom: 50, right: 12 }}
      mobileOffset={{ bottom: 50, right: 12 }}
      gap={8}
      toastOptions={{
        classNames: {
          toast: skin.toast,
          title: skin.title,
          description: skin.description,
          content: skin.content,
          icon: skin.icon,
          closeButton: skin.closeButton,
          actionButton: skin.actionButton,
          cancelButton: skin.cancelButton,
          success: skin.success,
          info: skin.info,
          warning: skin.warning,
          error: skin.error,
        },
      }}
      {...attributes}
      className={[skin.toaster, attributes.className].filter(Boolean).join(' ')}
    />
  )
}

export { Toaster, toast }
