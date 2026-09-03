import { definePlugin } from '@s4wave/sdk/plugin/index.js'
import { ObjectTypeVisibility } from '@s4wave/sdk/objecttype/registry/registry.pb.js'
import { ViewerSurface } from '@s4wave/sdk/viewer/registry/registry.pb.js'

import { CounterHandler } from './counter-handler.js'
import { CounterTypeID } from './counter.js'
import { CounterResourceServiceDefinition } from './counter_srpc.pb.js'

export default definePlugin({
  objectTypes: [
    {
      typeId: CounterTypeID,
      metadata: {
        displayName: 'Counter',
        iconName: 'hash',
        visibility: ObjectTypeVisibility.VISIBLE,
      },
      service: CounterResourceServiceDefinition,
      create: ({ engine, objectKey }) => new CounterHandler(engine, objectKey),
    },
  ],
  viewers: [
    {
      typeId: CounterTypeID,
      componentId: 'example.counter.viewer',
      viewerName: 'Counter',
      scriptPath: './sdk/plugin/testdata/CounterViewer.tsx',
      surface: ViewerSurface.WEB,
    },
  ],
})
