import { useCallback } from 'react'
import {
  type BorderNode,
  type ITabSetRenderValues,
  type TabSetNode,
} from '@aptre/flex-layout'
import { LuExternalLink, LuPlus, LuX } from 'react-icons/lu'

interface ShellTabToolbarOptions {
  canClose: boolean
  onClose: () => void
  onNew: () => void
  onPopout: () => void
}

export function useShellTabToolbar({
  canClose,
  onClose,
  onNew,
  onPopout,
}: ShellTabToolbarOptions) {
  return useCallback(
    (node: TabSetNode | BorderNode, renderValues: ITabSetRenderValues) => {
      if (node.getType() !== 'tabset') return
      renderValues.stickyButtons.push(
        <button
          key="close-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={onClose}
          title="Close tab"
          disabled={!canClose}
        >
          <LuX className="size-2.5" />
        </button>,
        <button
          key="add-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={onNew}
          title="New tab"
        >
          <LuPlus className="size-2.5" />
        </button>,
        <button
          key="popout-tab"
          className="flexlayout__tab_toolbar_button"
          onClick={onPopout}
          title="Open in new tab"
        >
          <LuExternalLink className="size-2.5" />
        </button>,
      )
    },
    [canClose, onClose, onNew, onPopout],
  )
}
