import { useEffect } from 'react'

import { hasNativeFileDrag } from '@s4wave/web/dnd/app-drag.js'

export function FileDropGuard() {
  useEffect(() => {
    const onDragOver = (event: DragEvent) => {
      if (event.defaultPrevented || !hasNativeFileDrag(event.dataTransfer))
        return
      event.preventDefault()
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'none'
    }
    const onDrop = (event: DragEvent) => {
      if (event.defaultPrevented || !hasNativeFileDrag(event.dataTransfer))
        return
      event.preventDefault()
    }

    document.addEventListener('dragover', onDragOver)
    document.addEventListener('drop', onDrop)
    return () => {
      document.removeEventListener('dragover', onDragOver)
      document.removeEventListener('drop', onDrop)
    }
  }, [])

  return null
}
