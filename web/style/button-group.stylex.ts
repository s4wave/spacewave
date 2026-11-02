import * as stylex from '@stylexjs/stylex'
import {
  buttonGroupMarker,
  buttonGroupHorizontalMarker,
  buttonGroupVerticalMarker,
} from './relations.stylex.js'

/** buttonGroupControlStyles keeps joined edges and focus on each actual control. */
export const buttonGroupControlStyles = stylex.create({
  child: {
    position: {
      default: null,
      ':focus-visible': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupMarker)]: 'relative',
      },
    },
    zIndex: {
      default: null,
      ':focus-visible': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupMarker)]: 10,
      },
    },
    borderTopLeftRadius: {
      default: null,
      ':not(:first-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupHorizontalMarker)]: 0,
        [stylex.when.ancestor(':is(*) >', buttonGroupVerticalMarker)]: 0,
      },
    },
    borderBottomLeftRadius: {
      default: null,
      ':not(:first-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupHorizontalMarker)]: 0,
      },
      ':not(:last-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupVerticalMarker)]: 0,
      },
    },
    borderTopRightRadius: {
      default: null,
      ':not(:last-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupHorizontalMarker)]: 0,
      },
      ':not(:first-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupVerticalMarker)]: 0,
      },
    },
    borderBottomRightRadius: {
      default: null,
      ':not(:last-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupHorizontalMarker)]: 0,
        [stylex.when.ancestor(':is(*) >', buttonGroupVerticalMarker)]: 0,
      },
    },
    borderLeftWidth: {
      default: null,
      ':not(:first-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupHorizontalMarker)]: 0,
      },
    },
    borderTopWidth: {
      default: null,
      ':not(:first-child)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupVerticalMarker)]: 0,
      },
    },
  },
  input: {
    flexGrow: {
      default: null,
      ':is(input)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupMarker)]: 1,
      },
    },
    flexShrink: {
      default: null,
      ':is(input)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupMarker)]: 1,
      },
    },
    flexBasis: {
      default: null,
      ':is(input)': {
        default: null,
        [stylex.when.ancestor(':is(*) >', buttonGroupMarker)]: '0%',
      },
    },
  },
})
