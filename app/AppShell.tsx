import { isElectron as bldrIsElectron, isMac as bldrIsMac } from '@aptre/bldr'
import { TooltipProvider } from '@s4wave/web/ui/tooltip.js'
import { Toaster } from '@s4wave/web/ui/toaster.js'
import { StateNamespaceProvider } from '@s4wave/web/state/index.js'
import { localStateAtom } from '@s4wave/web/state/global.js'

import {
  WindowFrame,
  IWindowFrameProps,
} from '@s4wave/app/window/WindowFrame.js'

const electronStyles = `/* bldr: using electron */`

export interface IAppShellProps {
  children?: React.ReactNode
  isElectron?: boolean
  isMac?: boolean
  windowFrame?: IWindowFrameProps
}

export function AppShell(props: IAppShellProps) {
  const isElectron = props.isElectron ?? bldrIsElectron
  const isMac = props.isMac ?? bldrIsMac
  const isMacElectron = isElectron && isMac
  return (
    <StateNamespaceProvider rootAtom={localStateAtom}>
      {isElectron ?
        <style>{electronStyles}</style>
      : null}
      <WindowFrame
        className={'dark'}
        centerTopBar={isMacElectron || undefined}
        topBarHeight={isMacElectron ? 28 : undefined}
        topBar={{
          hidden: !isElectron,
        }}
        onClose={!isMacElectron ? window.close : undefined}
        {...props.windowFrame}
      >
        <TooltipProvider>
          {props.children}
          <Toaster />
        </TooltipProvider>
      </WindowFrame>
    </StateNamespaceProvider>
  )
}
