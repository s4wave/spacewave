// ReadmeSectionProps are props for the ReadmeSection component.
export interface ReadmeSectionProps {
  readmePath: string
  content: string | null
  loading: boolean
}

// ReadmeSection displays the README file content as raw preformatted text.
// Silently hidden when content fails to load (README is supplementary).
export function ReadmeSection({
  readmePath,
  content,
  loading,
}: ReadmeSectionProps) {
  if (loading || !content) return null

  const filename = readmePath.split('/').pop() ?? readmePath

  return (
    <div className="border-foreground/8 border-t">
      <div className="border-foreground/8 text-foreground flex items-center border-b px-3 py-1.5 text-xs font-medium select-none">
        {filename}
      </div>
      <div className="px-3 py-2">
        <pre className="text-foreground overflow-auto font-mono text-xs whitespace-pre-wrap">
          {content}
        </pre>
      </div>
    </div>
  )
}
