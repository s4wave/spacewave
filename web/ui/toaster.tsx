import { Toaster as Sonner, toast } from 'sonner'

type ToasterProps = React.ComponentProps<typeof Sonner>

function Toaster({ ...props }: ToasterProps) {
  return (
    <Sonner
      theme="dark"
      className="toaster group"
      position="bottom-right"
      offset={{ bottom: 50, right: 12 }}
      mobileOffset={{ bottom: 50, right: 12 }}
      gap={8}
      style={{
        '--width': '300px',
        width: 'min(300px, calc(100vw - 24px))',
        left: 'auto',
      }}
      toastOptions={{
        classNames: {
          toast:
            'group toast group-[.toaster]:!min-h-0 group-[.toaster]:!gap-2 group-[.toaster]:!rounded-lg group-[.toaster]:!border-foreground/8 group-[.toaster]:!bg-background-card/90 group-[.toaster]:!px-3 group-[.toaster]:!py-2 group-[.toaster]:!text-xs group-[.toaster]:!text-foreground group-[.toaster]:!shadow-menu group-[.toaster]:backdrop-blur-md',
          title:
            'group-[.toast]:!text-xs group-[.toast]:!font-medium group-[.toast]:!leading-snug',
          description:
            'group-[.toast]:!text-[0.6rem] group-[.toast]:!leading-snug group-[.toast]:!text-foreground-alt/50',
          content: 'group-[.toast]:!gap-0.5',
          icon: 'group-[.toast]:!h-3.5 group-[.toast]:!w-3.5 group-[.toast]:text-brand group-[.toast]:[&>svg]:!h-3.5 group-[.toast]:[&>svg]:!w-3.5',
          closeButton:
            'group-[.toast]:!h-5 group-[.toast]:!w-5 group-[.toast]:!border-foreground/12 group-[.toast]:!bg-background-card group-[.toast]:!text-foreground-alt group-[.toast]:hover:!bg-foreground/8 group-[.toast]:hover:!text-foreground',
          actionButton:
            'group-[.toast]:!h-6 group-[.toast]:!rounded-md group-[.toast]:!bg-brand/10 group-[.toast]:!px-2 group-[.toast]:!text-[0.6rem] group-[.toast]:!font-medium group-[.toast]:!text-foreground group-[.toast]:hover:!bg-brand/15',
          cancelButton:
            'group-[.toast]:!h-6 group-[.toast]:!rounded-md group-[.toast]:!bg-foreground/5 group-[.toast]:!px-2 group-[.toast]:!text-[0.6rem] group-[.toast]:!font-medium group-[.toast]:!text-foreground-alt group-[.toast]:hover:!bg-foreground/8 group-[.toast]:hover:!text-foreground',
          success: 'group-[.toaster]:!border-brand/15',
          info: 'group-[.toaster]:!border-foreground/12',
          warning: 'group-[.toaster]:!border-warning/20',
          error: 'group-[.toaster]:!border-destructive/20',
        },
      }}
      {...props}
    />
  )
}

export { Toaster, toast }
