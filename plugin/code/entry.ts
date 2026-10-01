// Entry module for the spacewave-code plugin.
//
// The plugin provides the packages below to other plugins. The webPkgs entry
// for each package in bldr.star declares the subpaths consumers import.

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
