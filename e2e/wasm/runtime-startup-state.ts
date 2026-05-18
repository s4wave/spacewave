import { projectBrowserStartup } from '@s4wave/app/loading/status/browser-startup-model.js'
import type {
  BrowserBootStatus,
  BrowserStartupMark,
} from '@s4wave/app/prerender/boot-status.js'

type StartupGlobals = typeof globalThis & {
  __swBootStatus?: BrowserBootStatus
  __swStartupMarks?: BrowserStartupMark[]
}

type ImportMapShape = {
  imports?: Record<string, string>
}

function readImportMapSpecifiers(): string[] {
  const specifiers = new Set<string>()
  for (const node of document.querySelectorAll<HTMLScriptElement>(
    'script[type="importmap"]',
  )) {
    try {
      const parsed = JSON.parse(node.textContent ?? '{}') as ImportMapShape
      for (const specifier of Object.keys(parsed.imports ?? {})) {
        specifiers.add(specifier)
      }
    } catch (err) {
      specifiers.add(`invalid-import-map:${String(err)}`)
    }
  }
  return [...specifiers].sort()
}

export default function () {
  const globals = globalThis as StartupGlobals
  const startup = projectBrowserStartup(
    globals.__swBootStatus ?? {
      phase: 'loading',
      detail: 'Loading application...',
      state: 'loading',
      progress: 0.04,
    },
    globals.__swStartupMarks ?? [],
  )
  const importMapSpecifiers = readImportMapSpecifiers()

  return {
    phaseId: startup.phase.id,
    viewState: startup.view.state,
    viewDetail: startup.view.detail ?? '',
    runtime: {
      documentState: startup.evidence.runtime.document.state,
      runtimeClientState: startup.evidence.runtime.runtimeClient.state,
      serviceWorkerState: startup.evidence.runtime.serviceWorker.state,
      pluginGenerationState: startup.evidence.runtime.pluginGeneration.state,
      frameState: startup.evidence.runtime.frame.state,
      terminalFailure: startup.evidence.runtime.terminalFailure ?? null,
    },
    importMap: {
      specifiers: importMapSpecifiers,
      hasReact: importMapSpecifiers.includes('react'),
      hasReactDomClient: importMapSpecifiers.includes('react-dom/client'),
      importCount: importMapSpecifiers.length,
    },
    startupMarks: startup.evidence.marks.map((mark) => ({
      label: mark.label,
      sequence: mark.sequence,
    })),
  }
}
