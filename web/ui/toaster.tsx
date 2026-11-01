import { Toaster as Sonner, toast } from 'sonner'
import { sonnerClassNames as skin } from '../style/sonner.js'

type ToasterProps = React.ComponentProps<typeof Sonner>

function Toaster({ ...props }: ToasterProps) {
  return (
    <Sonner
      theme="dark"
      className={skin.toaster}
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
      {...props}
    />
  )
}

export { Toaster, toast }
