import React, { PureComponent } from 'react'
import {
  OptimizedLayout,
  IOptimizedLayoutProps,
  Model,
  TabNode,
  TabSetNode,
  Action,
  IJsonModel,
  Actions,
  DockLocation,
} from '@aptre/flex-layout'
import isDeepEqual from 'lodash.isequal'

import { AbortComponent } from '@aptre/bldr-react'
import { ItState, retryWithAbort } from '@aptre/bldr'

import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'

import {
  LayoutModel,
  NavigateTabResponse,
  WatchLayoutModelRequest,
  AddTabRequest,
  AddTabResponse,
} from '@s4wave/sdk/layout/layout.pb.js'
import { LayoutHost } from '@s4wave/sdk/layout/layout_srpc.pb.js'
import {
  TabDataMap,
  jsonModelToLayoutModel,
  layoutModelToJsonModel,
  applyLocalStateToModel,
  compareLocalState,
  ILocalState,
} from '@s4wave/sdk/layout/layout.js'
import { BASE_MODEL } from './layout.js'
import { BaseLayoutContext } from './BaseLayoutContext.js'

// jsonModelToModel converts a json model to a model
//
// writes tab data to tabDataMap.
// tabDataMap should usually be empty when calling this.
function jsonModelToModel(
  modelJson: IJsonModel,
  localState?: ILocalState,
): Model {
  const model = Model.fromJson(modelJson)
  if (localState) {
    applyLocalStateToModel(model, localState)
  }
  return model
}

// ITabComponentProps are props passed to the tab component render func.
export interface ITabComponentProps {
  // tabID contains the tab node ID.
  tabID: string
  // navigate is a function to navigate the tab path.
  navigate: (path: string) => Promise<NavigateTabResponse>
  // addTab is a function to add a new tab to the layout.
  addTab: (request: AddTabRequest) => Promise<AddTabResponse>
  // tabData contains an optional Uint8Array tab data protobuf message.
  tabData?: Uint8Array
}

// IBaseLayoutProps are properties for BaseLayout.
export interface IBaseLayoutProps {
  // layoutHost is the layout host client.
  layoutHost: LayoutHost
  // renderTab renders a tab component from a node.
  renderTab: (props: ITabComponentProps) => React.ReactNode
  // flexLayoutProps are additional props to pass to FlexLayout.
  flexLayoutProps?: Partial<Omit<IOptimizedLayoutProps, 'model' | 'renderTab'>>
  // localState contains the current local state. If not provided, internal state is used.
  localState?: ILocalState
  // onLocalStateChange is called when the local state changes.
  onLocalStateChange?: (localState: ILocalState) => void
}

// IBaseLayoutState is state for BaseLayout.
interface IBaseLayoutState {
  // Model contains the model for the layout.
  // The layout is uninitialized until this is set.
  model?: Model
  // modelJson is the json representation for the layout.
  modelJson?: IJsonModel
  // protoModel contains the current protobuf model.
  protoModel?: LayoutModel
  // tabDataMap contains the tab data.
  tabDataMap?: TabDataMap
  // localState contains the current local state.
  localState: ILocalState
}

// TabDataCb is a function called when tab data changes.
type TabDataCb = (tabData: Uint8Array | undefined) => void

// BaseLayoutTabWrapper renders tab content with tab data subscription.
// This is a separate component to properly handle tab data subscriptions.
class BaseLayoutTabWrapper extends PureComponent<
  {
    tabID: string
    baseLayout: BaseLayout
  },
  { tabData: Uint8Array | undefined }
