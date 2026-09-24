// Entry module for the spacewave-code plugin.
//
// The plugin provides the packages below to other plugins. Importing them here
// makes the web package build include each subpath a consumer imports.

import '@s4wave/code/CodeBlock.js'
import '@s4wave/code/markdown.js'
import '@pierre/diffs'
import '@pierre/diffs/react'
import '@shikijs/core'
import '@shikijs/engine-javascript'
import 'shiki'
import 'shiki/core'
import 'shiki/langs'
import 'shiki/themes'
