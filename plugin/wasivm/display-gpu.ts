import {
  takeDirty,
  type SceneRenderer,
  type SceneTexture,
} from './display-scene.js'

// shader draws one layer: a quad over the clip space rectangle in quad.rect
// (left, top, right, bottom), sampling the layer's premultiplied texture.
const shader = /* wgsl */ `
struct Quad { rect: vec4f }
@group(0) @binding(0) var<uniform> quad: Quad;
@group(0) @binding(1) var samp: sampler;
@group(0) @binding(2) var tex: texture_2d<f32>;

struct Out {
  @builtin(position) pos: vec4f,
  @location(0) uv: vec2f,
}

@vertex fn vs(@builtin(vertex_index) i: u32) -> Out {
  let uv = vec2f(f32(i & 1u), f32(i >> 1u));
  var o: Out;
  o.pos = vec4f(mix(quad.rect.xy, quad.rect.zw, uv), 0.0, 1.0);
  o.uv = uv;
  return o;
}

@fragment fn fs(o: Out) -> @location(0) vec4f {
  return textureSample(tex, samp, o.uv);
}
`

// quadStride is the bytes between layers' quads in the uniform buffer, the
// dynamic offset alignment every device supports.
const quadStride = 256

// GpuTexture is a texture's copy on the device and the bind group that
// samples it with the uniform buffer it was made for.
interface GpuTexture {
  texture: GPUTexture
  group: GPUBindGroup
  quads: GPUBuffer
}

