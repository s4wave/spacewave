import React from 'react'
import { To } from './router.js'
import { NavigatePath } from './NavigatePath.js'

interface RedirectProps {
  to: string | To
}

/**
 * Redirect component that performs navigation with replace=true when mounted
 * or when the "to" prop changes
 */
export function Redirect({ to }: RedirectProps) {
  return <NavigatePath to={to} replace={true} />
}