> {
  private releaseCb?: () => void
  private navigate: (path: string) => Promise<NavigateTabResponse>
  private addTab: (request: AddTabRequest) => Promise<AddTabResponse>

  constructor(props: { tabID: string; baseLayout: BaseLayout }) {
    super(props)
    const tabData = props.baseLayout.getTabData(props.tabID)
    this.state = { tabData }
    this.releaseCb = props.baseLayout.subscribeTabData(
      props.tabID,
      this.onTabData.bind(this),
    )
    this.navigate = props.baseLayout.navigateTab.bind(
      props.baseLayout,
      props.tabID,
    )
    this.addTab = props.baseLayout.addTab.bind(props.baseLayout)
  }

  componentWillUnmount() {
    if (this.releaseCb) {
      this.releaseCb()
      this.releaseCb = undefined
    }
  }

  private onTabData(tabData: Uint8Array | undefined) {
    this.setState({ tabData })
  }

  render() {
    return this.props.baseLayout.props.renderTab({
      tabID: this.props.tabID,
      navigate: this.navigate,
      addTab: this.addTab,
      tabData: this.state.tabData,
    })
  }
}

// BaseLayout is a FlexLayout displaying Tabs with ObjectViewers.
// Uses OptimizedLayout for better performance - tab content persists across layout changes.
export class BaseLayout extends AbortComponent<
  IBaseLayoutProps,
  IBaseLayoutState