// openGpuRenderer draws scenes with WebGPU, keeping each texture on the
// device. onLost is called once if the device is lost; the renderer then
// draws nothing until it is destroyed. It returns null when the browser has
// no WebGPU adapter, before touching the canvas, so a Canvas 2D renderer can
// take it.
export async function openGpuRenderer(
  canvas: HTMLCanvasElement,
  onLost: () => void,
): Promise<SceneRenderer | null> {
  // Open a device, then the canvas context.
  const adapter = await navigator.gpu.requestAdapter()
  if (!adapter) return null
  const device = await adapter.requestDevice()
  const ctx = canvas.getContext('webgpu')
  if (!(ctx instanceof GPUCanvasContext)) {
    device.destroy()
    return null
  }

  // Draw into the canvas in the screen's preferred format, and stop drawing
  // once the device is lost.
  const format = navigator.gpu.getPreferredCanvasFormat()
  ctx.configure({ device, format, alphaMode: 'opaque' })
  let lost = false
  void device.lost.then((info) => {
    lost = true
    if (info.reason !== 'destroyed') onLost()
  })

  // Build the two pipelines: opaque layers replace what is under them and
  // the rest blend as premultiplied alpha.
  const module = device.createShaderModule({ code: shader })
  const layout = device.createBindGroupLayout({
    entries: [
      {
        binding: 0,
        visibility: GPUShaderStage.VERTEX,
        buffer: { type: 'uniform', hasDynamicOffset: true },
      },
      { binding: 1, visibility: GPUShaderStage.FRAGMENT, sampler: {} },
      { binding: 2, visibility: GPUShaderStage.FRAGMENT, texture: {} },
    ],
  })
  const pipeline = (blend?: GPUBlendState) =>
    device.createRenderPipeline({
      layout: device.createPipelineLayout({ bindGroupLayouts: [layout] }),
      vertex: { module, entryPoint: 'vs' },
      fragment: { module, entryPoint: 'fs', targets: [{ format, blend }] },
      primitive: { topology: 'triangle-strip' },
    })
  const opaque = pipeline()
  const blended = pipeline({
    color: { srcFactor: 'one', dstFactor: 'one-minus-src-alpha' },
    alpha: { srcFactor: 'one', dstFactor: 'one-minus-src-alpha' },
  })
  const sampler = device.createSampler({
    magFilter: 'linear',
    minFilter: 'linear',
  })

  // quads holds one quad per layer, grown as the layer count grows.
  let quads = device.createBuffer({
    size: quadStride * 16,
    usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
  })

  // textures maps each scene texture to its copy on the device.
  let textures = new Map<SceneTexture, GpuTexture>()

  // upload returns the device copy of t, made or bound anew as needed, with
  // the areas changed since the last draw written.
  const upload = (t: SceneTexture): GpuTexture => {
    // Make the texture on first use, and bind it again when the quad
    // buffer grew.
    let g = textures.get(t)
    if (!g) {
      const texture = device.createTexture({
        size: [t.width, t.height],
        format: 'rgba8unorm',
        usage: GPUTextureUsage.TEXTURE_BINDING | GPUTextureUsage.COPY_DST,
      })
      g = { texture, group: bind(texture), quads }
      t.dirty = null
    } else if (g.quads !== quads) {
      g = { texture: g.texture, group: bind(g.texture), quads }
    }

    // Write the changed areas.
    const dirty = takeDirty(t)
    const rects = dirty ?? [{ x: 0, y: 0, width: t.width, height: t.height }]
    for (const r of rects) {
      device.queue.writeTexture(
        { texture: g.texture, origin: [r.x, r.y] },
        t.pixels,
        { offset: (r.y * t.width + r.x) * 4, bytesPerRow: t.width * 4 },
        [r.width, r.height],
      )
    }
    return g
  }

  // bind returns a bind group sampling texture with the current quads.
  const bind = (texture: GPUTexture) =>
    device.createBindGroup({
      layout,
      entries: [
        { binding: 0, resource: { buffer: quads, size: 16 } },
        { binding: 1, resource: sampler },
        { binding: 2, resource: texture.createView() },
      ],
    })

  return {
    draw(scene) {
      // Size the canvas to the screen, unless the device is gone.
      const { width, height, scale, background } = scene.screen
      if (lost || !width || !height) return
      if (canvas.width !== width * scale || canvas.height !== height * scale) {
        canvas.width = width * scale
        canvas.height = height * scale
      }

      // Grow the quad buffer to hold every layer.
      const layers = scene.drawList()
      if (quads.size < layers.length * quadStride) {
        quads.destroy()
        quads = device.createBuffer({
          size: quadStride * Math.max(16, layers.length * 2),
          usage: GPUBufferUsage.UNIFORM | GPUBufferUsage.COPY_DST,
        })
      }

      // Upload the changed areas and place each layer's quad in clip space,
      // destroying the copies of textures the scene no longer draws.
      const data = new Float32Array((layers.length * quadStride) / 4)
      const kept = new Map<SceneTexture, GpuTexture>()
      const drawn = layers.map((l, i) => {
        const g = upload(l.texture)
        kept.set(l.texture, g)
        data.set(
          [
            (l.x / width) * 2 - 1,
            1 - (l.y / height) * 2,
            ((l.x + l.width) / width) * 2 - 1,
            1 - ((l.y + l.height) / height) * 2,
          ],
          (i * quadStride) / 4,
        )
        return { opaque: l.opaque, group: g.group }
      })
      for (const [t, g] of textures) {
        if (!kept.has(t)) g.texture.destroy()
      }
      textures = kept
      if (data.length) device.queue.writeBuffer(quads, 0, data)

      // Clear to the background and draw the layers bottom to top.
      const encoder = device.createCommandEncoder()
      const pass = encoder.beginRenderPass({
        colorAttachments: [
          {
            view: ctx.getCurrentTexture().createView(),
            clearValue: {
              r: (background >>> 24) / 255,
              g: ((background >>> 16) & 255) / 255,
              b: ((background >>> 8) & 255) / 255,
              a: 1,
            },
            loadOp: 'clear',
            storeOp: 'store',
          },
        ],
      })
      drawn.forEach((l, i) => {
        pass.setPipeline(l.opaque ? opaque : blended)
        pass.setBindGroup(0, l.group, [i * quadStride])
        pass.draw(4)
      })
      pass.end()
      device.queue.submit([encoder.finish()])
    },
    destroy() {
      for (const g of textures.values()) g.texture.destroy()
      textures.clear()
      ctx.unconfigure()
      device.destroy()
    },
  }
}
