import { describe, expect, it } from 'vitest'

import type { ObjectViewerComponent } from '@s4wave/web/object/object.js'

import { getObjectViewersForType } from './viewers.js'

describe('getObjectViewersForType', () => {
  it('keeps the UnixFS browser ahead of the gallery in default order', () => {
    const viewers = getObjectViewersForType('unixfs/fs-node')

    expect(viewers.slice(0, 2).map((viewer) => viewer.name)).toEqual([
      'UnixFS Viewer',
      'UnixFS Gallery',
    ])
  })

  it('lets dynamic exact viewers passively win over wildcard fallback', () => {
    function DynamicViewer() {
      return null
    }
    const dynamicViewers: ObjectViewerComponent[] = [
      {
        componentID: 'plugin.custom.surface',
        typeID: 'plugin/custom',
        name: 'Plugin Surface',
        component: DynamicViewer,
      },
    ]

    const viewers = getObjectViewersForType('plugin/custom', dynamicViewers)

    expect(viewers.map((viewer) => viewer.name)).toEqual([
      'Plugin Surface',
      'Debug Viewer',
    ])
  })

  it('does not statically register notes viewers in the app shell', () => {
    expect(
      getObjectViewersForType('notes/notebook').map((viewer) => viewer.name),
    ).toEqual(['Debug Viewer'])
    expect(
      getObjectViewersForType('notes/blog').map((viewer) => viewer.name),
    ).toEqual(['Debug Viewer'])
    expect(
      getObjectViewersForType('notes/docs').map((viewer) => viewer.name),
    ).toEqual(['Debug Viewer'])
  })

  it('keeps the Drive intro wizard ahead of the generic wizard viewer', () => {
    expect(
      getObjectViewersForType('wizard/drive/intro').map(
        (viewer) => viewer.name,
      ),
    ).toEqual(['Drive Intro', 'Wizard', 'Debug Viewer'])
  })

  it('registers the Device viewer as a typed Space object surface', () => {
    expect(
      getObjectViewersForType('spacewave/device').map((viewer) => [
        viewer.name,
        viewer.category,
      ]),
    ).toEqual([
      ['Device', 'Devices'],
      ['Debug Viewer', 'Developer'],
    ])
  })

  it('registers Computers, Terminal, and Add Device as typed Device setup surfaces', () => {
    expect(
      getObjectViewersForType('spacewave/computers').map((viewer) => [
        viewer.name,
        viewer.category,
      ]),
    ).toEqual([
      ['Computers', 'Devices'],
      ['Debug Viewer', 'Developer'],
    ])
    expect(
      getObjectViewersForType('spacewave/ssh-host').map((viewer) => [
        viewer.name,
        viewer.category,
      ]),
    ).toEqual([
      ['SSH Host', 'Devices'],
      ['Debug Viewer', 'Developer'],
    ])
    expect(
      getObjectViewersForType('spacewave/terminal').map((viewer) => [
        viewer.name,
        viewer.category,
      ]),
    ).toEqual([
      ['Terminal', 'Devices'],
      ['Debug Viewer', 'Developer'],
    ])
    expect(
      getObjectViewersForType('wizard/device/add')
        .filter((viewer) => viewer.typeID.startsWith('wizard/'))
        .map((viewer) => [viewer.name, viewer.requiresObjectState]),
    ).toEqual([
      ['Add Device', false],
      ['Wizard', false],
    ])
  })

  it('lets wizard viewers open through their typed resource handle', () => {
    expect(
      getObjectViewersForType('wizard/drive/intro')
        .filter((viewer) => viewer.typeID.startsWith('wizard/'))
        .map((viewer) => [viewer.name, viewer.requiresObjectState]),
    ).toEqual([
      ['Drive Intro', false],
      ['Wizard', false],
    ])
    expect(
      getObjectViewersForType('wizard/forge/job')
        .filter((viewer) => viewer.typeID.startsWith('wizard/'))
        .map((viewer) => [viewer.name, viewer.requiresObjectState]),
    ).toEqual([
      ['Job Wizard', false],
      ['Wizard', false],
    ])
  })

  it('renders notes objects through dynamic plugin viewer registrations', () => {
    function DynamicViewer() {
      return null
    }
    const dynamicViewers: ObjectViewerComponent[] = [
      {
        componentID: 'notes.notebook.viewer',
        typeID: 'notes/notebook',
        name: 'Notebook',
        component: DynamicViewer,
      },
      {
        componentID: 'notes.blog.viewer',
        typeID: 'notes/blog',
        name: 'Blog',
        component: DynamicViewer,
      },
      {
        componentID: 'notes.docs.viewer',
        typeID: 'notes/docs',
        name: 'Documentation',
        component: DynamicViewer,
      },
    ]

    expect(
      getObjectViewersForType('notes/notebook', dynamicViewers).map(
        (viewer) => viewer.name,
      ),
    ).toEqual(['Notebook', 'Debug Viewer'])
    expect(
      getObjectViewersForType('notes/blog', dynamicViewers).map(
        (viewer) => viewer.name,
      ),
    ).toEqual(['Blog', 'Debug Viewer'])
    expect(
      getObjectViewersForType('notes/docs', dynamicViewers).map(
        (viewer) => viewer.name,
      ),
    ).toEqual(['Documentation', 'Debug Viewer'])
  })

  it('does not statically register the v86 viewer in the app shell', () => {
    expect(
      getObjectViewersForType('vm/v86').map((viewer) => viewer.name),
    ).toEqual(['Debug Viewer'])
  })

  it('renders v86 objects through dynamic plugin viewer registrations', () => {
    function DynamicViewer() {
      return null
    }
    const dynamicViewers: ObjectViewerComponent[] = [
      {
        componentID: 'vm.v86.viewer',
        typeID: 'vm/v86',
        name: 'V86',
        component: DynamicViewer,
      },
    ]

    expect(
      getObjectViewersForType('vm/v86', dynamicViewers).map(
        (viewer) => viewer.name,
      ),
    ).toEqual(['V86', 'Debug Viewer'])
  })
})
