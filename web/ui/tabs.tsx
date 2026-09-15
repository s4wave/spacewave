import * as React from 'react'
import * as TabsPrimitive from '@radix-ui/react-tabs'

import { cn } from '@s4wave/web/style/utils.js'

function Tabs({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Root> & {
  variant?: 'default' | 'debug'
}) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      className={cn(
        'flex flex-col gap-2',
        variant === 'debug' && 'gap-5',
        className,
      )}
      {...props}
    />
  )
}

function TabsList({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.List> & {
  variant?: 'default' | 'debug'
}) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      className={cn(
        'bg-muted text-muted-foreground inline-flex h-9 w-full items-center justify-start gap-1 rounded-lg p-1',
        variant === 'debug' &&
          'bg-foreground/5 h-auto min-w-max gap-1 rounded-lg p-1',
        className,
      )}
      {...props}
    />
  )
}

function TabsTrigger({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Trigger> & {
  variant?: 'default' | 'debug'
}) {
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      className={cn(
        'data-[state=active]:bg-background data-[state=active]:text-foreground inline-flex flex-1 items-center justify-center gap-1.5 rounded-md px-2 py-1 text-sm font-medium whitespace-nowrap transition-[color,box-shadow]',
        'focus-visible:ring-ring/50 focus-visible:outline-ring focus-visible:ring-3 focus-visible:outline-1',
        'disabled:pointer-events-none disabled:opacity-50',
        'data-[state=active]:shadow-sm',
        variant === 'debug' &&
          'data-[state=active]:border-brand/20 data-[state=active]:bg-brand/10 data-[state=active]:text-foreground border border-transparent px-3 py-2',
        className,
      )}
      {...props}
    />
  )
}

function TabsContent({
  className,
  variant,
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Content> & {
  variant?: 'default' | 'form' | 'formTight'
}) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      className={cn(
        'flex-1 outline-none',
        variant === 'form' && 'space-y-4 pt-3',
        variant === 'formTight' && 'space-y-3 pt-2',
        className,
      )}
      {...props}
    />
  )
}

export { Tabs, TabsContent, TabsList, TabsTrigger }
