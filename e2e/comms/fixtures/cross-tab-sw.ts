// cross-tab-sw.ts - Minimal ServiceWorker for cross-tab channel brokering tests.
//
// Imports the actual cross-tab-broker module from the bldr codebase.
// Handles "hello"/"goodbye" messages and brokers direct MessagePort channels.

import {
  isCrossTabMessage,
  handleCrossTabMessage,
} from '../../../web/bldr/cross-tab-broker.js'

declare const self: ServiceWorkerGlobalScope

self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (ev) => {
  ev.waitUntil(self.clients.claim())
})

self.addEventListener('message', (ev: ExtendableMessageEvent) => {
  if (isCrossTabMessage(ev.data)) {
    const senderId = (ev.source as Client)?.id
    if (senderId) {
      ev.waitUntil(handleCrossTabMessage(self.clients, senderId, ev.data))
    }
  }
})
