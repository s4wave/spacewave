/* eslint-disable @typescript-eslint/no-unsafe-member-access, @typescript-eslint/no-unsafe-assignment, @typescript-eslint/no-unsafe-argument, @typescript-eslint/no-unsafe-return */
// This file works with untyped JSON data from localStorage for flex-layout persistence.
// The any types are intentional and unavoidable for JSON parsing/manipulation.

import React, {
  useState,
  forwardRef,
  ForwardedRef,
  useCallback,
  useEffect,
  useRef,
} from 'react'
import {
  Layout,
  Model,
  ILayoutProps,
  IJsonModel,
  TabNode,
  TabSetNode,
} from '@aptre/flex-layout'
import { LuMaximize2, LuMinimize2 } from 'react-icons/lu'

const icons: ILayoutProps['icons'] = {
  maximize: <LuMaximize2 className="text-foreground-alt h-[1em] w-[1em]" />,
  restore: <LuMinimize2 className="text-foreground-alt h-[1em] w-[1em]" />,
}

// getSelectedTabIdFromModel extracts the selected tab ID for each tabset.
function getSelectedTabIdFromModel(model: Model): Record<string, string> {
  const selection: Record<string, string> = {}
  model.visitNodes((node) => {
    if (node.getType() === 'tabset') {
      const tabset = node as TabSetNode
      const selectedNode = tabset.getSelectedNode()
      if (selectedNode) {
        selection[tabset.getId()] = selectedNode.getId()
      }
    }
  })
  return selection
}

// applySelectionByTabIdToJson applies selection state by finding tab IDs and converting to indices.
function applySelectionByTabIdToJson(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  json: any,
  selectedTabIds: Record<string, string>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
): any {
  if (!json || typeof json !== 'object') return json
  if (Array.isArray(json)) {
    return json.map((item) => applySelectionByTabIdToJson(item, selectedTabIds))
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const result: any = { ...json }

  if (json.type === 'tabset' && json.id && selectedTabIds[json.id]) {
    const targetTabId = selectedTabIds[json.id]
    const children = json.children as Array<{ id?: string }> | undefined
    if (children) {
      const index = children.findIndex((child) => child.id === targetTabId)
      if (index >= 0) {
        result.selected = index
      }
    }
  }

  if (json.children) {
    result.children = applySelectionByTabIdToJson(json.children, selectedTabIds)
  }
  if (json.layout) {
    result.layout = applySelectionByTabIdToJson(json.layout, selectedTabIds)
  }
  return result
}

interface LocalStorageLayoutProps extends Omit<
  ILayoutProps,
  'model' | 'factory'
> {
  storageKey: string
  defaultModel: IJsonModel
  onModelChange?: (next: Model) => void
  factory: (node: TabNode, model: Model) => React.ReactNode
  clearStateNonce: number
  // syncSelection controls whether tab selection syncs across windows (default: false)
  syncSelection?: boolean
}

export const LocalStorageLayout: React.FC<LocalStorageLayoutProps> = forwardRef(
  (props: LocalStorageLayoutProps, ref: ForwardedRef<Layout>) => {
    const { defaultModel, onModelChange, factory, ...otherProps } = props
    const syncSelection = props.syncSelection ?? false

    // Track current selection by tab ID to preserve it during cross-window sync
    const selectionRef = useRef<Record<string, string>>({})
    // Track if we've called onModelChange for initial mount
    const mountedRef = useRef(false)

    const [layoutModel, setLayoutModel] = useState<Model>(() => {
      let serializedModel: string | null =
        localStorage[props.storageKey] ?? null
      if (typeof serializedModel !== 'string') {
        serializedModel = null
      }
      let model: Model | null = null
      if (serializedModel) {
        try {
          const jsonObj = JSON.parse(serializedModel)
          if (jsonObj.clearStateNonce !== props.clearStateNonce) {
            model = null
          } else {
            delete jsonObj.clearStateNonce
            model = Model.fromJson(jsonObj)
          }
        } catch {
          model = null
        }
      }
      return model ?? Model.fromJson(defaultModel)
    })

    const handleModelChange = useCallback(
      (next: Model) => {
        setLayoutModel(next)
        // Update selection ref with tab IDs
        selectionRef.current = getSelectedTabIdFromModel(next)
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const nextJson = next.toJson() as any
        nextJson.clearStateNonce = props.clearStateNonce
        localStorage[props.storageKey] = JSON.stringify(nextJson)
        onModelChange?.(next)
      },
      [props.storageKey, props.clearStateNonce, onModelChange],
    )

    const tabFactory = useCallback(
      (tab: TabNode) => factory(tab, layoutModel),
      [factory, layoutModel],
    )

    // Call onModelChange with the initial model on mount so parent has access immediately
    useEffect(() => {
      if (mountedRef.current) return
      mountedRef.current = true
      selectionRef.current = getSelectedTabIdFromModel(layoutModel)
      onModelChange?.(layoutModel)
    }, [layoutModel, onModelChange])

    // Listen for cross-window localStorage changes
    useEffect(() => {
      const handleStorage = (e: StorageEvent) => {
        if (e.key !== props.storageKey || !e.newValue) return
        try {
          let jsonObj = JSON.parse(e.newValue)
          if (jsonObj.clearStateNonce !== props.clearStateNonce) return
          delete jsonObj.clearStateNonce
          // Preserve current selection by tab ID if not syncing
          if (!syncSelection) {
            jsonObj = applySelectionByTabIdToJson(jsonObj, selectionRef.current)
          }
          const newModel = Model.fromJson(jsonObj)
          setLayoutModel(newModel)
          onModelChange?.(newModel)
        } catch {
          // Ignore invalid JSON
        }
      }
      window.addEventListener('storage', handleStorage)
      return () => window.removeEventListener('storage', handleStorage)
    }, [props.storageKey, props.clearStateNonce, onModelChange, syncSelection])

    return (
      <Layout
        {...otherProps}
        ref={ref}
        model={layoutModel}
        factory={tabFactory}
        icons={icons}
        onModelChange={handleModelChange}
      />
    )
  },
)
