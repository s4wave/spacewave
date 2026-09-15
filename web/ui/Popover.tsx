import * as React from 'react'
import * as PopoverPrimitive from '@radix-ui/react-popover'

import { cn } from '../style/utils.js'
import { useAppEnvironment } from '@s4wave/web/sdk/app/environment.js'

function Popover({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />
}

function PopoverTrigger({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />
}

function PopoverContent({
  className,
  variant,
  align = 'center',
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content> & {
  variant?: 'default' | 'status' | 'compact'
}) {
  const environment = useAppEnvironment()
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-spacewave-app={environment.id || undefined}
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        className={cn(
          'border-popover-border bg-popover text-popover-text data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 z-50 w-72 origin-(--radix-popover-content-transform-origin) rounded-md border p-4 shadow-lg outline-hidden',
          variant === 'status' &&
            'border-foreground/15 bg-background-card text-foreground z-50 w-80 max-w-(--max-width-viewport-control) rounded-lg p-0 shadow-xl backdrop-blur-md',
          variant === 'compact' && 'w-72 p-3',
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

function PopoverAnchor({
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Anchor>) {
  return <PopoverPrimitive.Anchor data-slot="popover-anchor" {...props} />
}

export { Popover, PopoverTrigger, PopoverContent, PopoverAnchor }
