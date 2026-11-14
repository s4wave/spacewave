import { useState } from 'react'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

import {
  freeName,
  type UploadAnswer,
  type UploadManager,
  type UploadQuestion,
} from './useUploadManager.js'

// folderOf returns the display path of the folder an asking item writes to.
function folderOf(question: UploadQuestion): string {
  const { item } = question
  const cut = item.path.lastIndexOf('/')
  const sub = cut < 0 ? '' : item.path.slice(0, cut)
  const place = item.place.replace(/\/$/, '')
  if (!sub) return place || '/'
  return `${place}/${sub}`
}

// UploadConflictDialog asks what to do with a dropped file whose name its
// folder already holds. It stays open until answered: closing it would leave
// the file neither sent nor skipped.
export function UploadConflictDialog({
  uploadManager,
}: {
  uploadManager: UploadManager
}) {
  const { question } = uploadManager
  return (
    <Dialog open={question !== null}>
      {question && (
        <UploadConflictQuestion
          key={question.item.id}
          question={question}
          onAnswer={uploadManager.answer}
        />
      )}
    </Dialog>
  )
}

function UploadConflictQuestion({
  question,
  onAnswer,
}: {
  question: UploadQuestion
  onAnswer: UploadManager['answer']
}) {
  const [all, setAll] = useState(false)
  const { item, others } = question
  const file = item.conflict?.version != null
  const folder = folderOf(question)
  const [kept] = freeName(item.name, new Set())
  const press = (choice: UploadAnswer) => onAnswer(item.id, choice, all)
  const button =
    'text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors'
  const primary =
    'border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition duration-150'

  return (
    <DialogContent
      showCloseButton={false}
      onEscapeKeyDown={(event) => event.preventDefault()}
      onInteractOutside={(event) => event.preventDefault()}
    >
      <DialogHeader>
        <DialogTitle>
          {file
            ? `Replace ${item.name}?`
            : `${item.name} cannot replace a folder`}
        </DialogTitle>
        <DialogDescription>
          {file
            ? `${folder} already has a file named ${item.name}. Replace it with the one you dropped, keep both with yours numbered like ${kept}, or skip this file.`
            : `${folder} has a folder or link named ${item.name}. Keep both saves yours numbered like ${kept}, or skip it.`}
        </DialogDescription>
      </DialogHeader>
      {others.length > 0 && (
        <label className="border-foreground/6 bg-background/20 hover:border-foreground/12 hover:bg-background/30 flex items-center gap-3 rounded-md border px-3 py-2 text-sm transition duration-150">
          <input
            type="checkbox"
            checked={all}
            onChange={(event) => setAll(event.target.checked)}
            className="accent-brand size-4 rounded"
          />
          Do the same for the other {others.length}{' '}
          {others.length === 1 ? 'name' : 'names'} already taken
        </label>
      )}
      <DialogFooter>
        <button type="button" className={button} onClick={() => press('skip')}>
          Skip
        </button>
        <button
          type="button"
          className={file ? button : primary}
          onClick={() => press('keep')}
        >
          Keep both
        </button>
        {file && (
          <button
            type="button"
            className={primary}
            onClick={() => press('replace')}
          >
            Replace
          </button>
        )}
      </DialogFooter>
    </DialogContent>
  )
}
