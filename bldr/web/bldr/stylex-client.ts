import { watchStylexOrder } from './stylex-order.js'

// Browser entries share the document owner; server renders have no document.
if (typeof document !== 'undefined') watchStylexOrder(document)
