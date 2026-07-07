import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import {
  beginDriveSpaceOpenRegion,
  driveSpaceOpenTaskName,
  endDriveSpaceOpenRegion,
  endDriveSpaceOpenTrace,
  resetDriveSpaceOpenTraceForTest,
  startDriveSpaceOpenTrace,
} from './drive-space-open-trace.js'

describe('drive space open trace', () => {
  beforeEach(() => {
    resetDriveSpaceOpenTraceForTest()
    performance.clearMarks()
    performance.clearMeasures()
  })

  afterEach(() => {
    resetDriveSpaceOpenTraceForTest()
    performance.clearMarks()
    performance.clearMeasures()
  })

  it('records one task with nested hot-path regions', () => {
    startDriveSpaceOpenTrace({
      sessionIndex: 1,
      sharedObjectId: 'space-1',
      spaceName: 'Drive',
      source: 'local',
    })

    endDriveSpaceOpenRegion('space-1', 'mount')
    beginDriveSpaceOpenRegion('space-1', 'so-fetch')
    endDriveSpaceOpenRegion('space-1', 'so-fetch')
    beginDriveSpaceOpenRegion('space-1', 'world-open')
    endDriveSpaceOpenRegion('space-1', 'world-open')
    beginDriveSpaceOpenRegion('space-1', 'first-listing-render')
    endDriveSpaceOpenRegion('space-1', 'first-listing-render')
    endDriveSpaceOpenTrace('space-1')
    const markNames = performance
      .getEntriesByType('mark')
      .map((entry) => entry.name)

    expect(markNames).toEqual(
      expect.arrayContaining([
        `spacewave.trace.task.${driveSpaceOpenTaskName}.1.start`,
        `spacewave.trace.task.${driveSpaceOpenTaskName}.1.end`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/mount.1.start`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/mount.1.end`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/so-fetch.1.start`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/so-fetch.1.end`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/world-open.1.start`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/world-open.1.end`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/first-listing-render.1.start`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/first-listing-render.1.end`,
      ]),
    )
    const measureNames = performance
      .getEntriesByType('measure')
      .map((entry) => entry.name)
    expect(measureNames).toEqual(
      expect.arrayContaining([
        `spacewave.trace.task.${driveSpaceOpenTaskName}`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/mount`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/so-fetch`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/world-open`,
        `spacewave.trace.region.${driveSpaceOpenTaskName}/first-listing-render`,
      ]),
    )
  })

  it('replaces an active task for the same shared object', () => {
    startDriveSpaceOpenTrace({
      sessionIndex: 1,
      sharedObjectId: 'space-1',
      spaceName: 'Drive',
    })
    startDriveSpaceOpenTrace({
      sessionIndex: 1,
      sharedObjectId: 'space-1',
      spaceName: 'Drive again',
    })
    endDriveSpaceOpenTrace('space-1')
    const markNames = performance
      .getEntriesByType('mark')
      .map((entry) => entry.name)

    expect(markNames).toEqual(
      expect.arrayContaining([
        `spacewave.trace.task.${driveSpaceOpenTaskName}.1.end`,
        `spacewave.trace.task.${driveSpaceOpenTaskName}.2.start`,
        `spacewave.trace.task.${driveSpaceOpenTaskName}.2.end`,
      ]),
    )
  })
})
