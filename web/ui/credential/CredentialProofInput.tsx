import { LuUpload } from 'react-icons/lu'

import { cn } from '@s4wave/web/style/utils.js'

export interface CredentialProofInputProps {
  password: string
  onPasswordChange: (v: string) => void
  pemFileName?: string | null
  onFileChange?: (e: React.ChangeEvent<HTMLInputElement>) => void
  fileInputRef?: React.RefObject<HTMLInputElement | null>
  showPassword?: boolean
  showPem?: boolean
  passwordLabel?: string
  passwordPlaceholder?: string
  pemLabel?: string
  error?: string | null
  disabled?: boolean
  autoFocus?: boolean
  onPasswordKeyDown?: (e: React.KeyboardEvent) => void
  className?: string
}

export const inputClass = cn(
  'border-foreground/20 bg-background/30 text-foreground placeholder:text-foreground-alt/50 w-full rounded-md border px-3 py-2 text-sm transition-colors outline-none',
  'focus:border-brand/50',
)

// CredentialProofInput renders password + optional PEM file upload for
// EntityCredential proof. Presentational only, all state managed by parent
// or useCredentialProof hook.
export function CredentialProofInput({
  password,
  onPasswordChange,
  pemFileName,
  onFileChange,
  fileInputRef,
  showPassword = true,
  showPem = true,
  passwordLabel = 'Account password',
  passwordPlaceholder = 'Enter your password',
  pemLabel = 'Backup key (.pem)',
  error,
  disabled,
  autoFocus,
  onPasswordKeyDown,
  className,
}: CredentialProofInputProps) {
  return (
    <div className={cn('space-y-3', className)}>
      {showPassword && (
        <div>
          <label className="text-foreground-alt mb-1.5 block text-xs select-none">
            {passwordLabel}
          </label>
          <input
            type="password"
            value={password}
            onChange={(e) => onPasswordChange(e.target.value)}
            placeholder={passwordPlaceholder}
            disabled={disabled}
            readOnly={disabled}
            autoFocus={autoFocus}
            onKeyDown={onPasswordKeyDown}
            className={cn(inputClass, disabled && 'opacity-50')}
          />
        </div>
      )}
      {showPem && (
        <>
          {showPassword && (
            <div className="text-foreground-alt flex items-center gap-2 text-xs">
              <div className="bg-foreground/20 h-px flex-1" />
              <span>or</span>
              <div className="bg-foreground/20 h-px flex-1" />
            </div>
          )}
          <div>
            <label className="text-foreground-alt mb-1.5 block text-xs select-none">
              {pemLabel}
            </label>
            <button
              type="button"
              onClick={() => fileInputRef?.current?.click()}
              disabled={disabled}
              className={cn(
                inputClass,
                'flex items-center gap-2 text-left',
                'hover:border-foreground/30',
                disabled && 'opacity-50',
              )}
            >
              <LuUpload className="text-foreground-alt h-3.5 w-3.5 shrink-0" />
              <span className={cn(!pemFileName && 'text-foreground-alt/50')}>
                {pemFileName ?? 'Choose .pem file'}
              </span>
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".pem"
              onChange={onFileChange}
              disabled={disabled}
              className="hidden"
            />
          </div>
        </>
      )}
      {error && <p className="text-destructive text-xs">{error}</p>}
    </div>
  )
}
