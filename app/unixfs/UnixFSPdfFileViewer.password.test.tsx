import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import { cleanup, render, screen } from '@testing-library/react'
import { pdfjs } from 'react-pdf'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UnixFSPdfFileViewer } from './UnixFSPdfFileViewer.js'

// lockedPdf is a one-page PDF encrypted with the user password "secret".
const lockedPdf =
  'data:application/pdf;base64,JVBERi0xLjMKJeLjz9MKMSAwIG9iago8PAovUHJvZHVjZXIgPGRiYmU2NjlmZTM+Cj4+CmVuZG9iagoyIDAgb2JqCjw8Ci9UeXBlIC9QYWdlcwovQ291bnQgMQovS2lkcyBbIDQgMCBSIF0KPj4KZW5kb2JqCjMgMCBvYmoKPDwKL1R5cGUgL0NhdGFsb2cKL1BhZ2VzIDIgMCBSCj4+CmVuZG9iago0IDAgb2JqCjw8Ci9UeXBlIC9QYWdlCi9SZXNvdXJjZXMgPDwKPj4KL01lZGlhQm94IFsgMC4wIDAuMCAyMDAgMjAwIF0KL1BhcmVudCAyIDAgUgo+PgplbmRvYmoKNSAwIG9iago8PAovViAyCi9SIDMKL0xlbmd0aCAxMjgKL1AgNDI5NDk2NzI5MgovRmlsdGVyIC9TdGFuZGFyZAovTyA8MGRiNTg1NWZjNTMyNjU2OWU3NjU5MDZjYWY2NGU0NDI5YTRjMjBkNmU5OTZmZGVmOTYzZTliNTA4MGY5ZTA4Mz4KL1UgPGVkY2YyMDAwYjAwZmJiYjJjNzkwYTgyYzZhM2Q5YjhjMjhiZjRlNWU0ZTc1OGE0MTY0MDA0ZTU2ZmZmYTAxMDg+Cj4+CmVuZG9iagp4cmVmCjAgNgowMDAwMDAwMDAwIDY1NTM1IGYgCjAwMDAwMDAwMTUgMDAwMDAgbiAKMDAwMDAwMDA1OSAwMDAwMCBuIAowMDAwMDAwMTE4IDAwMDAwIG4gCjAwMDAwMDAxNjcgMDAwMDAgbiAKMDAwMDAwMDI2MSAwMDAwMCBuIAp0cmFpbGVyCjw8Ci9TaXplIDYKL1Jvb3QgMyAwIFIKL0luZm8gMSAwIFIKL0lEIFsgPGQ0MjlhZWY0NDYwNTZjNTdhOTVmOGQzM2ZlYzI5NzAwPiA8ZDQyOWFlZjQ0NjA1NmM1N2E5NWY4ZDMzZmVjMjk3MDA+IF0KL0VuY3J5cHQgNSAwIFIKPj4Kc3RhcnR4cmVmCjQ3NgolJUVPRgo='

// These run the real react-pdf Document and PDF.js, unlike the mocked tests
// beside them, because the password path lives inside PDF.js.
describe('UnixFSPdfFileViewer with PDF.js', () => {
  // Under Node, PDF.js runs its worker in-process and imports it by file URL;
  // the viewer's own worker URL resolves to http under happy-dom.
  beforeEach(() => {
    const require = createRequire(import.meta.url)
    pdfjs.GlobalWorkerOptions.workerSrc = pathToFileURL(
      require.resolve('pdfjs-dist/build/pdf.worker.min.mjs'),
    ).href
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('fails a password-protected PDF without prompting', async () => {
    // A prompt would answer Cancel; the viewer must never ask.
    const prompt = vi.fn(() => null)
    vi.stubGlobal('prompt', prompt)
    render(<UnixFSPdfFileViewer title="locked.pdf" inlineFileURL={lockedPdf} />)

    // PDF.js reads the file in-process and refuses it for its password.
    const alert = await screen.findByRole('alert', undefined, {
      timeout: 8000,
    })
    expect(alert.textContent).toContain('This PDF is protected by a password.')
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(prompt).not.toHaveBeenCalled()
  })
})
