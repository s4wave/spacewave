import { describe, expect, it } from 'vitest'

import { INIT_NOTEBOOK_OP_ID } from '../../plugin/notes/proto/init-notebook.js'
import { InitNotebookOp } from '../../plugin/notes/proto/notebook.pb.js'

import {
  buildObjectKey,
  buildWizardObjectKey,
  lookupCreateOpBuilder,
} from './create-op-builders.js'

describe('buildObjectKey', () => {
  it('uses simple name-based numbered keys', () => {
    expect(buildObjectKey('canvas/', 'Canvas')).toBe('canvas-1')
    expect(buildObjectKey('forge/cluster/', 'Forge Cluster')).toBe(
      'forge-cluster-1',
    )
  })

  it('selects the next available numbered key', () => {
    expect(buildObjectKey('canvas/', 'Canvas', ['canvas-1'])).toBe('canvas-2')
  })

  it('uses the prefix only when the name is empty', () => {
    expect(buildObjectKey('object-layout/', '')).toBe('object-layout-1')
  })

  it('uses the wizard prefix without coupling to the wizard type id', () => {
    expect(buildWizardObjectKey('Git Repository')).toBe(
      'wizard/git-repository-1',
    )
    expect(
      buildWizardObjectKey('Git Repository', ['wizard/git-repository-1']),
    ).toBe('wizard/git-repository-2')
  })
})

describe('lookupCreateOpBuilder', () => {
  it('derives notebook unixfs keys from simple notebook keys', () => {
    const builder = lookupCreateOpBuilder(INIT_NOTEBOOK_OP_ID)
    if (!builder) {
      throw new Error('expected notebook create-op builder')
    }

    const simple = InitNotebookOp.fromBinary(builder('notebook-1', 'Notebook'))
    expect(simple.notebookObjectKey).toBe('notebook-1')
    expect(simple.unixfsObjectKey).toBe('notebook-1-fs')
  })
})
