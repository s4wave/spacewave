import { useCallback, useMemo, useRef, useState } from 'react'
import type { EntityCredential } from '@s4wave/core/session/session.pb.js'

// useCredentialProof manages password/PEM state for EntityCredential input.
// Returns state, handlers, and the constructed credential.
export function useCredentialProof() {
  const [password, setPassword] = useState('')
  const [pemData, setPemData] = useState<Uint8Array | null>(null)
  const [pemFileName, setPemFileName] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handlePemChange = useCallback(
    (data: Uint8Array | null, name: string | null) => {
      setPemData(data)
      setPemFileName(name)
    },
    [],
  )

  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = () => {
        const buf = reader.result
        if (buf instanceof ArrayBuffer) {
          handlePemChange(new Uint8Array(buf), file.name)
        }
      }
      reader.readAsArrayBuffer(file)
    },
    [handlePemChange],
  )

  const credential = useMemo((): EntityCredential | null => {
    if (password) {
      return { credential: { case: 'password', value: password } }
    }
    if (pemData) {
      return { credential: { case: 'pemPrivateKey', value: pemData } }
    }
    return null
  }, [password, pemData])

  const hasCredential = password.length > 0 || pemData !== null

  const reset = useCallback(() => {
    setPassword('')
    setPemData(null)
    setPemFileName(null)
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }, [])

  return {
    password,
    setPassword,
    pemData,
    pemFileName,
    handlePemChange,
    handleFileChange,
    fileInputRef,
    credential,
    hasCredential,
    reset,
  }
}
