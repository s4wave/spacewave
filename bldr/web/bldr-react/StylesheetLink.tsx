import { useCallback, useLayoutEffect, useRef } from 'react'

import { watchStylexOrder } from '../bldr/stylex-order.js'

interface IStylesheetLinkProps {
  id: string
  href: string
  onLoad: (id: string) => void
}

// StylesheetLink renders a stylesheet link and handles the load event.
// It checks if the stylesheet is already loaded from cache on mount.
export function StylesheetLink({ id, href, onLoad }: IStylesheetLinkProps) {
  const ref = useRef<HTMLLinkElement>(null)

  useLayoutEffect(() => {
    const link = ref.current
    if (!link) {
      return
    }
    watchStylexOrder(link.ownerDocument)
    // Check if the stylesheet is already loaded (cached).
    // When a stylesheet is cached, onload may fire before we attach the handler.
    // The sheet property is set once the stylesheet is fully loaded and parsed.
    if (link.sheet) {
      onLoad(id)
    }
  }, [id, href, onLoad])

  const handleLoad = useCallback(() => {
    const document = ref.current?.ownerDocument
    if (document) watchStylexOrder(document)
    onLoad(id)
  }, [id, onLoad])

  return (
    <link
      ref={ref}
      href={href}
      rel="stylesheet"
      onLoad={handleLoad}
      onError={handleLoad}
    />
  )
}
