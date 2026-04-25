import React from 'react'

export function StatusBar() {
  return (
    <div className="bg-background-tertiary border-ui-border text-text-status flex items-center justify-between border-t px-3 py-1 text-xs">
      <div className="flex items-center gap-3">
        <span>Frame: 1</span>
        <span className="text-text-status/50">|</span>
        <span>Objects: 3</span>
      </div>

      <div className="flex items-center gap-3">
        <span className="text-foreground-alt">Editor</span>
      </div>
    </div>
  )
}
