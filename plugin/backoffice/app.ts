import { defineApp } from '../../sdk/sync/app.js'

/**
 * backoffice marks the Space object that opens the Spacewave backoffice. It
 * stores no records: the viewer reads the cloud live through the viewer's
 * own Session.
 */
export const backoffice = defineApp(
  {
    id: 'spacewave/backoffice',
    version: 1,
    collections: {},
    mutations: {},
  },
  {},
)
