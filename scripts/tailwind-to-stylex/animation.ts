import { transform, type Declaration } from 'lightningcss'

// expandAnimation uses the native CSS parser because StyleX drops the animation shorthand.
export function expandAnimation(value: string): string {
  // Keep the parsed longhands until printing, after the native optimizer would combine them again.
  let expanded: Declaration[] = []
  let timeline: Declaration | undefined
  const longhands = transform({
    filename: 'animation.css',
    code: Buffer.from(`.animation { animation: ${value} }`),
    minify: false,
    visitor: {
      Declaration: {
        animation(declaration) {
          if (declaration.property !== 'animation')
            throw new Error('unresolved animation shorthand')
          const animations = declaration.value
          expanded = [
            {
              property: 'animation-name',
              value: animations.map((value) => value.name),
              vendorPrefix: [],
            },
            {
              property: 'animation-duration',
              value: animations.map((value) => value.duration),
              vendorPrefix: [],
            },
            {
              property: 'animation-timing-function',
              value: animations.map((value) => value.timingFunction),
              vendorPrefix: [],
            },
            {
              property: 'animation-iteration-count',
              value: animations.map((value) => value.iterationCount),
              vendorPrefix: [],
            },
            {
              property: 'animation-direction',
              value: animations.map((value) => value.direction),
              vendorPrefix: [],
            },
            {
              property: 'animation-play-state',
              value: animations.map((value) => value.playState),
              vendorPrefix: [],
            },
            {
              property: 'animation-delay',
              value: animations.map((value) => value.delay),
              vendorPrefix: [],
            },
            {
              property: 'animation-fill-mode',
              value: animations.map((value) => value.fillMode),
              vendorPrefix: [],
            },
          ]
          timeline = {
            property: 'animation-timeline',
            value: animations.map((value) => value.timeline),
          }
        },
      },
      StyleSheetExit(sheet) {
        const rule = sheet.rules[0]
        if (rule?.type !== 'style') throw new Error('missing animation rule')
        rule.value.declarations!.declarations = expanded
        return sheet
      },
    },
  }).code.toString()

  // Serialize the reset-only timeline separately so the optimizer cannot recreate a shorthand.
  if (!timeline) throw new Error('missing animation timeline')
  const reset = transform({
    filename: 'animation-timeline.css',
    code: Buffer.from('.timeline { --animation-timeline: 0 }'),
    visitor: {
      Declaration: { custom: { '--animation-timeline': () => timeline } },
    },
  }).code.toString()
  return longhands + reset
}
