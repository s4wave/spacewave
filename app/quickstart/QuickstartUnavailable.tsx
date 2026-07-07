import { EmptyState } from '@s4wave/web/ui/EmptyState.js'
import { Button } from '@s4wave/web/ui/button.js'

interface QuickstartUnavailableProps {
  quickstartId?: string
  homeHref?: string
  onBack?: () => void
}

export function QuickstartUnavailable({
  quickstartId,
  homeHref = '/',
  onBack,
}: QuickstartUnavailableProps) {
  const description = quickstartId
    ? `The "${quickstartId}" quickstart is not part of the current public Spacewave catalog. Choose an available quickstart from the home page.`
    : 'This quickstart route is not part of the current public Spacewave catalog. Choose an available quickstart from the home page.'

  return (
    <div className="bg-background-landing flex min-h-screen w-full flex-1 items-center justify-center p-6">
      <div className="bg-background/90 w-full max-w-md rounded-2xl border p-6 shadow-sm">
        <EmptyState
          title="Quickstart not available"
          description={description}
          className="p-0"
        />
        <div className="mt-6 flex justify-center">
          {onBack ? (
            <Button type="button" variant="outline" onClick={onBack}>
              Back to home
            </Button>
          ) : (
            <Button asChild variant="outline">
              <a href={homeHref}>Back to home</a>
            </Button>
          )}
        </div>
      </div>
    </div>
  )
}
