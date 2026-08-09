import { useCallback } from 'react'

import { useNavigate } from '@s4wave/web/router/router.js'

interface TagChipProps {
  tag: string
}

export function TagChip({ tag }: TagChipProps) {
  const navigate = useNavigate()

  const handleTagSelect = useCallback(() => {
    navigate({ path: `/blog/tag/${tag}` })
  }, [navigate, tag])

  return (
    <button
      onClick={handleTagSelect}
      className="text-foreground-alt/70 hover:text-brand hover:border-brand/30 hover:bg-brand/5 pointer-events-auto cursor-pointer rounded-md border border-white/8 px-2 py-0.5 text-xs font-medium transition duration-200"
    >
      {tag}
    </button>
  )
}
