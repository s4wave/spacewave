import type { DiffFileStat } from '@s4wave/sdk/git/repo.pb.js'

// DiffStatSectionProps are props for the DiffStatSection component.
export interface DiffStatSectionProps {
  files: DiffFileStat[] | undefined
  loading: boolean
}

// DiffStatSection displays the file-level diff stats for a commit.
export function DiffStatSection({ files, loading }: DiffStatSectionProps) {
  if (loading) {
    return (
      <div className="text-foreground-alt text-xs">Loading diff stat...</div>
    )
  }

  if (!files || files.length === 0) {
    return <div className="text-foreground-alt/70 text-xs">No file changes</div>
  }

  const totalAdditions = files.reduce((sum, f) => sum + (f.additions ?? 0), 0)
  const totalDeletions = files.reduce((sum, f) => sum + (f.deletions ?? 0), 0)

  return (
    <div className="font-mono text-xs">
      {files.map((file) => (
        <div key={file.path} className="flex items-center gap-2 py-0.5">
          <span className="text-foreground min-w-0 flex-1 truncate">
            {file.path}
          </span>
          <span className="shrink-0">
            {(file.additions ?? 0) > 0 && (
              <span className="text-green-500">+{file.additions}</span>
            )}
            {(file.additions ?? 0) > 0 && (file.deletions ?? 0) > 0 && ' '}
            {(file.deletions ?? 0) > 0 && (
              <span className="text-red-500">-{file.deletions}</span>
            )}
          </span>
        </div>
      ))}
      <div className="border-foreground/8 text-foreground-alt mt-1 flex items-center gap-2 border-t pt-1">
        <span>
          {files.length} file{files.length !== 1 ? 's' : ''} changed
        </span>
        {totalAdditions > 0 && (
          <span className="text-green-500">+{totalAdditions}</span>
        )}
        {totalDeletions > 0 && (
          <span className="text-red-500">-{totalDeletions}</span>
        )}
      </div>
    </div>
  )
}
