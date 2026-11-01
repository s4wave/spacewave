import * as React from 'react'

import { cn } from '@s4wave/web/style/utils.js'

function Card({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'bg-card text-card-foreground flex flex-col gap-6 rounded-xl border py-6 shadow-sm',
          className,
        )
      }
      {...props}
    />
  )
}

function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-header"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: [.border-b]:pb-6 */ cn(
          '@container/card-header grid auto-rows-min grid-rows-[auto_auto] items-start gap-2 px-6 has-data-[slot=card-action]:grid-cols-[1fr_auto] [.border-b]:pb-6',
          className,
        )
      }
      {...props}
    />
  )
}

function CardTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-title"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'leading-none font-semibold',
          className,
        )
      }
      {...props}
    />
  )
}

function CardDescription({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-description"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'text-muted-foreground text-sm',
          className,
        )
      }
      {...props}
    />
  )
}

function CardAction({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-action"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'col-start-2 row-span-2 row-start-1 self-start justify-self-end',
          className,
        )
      }
      {...props}
    />
  )
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-content"
      className={
        /* TODO(stylex): dynamic class or forwarded className */ cn(
          'px-6',
          className,
        )
      }
      {...props}
    />
  )
}

function CardFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-footer"
      className={
        /* TODO(stylex): a descendant, group, peer or compound selector: [.border-t]:pt-6 */ cn(
          'flex items-center px-6 [.border-t]:pt-6',
          className,
        )
      }
      {...props}
    />
  )
}

export {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
}
