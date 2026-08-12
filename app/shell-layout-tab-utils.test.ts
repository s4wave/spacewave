import { describe, expect, it } from 'vitest'
import {
  Actions,
  DockLocation,
  Model,
  type IJsonModel,
} from '@aptre/flex-layout'

import {
  addAndSelectShellModelTab,
  addShellModelTab,
  getShellTabsetId,
} from './shell-layout-tab-utils.js'

function getModelTabIds(model: Model): string[] {
  const ids: string[] = []
  model.visitNodes((node) => {
    if (node.getType() === 'tab') ids.push(node.getId())
  })
  return ids
}

const modelJson: IJsonModel = {
  global: { tabSetEnableDeleteWhenEmpty: false },
  layout: {
    type: 'row',
    children: [
      {
        type: 'tabset',
        id: 'shell-tabset',
        children: [],
      },
    ],
  },
}

describe('addShellModelTab', () => {
  it('projects a committed tab only once in its requested pane', () => {
    const model = Model.fromJson(modelJson)
    const tab = { id: 'tab-created', name: 'Created', path: '/created' }

    addShellModelTab(model, 'shell-tabset', tab, 'shell-content')
    addShellModelTab(model, 'shell-tabset', tab, 'shell-content')

    expect(getModelTabIds(model)).toEqual(['tab-created'])
    expect(getShellTabsetId(model, tab.id)).toBe('shell-tabset')
  })

  it('moves an already projected tab to its requested pane', () => {
    const model = Model.fromJson({
      ...modelJson,
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'left-tabset',
            children: [],
          },
          {
            type: 'tabset',
            id: 'right-tabset',
            children: [
              { type: 'tab', id: 'right-anchor', name: 'Right anchor' },
            ],
          },
        ],
      },
    })
    const tab = { id: 'tab-created', name: 'Created', path: '/created' }

    addShellModelTab(model, 'left-tabset', tab, 'shell-content')
    addShellModelTab(model, 'right-tabset', tab, 'shell-content')

    expect(getModelTabIds(model)).toEqual(['right-anchor', 'tab-created'])
    expect(getShellTabsetId(model, tab.id)).toBe('right-tabset')
  })

  it('rejects a Shell Tab ID that collides with a structural node', () => {
    const model = Model.fromJson({
      ...modelJson,
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'shell-tabset',
            children: [
              { type: 'tab', id: 'shell-anchor', name: 'Shell anchor' },
            ],
          },
          {
            type: 'tabset',
            id: 'tab-created',
            children: [
              { type: 'tab', id: 'right-anchor', name: 'Right anchor' },
            ],
          },
        ],
      },
    })

    expect(() =>
      addShellModelTab(
        model,
        'shell-tabset',
        { id: 'tab-created', name: 'Created', path: '/created' },
        'shell-content',
      ),
    ).toThrow('Shell tab ID collides with tabset node: tab-created')
  })

  it('collapses a closed grid pane before projecting a committed tab twice', () => {
    const model = Model.fromJson({
      ...modelJson,
      layout: {
        type: 'row',
        children: [
          {
            type: 'tabset',
            id: 'shell-tabset',
            children: [
              { type: 'tab', id: 'left-tab', name: 'Left' },
              { type: 'tab', id: 'right-tab', name: 'Right' },
            ],
          },
        ],
      },
    })

    model.doAction(
      Actions.moveNode('right-tab', 'shell-tabset', DockLocation.RIGHT, -1),
    )
    const rightTabsetId = model.getNodeById('right-tab')?.getParent()?.getId()
    expect(rightTabsetId).toBeDefined()

    model.doAction(
      Actions.updateModelAttributes({ tabSetEnableDeleteWhenEmpty: true }),
    )
    model.doAction(Actions.deleteTab('right-tab'))

    expect(model.getNodeById(rightTabsetId!)).toBeUndefined()
    const tab = { id: 'tab-created', name: 'Created', path: '/created' }

    addShellModelTab(model, 'shell-tabset', tab, 'shell-content')
    addAndSelectShellModelTab(model, 'shell-tabset', tab, 'shell-content')

    expect(getModelTabIds(model)).toEqual(['left-tab', 'tab-created'])
    expect(getShellTabsetId(model, tab.id)).toBe('shell-tabset')
  })
})
