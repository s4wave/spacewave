import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@s4wave/web/ui/dialog.js'

function getFormString(form: FormData, name: string): string {
  const value = form.get(name)
  return typeof value === 'string' ? value.trim() : ''
}

interface TextInputDialogProps {
  open: boolean
  title: string
  description?: string
  label: string
  defaultValue?: string
  placeholder?: string
  confirmLabel: string
  requireValue?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (value: string) => void
}

export function TextInputDialog({
  open,
  title,
  description,
  label,
  defaultValue = '',
  placeholder,
  confirmLabel,
  requireValue,
  onOpenChange,
  onConfirm,
}: TextInputDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* eslint-disable-next-line react-doctor/no-prevent-default -- plugin dialogs submit to in-memory callbacks, not no-JS document actions. */}
        <form
          onSubmit={(event) => {
            event.preventDefault()
            const form = new FormData(event.currentTarget)
            const value = getFormString(form, 'value')
            if (requireValue && !value) return
            onConfirm(value)
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : null}
          </DialogHeader>
          <label className="text-foreground mt-4 flex flex-col gap-1 text-xs">
            {label}
            <input
              key={`${open ? 'open' : 'closed'}:${defaultValue}`}
              name="value"
              defaultValue={defaultValue}
              placeholder={placeholder}
              required={requireValue}
              className="border-border bg-background text-foreground focus:border-brand h-8 rounded-md border px-2 text-sm outline-none"
              autoFocus
            />
          </label>
          <DialogFooter className="mt-4">
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              className="text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition duration-150"
            >
              {confirmLabel}
            </button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface SourceInputDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (source: { name: string; ref: string }) => void
}

export function SourceInputDialog({
  open,
  onOpenChange,
  onConfirm,
}: SourceInputDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        {/* eslint-disable-next-line react-doctor/no-prevent-default -- plugin dialogs submit to in-memory callbacks, not no-JS document actions. */}
        <form
          onSubmit={(event) => {
            event.preventDefault()
            const form = new FormData(event.currentTarget)
            const name = getFormString(form, 'name')
            const ref = getFormString(form, 'ref')
            if (!ref) return
            onConfirm({ name, ref })
          }}
        >
          <DialogHeader>
            <DialogTitle>Add source</DialogTitle>
            <DialogDescription>
              Connect this notebook to a folder or document source.
            </DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <label className="text-foreground flex flex-col gap-1 text-xs">
              Source name
              <input
                key={open ? 'source-name-open' : 'source-name-closed'}
                name="name"
                placeholder="Docs"
                className="border-border bg-background text-foreground focus:border-brand h-8 rounded-md border px-2 text-sm outline-none"
                autoFocus
              />
            </label>
            <label className="text-foreground flex flex-col gap-1 text-xs">
              Source ref
              <input
                key={open ? 'source-ref-open' : 'source-ref-closed'}
                name="ref"
                placeholder="object-key/-/path"
                required
                className="border-border bg-background text-foreground focus:border-brand h-8 rounded-md border px-2 font-mono text-sm outline-none"
              />
            </label>
          </div>
          <DialogFooter className="mt-4">
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              className="text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="border-brand/30 bg-brand/10 hover:border-brand/50 hover:bg-brand/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition duration-150"
            >
              Add source
            </button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

interface ConfirmActionDialogProps {
  open: boolean
  title: string
  description: string
  confirmLabel: string
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}

export function ConfirmActionDialog({
  open,
  title,
  description,
  confirmLabel,
  onOpenChange,
  onConfirm,
}: ConfirmActionDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="text-foreground-alt hover:text-foreground h-7 rounded-md px-3 text-xs transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={onConfirm}
            className="border-destructive/30 bg-destructive/10 hover:border-destructive/50 hover:bg-destructive/15 text-foreground h-7 rounded-md border px-3 text-xs font-medium transition duration-150"
          >
            {confirmLabel}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
