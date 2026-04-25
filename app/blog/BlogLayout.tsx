import { useCallback } from 'react'
import { LuArrowLeft } from 'react-icons/lu'
import { useNavigate } from '@s4wave/web/router/router.js'
import { LegalFooter } from '@s4wave/app/landing/LegalFooter.js'

// BlogLayoutProps defines the props for BlogLayout.
interface BlogLayoutProps {
  children: React.ReactNode
  showBack?: boolean
}

// BlogLayout renders the premium layout for blog reading pages.
export function BlogLayout({ children, showBack = true }: BlogLayoutProps) {
  const navigate = useNavigate()

  const goBack = useCallback(() => {
    navigate({ path: '/blog' })
  }, [navigate])

  return (
    <div className="bg-background-landing @container flex w-full flex-1 flex-col overflow-y-auto">
      {showBack && (
        <div className="relative z-10 mx-auto w-full max-w-3xl px-4 pt-4 @lg:px-8">
          <button
            onClick={goBack}
            className="text-foreground-alt/60 hover:text-foreground flex cursor-pointer items-center gap-2 text-xs font-medium transition-colors"
          >
            <LuArrowLeft className="h-3.5 w-3.5" />
            All posts
          </button>
        </div>
      )}

      {children}

      <LegalFooter />
    </div>
  )
}
