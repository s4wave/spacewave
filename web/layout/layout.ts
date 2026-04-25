import { IJsonModel } from '@aptre/flex-layout'

// BASE_MODEL is the base json model for layouts.
//
// It sets the default configuration settings for flex-layout.
export const BASE_MODEL: IJsonModel = {
  borders: [],
  global: {
    tabEnableRename: false,
    tabEnableClose: true,
    tabSetEnableMaximize: true,
    splitterSize: 4,
    splitterExtra: 0,
    tabDragSpeed: 0.1,
    enableEdgeDock: true,
    tabSetEnableDivide: true,
    tabEnableRenderOnDemand: true,
    tabSetEnableDeleteWhenEmpty: true,
  },
  layout: { type: 'row', weight: 100, children: [] },
}
