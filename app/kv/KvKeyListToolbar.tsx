import { useCallback } from 'react'

import { Input } from '@s4wave/web/ui/input.js'

interface KvKeyListToolbarProps {
  prefix: string
  onPrefixChange: (prefix: string) => void
}

// KvKeyListToolbar renders the prefix filter for the key list, which narrows
// the watched list as the person types.
export function KvKeyListToolbar({
  prefix,
  onPrefixChange,
}: KvKeyListToolbarProps) {
  const handlePrefixChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      onPrefixChange(event.target.value)
    },
    [onPrefixChange],
  )

  return (
    <div className="border-foreground/8 flex shrink-0 items-center gap-2 border-b px-2 py-1.5">
      <Input
        value={prefix}
        onChange={handlePrefixChange}
        placeholder="Filter by key prefix"
        aria-label="Filter by key prefix"
        variant="toolbar"
        className="flex-1"
      />
    </div>
  )
}
