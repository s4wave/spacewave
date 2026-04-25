export function getGitWorktreeInlinePreviewObjectKey(params: {
  mode: 'files' | 'workdir'
  repoObjectKey?: string | null
  workdirObjectKey?: string | null
}): string | undefined {
  if (params.mode === 'files') {
    return params.repoObjectKey ?? undefined
  }
  return params.workdirObjectKey ?? undefined
}
