import { transform, type Declaration } from 'lightningcss'
import postcss from 'postcss'

/** expandBackground retains every shorthand reset while printing native CSS longhands. */
export function expandBackground(value: string, colorVariable = false): string {
  // Gradient-only layers are images even when their colors remain runtime variables.
  const layers = postcss.list.comma(value)
  if (
    layers.length &&
    layers.every((layer) =>
      /^(?:repeating-)?(?:linear|radial|conic)-gradient\([\s\S]*\)$/.test(
        layer,
      ),
    )
  ) {
    const repeated = (value: string) => layers.map(() => value).join(', ')
    return `.background { background-color: transparent; background-image: ${value}; background-position-x: ${repeated('0%')}; background-position-y: ${repeated('0%')}; background-size: ${repeated('auto')}; background-repeat: ${repeated('repeat')}; background-attachment: ${repeated('scroll')}; background-origin: ${repeated('padding-box')}; background-clip: ${repeated('border-box')}; }`
  }

  // Parse known color variables through a color placeholder; other unresolved shorthands stay manual.
  let expanded: Declaration[] = []
  transform({
    filename: 'background.css',
    code: Buffer.from(
      `.background { background: ${colorVariable ? 'transparent' : value} }`,
    ),
    minify: false,
    visitor: {
      Declaration: {
        background(declaration) {
          if (declaration.property !== 'background')
            throw new Error('unresolved background shorthand')
          const backgrounds = declaration.value
          expanded = [
            { property: 'background-color', value: backgrounds.at(-1)!.color },
            {
              property: 'background-image',
              value: backgrounds.map((background) => background.image),
            },
            {
              property: 'background-position-x',
              value: backgrounds.map((background) => background.position.x),
            },
            {
              property: 'background-position-y',
              value: backgrounds.map((background) => background.position.y),
            },
            {
              property: 'background-size',
              value: backgrounds.map((background) => background.size),
            },
            {
              property: 'background-repeat',
              value: backgrounds.map((background) => background.repeat),
            },
            {
              property: 'background-attachment',
              value: backgrounds.map((background) => background.attachment),
            },
            {
              property: 'background-origin',
              value: backgrounds.map((background) => background.origin),
            },
            {
              property: 'background-clip',
              value: backgrounds.map((background) => background.clip),
              vendorPrefix: [],
            },
          ]
        },
      },
    },
  })
  if (!expanded.length) throw new Error('unresolved background shorthand')

  // Print each longhand separately so the native optimizer cannot recreate the rejected shorthand.
  const result = expanded
    .map((declaration) =>
      transform({
        filename: 'background-longhand.css',
        code: Buffer.from('.background { --background: 0 }'),
        visitor: {
          Declaration: { custom: { '--background': () => declaration } },
        },
      }).code.toString(),
    )
    .join('\n')

  // Restore the runtime color without changing the parser's resets for the other background fields.
  const css = postcss.parse(result)
  if (colorVariable)
    css.walkDecls('background-color', (declaration) => {
      declaration.value = value
    })
  return css.toString()
}
