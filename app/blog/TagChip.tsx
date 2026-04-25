import { useCallback } from 'react'
import { useNavigate } from '@s4wave/web/router/router.js'

// TagChipProps defines the props for TagChip.
interface TagChipProps {
  tag: string
}

// TagChip renders a clickable tag chip linking to the tag page.
export function TagChip({ tag }: TagChipProps) {
  const navigate = useNavigate()

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      navigate({ path: `/blog/tag/${tag}` })
    },
    [navigate, tag],
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