> {
  static contextType = BaseLayoutContext
  declare context: React.ContextType<typeof BaseLayoutContext>

  // setLayoutModel is an iterable for the WatchLayoutModel request.
  private setLayoutModel: ItState<WatchLayoutModelRequest>
  // tabDataCbs are callbacks to call when tab data changes.
  private tabDataCbs: {
    [tabID: string]: { cbs: TabDataCb[]; prevValue: Uint8Array | undefined }
  }
  // boundOnModelChange is onModelChange bound to this
  private boundOnModelChange: (model: Model, action: Action) => void
  // boundRenderTab is renderTab bound to this
  private boundRenderTab: (node: TabNode) => React.ReactNode
  // pendingTabData holds tab data written by addTab before setState completes.
  // getTabData checks this first so new tabs can read their data synchronously.
  private pendingTabData: TabDataMap

  constructor(props: IBaseLayoutProps) {
    super(props)
    this.state = {
      localState: props.localState || { tabSetSelected: {} },
    }
    this.tabDataCbs = {}
    this.pendingTabData = {}
    this.setLayoutModel = new ItState<WatchLayoutModelRequest>(undefined, {
      mostRecentOnly: true,
    })
    this.boundOnModelChange = this.onModelChange.bind(this)
    this.boundRenderTab = this.renderTabContent.bind(this)
  }

  public componentDidMount() {
    void retryWithAbort(
      this.abortController.signal,
      this.watchLayoutModel.bind(this),
      {
        errorCb: (err) => {
          console.warn('watch LayoutModel failed', err)
        },
      },
    )
  }

  public componentDidUpdate(prevProps: IBaseLayoutProps) {
    // Update internal state if controlled localState prop changes
    if (
      this.props.localState !== undefined &&
      !isDeepEqual(prevProps.localState, this.props.localState)
    ) {
      this.setState({ localState: this.props.localState })
    }
  }

  public render() {
    if (!this.state.model) {
      return (
        <div
          role="status"
          aria-label="Loading layout"
          className="bg-background-primary flex h-full w-full flex-1 items-center justify-center p-4"
        >
          <LoadingCard
            view={{
              state: 'loading',
              title: 'Loading layout',
              detail: 'Waiting for the layout host to publish its model.',
            }}
            className="w-full max-w-sm"
          />
        </div>
      )
    }

    return (
      <OptimizedLayout
        model={this.state.model}
        renderTab={this.boundRenderTab}
        onModelChange={this.boundOnModelChange}
        realtimeResize={false}
        {...this.props.flexLayoutProps}
      />
    )
  }

  // renderTabContent renders the content for a tab using OptimizedLayout's renderTab pattern.
  private renderTabContent(node: TabNode): React.ReactNode {
    return <BaseLayoutTabWrapper tabID={node.getId()} baseLayout={this} />
  }

  // getModelTabData returns config-backed tab bytes from the live model.
  private getModelTabData(tabID: string): Uint8Array | undefined {
    const node = this.state.model?.getNodeById(tabID)
    if (typeof node !== 'object' || node === null) return undefined
    if (!(node instanceof TabNode)) return undefined
    const config: unknown = node.getConfig()
    return config instanceof Uint8Array ? config : undefined
  }

  // getTabData returns the tab data for the given tab id.
  public getTabData(tabID: string): Uint8Array | undefined {
    const pending = this.pendingTabData[tabID]
    if (pending !== undefined) return pending

    const stateData = this.state.tabDataMap?.[tabID]
    if (stateData !== undefined) return stateData

    return this.getModelTabData(tabID)
  }

  // subscribeTabData adds a callback to call when the tab data changes.
  // cb is not called until the value changes.
  // returns a release function
  public subscribeTabData(tabID: string, cb: TabDataCb): () => void {
    const list = this.tabDataCbs[tabID]
    if (list) {
      list.cbs.push(cb)
    } else {
      this.tabDataCbs[tabID] = {
        prevValue: this.getTabData(tabID),
        cbs: [cb],
      }
    }
    let removed = false
    return () => {
      if (removed) {
        return
      }
      removed = true
      const list = this.tabDataCbs[tabID]
      if (!list) {
        return
      }
      const idx = list.cbs.indexOf(cb)
      if (idx < 0) {
        return
      }
      if (list.cbs.length === 1) {
        delete this.tabDataCbs[tabID]
      } else {
        list.cbs.splice(idx, 1)
      }
    }
  }

  // navigateTab changes the path for a tab.
  public navigateTab(
    tabId: string,
    path: string,
  ): Promise<NavigateTabResponse> {
    return this.props.layoutHost.NavigateTab({ tabId, path })
  }

  // addTab adds a new tab to the layout.
  public addTab(request: AddTabRequest): Promise<AddTabResponse> {
    const model = this.state.model
    if (!model) {
      return Promise.resolve({ tabId: '' })
    }

    const tab = request.tab
    if (!tab) {
      return Promise.resolve({ tabId: '' })
    }

    // Find the target tabset
    let targetTabSetId = request.tabSetId
    if (!targetTabSetId) {
      // Find the active tabset or first available tabset
      const activeTabSet = model.getActiveTabset()
      if (activeTabSet) {
        targetTabSetId = activeTabSet.getId()
      } else {
        // Find first tabset in the model
        model.visitNodes((node) => {
          if (!targetTabSetId && node.getType() === 'tabset') {
            targetTabSetId = node.getId()
          }
        })
      }
    }

    if (!targetTabSetId) {
      console.warn('addTab: no tabset found')
      return Promise.resolve({ tabId: '' })
    }

    // Calculate insert index
    let insertIndex = -1 // -1 means add at end
    if (request.afterTabId) {
      const tabSetNode = model.getNodeById(targetTabSetId)
      if (tabSetNode && tabSetNode.getType() === 'tabset') {
        const tabSet = tabSetNode as TabSetNode
        const children = tabSet.getChildren()
        for (let i = 0; i < children.length; i++) {
          if (children[i].getId() === request.afterTabId) {
            insertIndex = i + 1
            break
          }
        }
      }
    }

    // Store tab data so it's available synchronously when the tab renders.
    // model.doAction triggers a synchronous render before setState completes,
    // so we write to pendingTabData first for getTabData to read immediately.
    const tabId = tab.id || `tab-${Date.now()}`
    if (tab.data && tab.data.length > 0) {
      this.pendingTabData[tabId] = tab.data
      const tabDataMap = {
        ...this.state.tabDataMap,
        ...this.pendingTabData,
      }
      this.setState({ tabDataMap }, () => {
        delete this.pendingTabData[tabId]
      })
    }

    // Add the tab to the FlexLayout model
    model.doAction(
      Actions.addNode(
        {
          type: 'tab',
          id: tabId,
          name: tab.name || 'New Tab',
          enableClose: tab.enableClose,
          component: 'tab-content',
        },
        targetTabSetId,
        DockLocation.CENTER,
        insertIndex,
        request.select,
      ),
    )

    // Also notify the backend (fire and forget)
    void this.props.layoutHost.AddTab(request)

    return Promise.resolve({ tabId })
  }

  // selectTab selects the tab with the given ID if it exists in the model
  public selectTab(tabID: string) {
    const tabNode = this.state.model?.getNodeById(tabID)
    if (!tabNode) return
    this.state.model?.doAction(Actions.selectTab(tabNode.getId()))
  }

  // updateTabData calls any tab data subscribers if the tab data changed.
  private updateTabData() {
    for (const [tabID, list] of Object.entries(this.tabDataCbs)) {
      const currValue = this.getTabData(tabID)
      const prevValue = list.prevValue
      list.prevValue = currValue
      if (!isDeepEqual(currValue, prevValue)) {
        for (const cb of list.cbs) {
          cb(currValue)
        }
      }
    }
  }

  // watchLayoutModel watches the layout model.
  private async watchLayoutModel(abortSignal: AbortSignal) {
    const stream = this.props.layoutHost.WatchLayoutModel(
      this.setLayoutModel.getIterable(),
      abortSignal,
    )
    for await (const resp of stream) {
      if (resp) {
        this.onLayoutModelUpdate(resp)
      }
    }
  }

  // onLayoutModelUpdate is called when the layout model is updated by the Go code.
  private onLayoutModelUpdate(layoutModel: LayoutModel) {
    if (
      this.state.protoModel &&
      layoutModel &&
      LayoutModel.equals(layoutModel, this.state.protoModel)
    ) {
      // ignore: no changes.
      return
    }
    const tabDataMap: TabDataMap = {}
    const modelJson = layoutModelToJsonModel(
      BASE_MODEL,
      tabDataMap,
      layoutModel,
    )
    const model = jsonModelToModel(modelJson, this.state.localState)
    this.setState(
      { model, modelJson, protoModel: layoutModel, tabDataMap },
      () => {
        this.updateTabData()
      },
    )
  }

  // onModelChange is called when the model is changed by the user interacting with the UI.
  // FlexLayout passes the already-updated model - we should NOT create a new Model instance
  // as that would break FlexLayout's internal drag state.
  private onModelChange(model: Model) {
    const prevModelJson = this.state.modelJson
    let modelJson = model.toJson()
    if (this.context?.onModelChange && prevModelJson) {
      const nextModelJson = this.context.onModelChange(prevModelJson, modelJson)
      if (nextModelJson == null) {
        return
      }
      modelJson = nextModelJson
    }

    const nextLocalState: ILocalState = { tabSetSelected: {} }
    const tabDataMap = {
      ...this.state.tabDataMap,
      ...this.pendingTabData,
    }
    const layoutModel = jsonModelToLayoutModel(
      modelJson,
      tabDataMap,
      nextLocalState,
    )

    // if the model is identical to before, skip updating model and protoModel.
    const localStateEqual = compareLocalState(
      this.state.localState,
      nextLocalState,
    )

    if (
      this.state.protoModel &&
      LayoutModel.equals(layoutModel, this.state.protoModel)
    ) {
      if (!localStateEqual) {
        if (this.props.localState === undefined) {
          this.setState({ localState: nextLocalState })
        }
        this.props.onLocalStateChange?.(nextLocalState)
      }
      return
    }

    // Use the model FlexLayout passed us directly - don't create a new Model instance.
    // Creating a new Model would break FlexLayout's internal drag state.
    const nextState: IBaseLayoutState = {
      model,
      modelJson,
      tabDataMap,
      protoModel: layoutModel,
      localState:
        this.props.localState !== undefined ?
          this.state.localState
        : nextLocalState,
    }

    this.setState(nextState, () => {
      this.updateTabData()
    })
    if (!localStateEqual) {
      this.props.onLocalStateChange?.(nextLocalState)
    }
    this.setLayoutModel.pushChangeEvent({
      body: { case: 'setModel', value: layoutModel },
    })
  }
}
