import { useCallback } from 'react'

// TagChipProps defines the props for TagChip.
interface TagChipProps {
  tag: string
  onSelectTag?: (tag: string) => void
}

// TagChip renders a clickable tag pill.
export function TagChip({ tag, onSelectTag }: TagChipProps) {
  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      onSelectTag?.(tag)
    },
    [onSelectTag, tag],
  )

  return (
    <button
      onClick={handleClick}
      className="text-foreground-alt/70 hover:text-brand hover:border-brand/30 hover:bg-brand/5 cursor-pointer rounded-md border border-white/8 px-2 py-0.5 text-xs font-medium transition-all duration-200"
    >
      {tag}
    </button>
  )
}
