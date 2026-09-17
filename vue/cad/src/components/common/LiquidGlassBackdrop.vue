<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { useThemeStore } from '@/stores/theme.store'
import {
  LiquidGlassRenderer,
  computeGaussianKernelByRadius,
  type RenderPassConfig,
  type UniformMap,
} from './liquid-glass/liquid-glass-renderer'
import {
  FRAGMENT_BG,
  FRAGMENT_HBLUR,
  FRAGMENT_MAIN,
  FRAGMENT_VBLUR,
  VERTEX_SHADER,
} from './liquid-glass/liquid-glass-shaders'

defineOptions({
  name: 'LiquidGlassBackdrop',
})

const theme = useThemeStore()
const host = ref<HTMLDivElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)

// 沿用原仓库 Controls.tsx 的默认参数，并按后台场景微调。
const BLUR_RADIUS = 12
const SHAPE_WIDTH_RATIO = 0.82
const SHAPE_HEIGHT_RATIO = 0.82
const MOUSE_FOLLOW = 0.08

const PASSES: RenderPassConfig[] = [
  { name: 'bgPass', vertex: VERTEX_SHADER, fragment: FRAGMENT_BG },
  { name: 'vBlurPass', vertex: VERTEX_SHADER, fragment: FRAGMENT_VBLUR, inputs: { u_prevPassTexture: 'bgPass' } },
  { name: 'hBlurPass', vertex: VERTEX_SHADER, fragment: FRAGMENT_HBLUR, inputs: { u_prevPassTexture: 'vBlurPass' } },
  {
    name: 'mainPass',
    vertex: VERTEX_SHADER,
    fragment: FRAGMENT_MAIN,
    inputs: { u_blurredBg: 'hBlurPass', u_bg: 'bgPass' },
    outputToScreen: true,
  },
]

let renderer: LiquidGlassRenderer | null = null
let dummyTexture: WebGLTexture | null = null
let raf = 0
let observer: ResizeObserver | undefined
let disposed = false
let frameCount = 0

const reducedMotion = typeof window !== 'undefined'
  ? window.matchMedia('(prefers-reduced-motion: reduce)')
  : null

const dpr = Math.min(typeof window === 'undefined' ? 1 : window.devicePixelRatio || 1, 1.5)
const blurWeights = new Float32Array(BLUR_RADIUS + 1)
const size = { w: 0, h: 0 }
const spring = { x: 0, y: 0 }
const target = { x: 0, y: 0 }

function writeBlurWeights(): void {
  const weights = computeGaussianKernelByRadius(BLUR_RADIUS)
  blurWeights.fill(0)
  weights.forEach((w, i) => { blurWeights[i] = w })
}

function resize(): void {
  const el = host.value
  if (!el || !renderer) return
  const rect = el.getBoundingClientRect()
  if (rect.width < 2 || rect.height < 2) return
  size.w = rect.width
  size.h = rect.height
  const width = Math.max(1, Math.round(size.w * dpr))
  const height = Math.max(1, Math.round(size.h * dpr))
  renderer.resize(width, height)
  renderer.setUniform('u_resolution', [width, height])

  const shapeW = size.w * SHAPE_WIDTH_RATIO
  const shapeH = size.h * SHAPE_HEIGHT_RATIO
  renderer.setUniform('u_shapeWidth', shapeW)
  renderer.setUniform('u_shapeHeight', shapeH)
  renderer.setUniform('u_shapeRadius', (Math.min(shapeW, shapeH) / 2) * 0.8)

  target.x = (size.w / 2) * dpr
  target.y = (size.h / 2) * dpr
  if (!spring.x && !spring.y) {
    spring.x = target.x
    spring.y = target.y
  }
}

function onPointerMove(event: PointerEvent): void {
  const rect = host.value?.getBoundingClientRect()
  if (!rect) return
  const nx = rect.width / 2 + (event.clientX - rect.left - rect.width / 2) * MOUSE_FOLLOW
  const ny = rect.height / 2 + (event.clientY - rect.top - rect.height / 2) * MOUSE_FOLLOW
  target.x = nx * dpr
  target.y = (rect.height - ny) * dpr
}

function renderFrame(): void {
  if (!renderer || !dummyTexture) return
  const light = theme.mode === 'light'
  const tint = light ? [0.06, 0.45, 0.62, 0.14] : [0.337, 0.878, 0.949, 0.07]

  renderer.setUniforms({
    u_dpr: dpr,
    u_mouse: [spring.x, spring.y],
    u_mouseSpring: [spring.x, spring.y],
    u_shapeRoundness: 5,
    u_mergeRate: 0.05,
    u_showShape1: 1,
    u_glareAngle: (-45 * Math.PI) / 180,
    u_blurRadius: BLUR_RADIUS,
    u_blurWeights: blurWeights,
  })

  const passUniforms: Record<string, UniformMap> = {
    bgPass: {
      u_bgType: 12,
      u_bgTexture: dummyTexture,
      u_bgTextureRatio: 1,
      u_bgTextureReady: 0,
      u_shadowExpand: 25,
      u_shadowFactor: light ? 0.06 : 0.15,
      u_shadowPosition: [0, -10],
      u_light: light ? 1 : 0,
    },
    mainPass: {
      u_tint: tint,
      u_refThickness: 20,
      u_refDistance: 0.05,
      u_refFactor: 1.4,
      u_refDispersion: 7,
      u_refFresnelRange: 30,
      u_refFresnelHardness: 0.2,
      u_refFresnelFactor: 0.2,
      u_glareRange: 30,
      u_glareHardness: 0.2,
      u_glareConvergence: 0.5,
      u_glareOppositeFactor: 0.8,
      u_glareFactor: 0.9,
      u_blurEdge: 1,
      STEP: 9,
    },
  }

  renderer.render(passUniforms)
}

function loop(): void {
  raf = requestAnimationFrame(loop)
  if (disposed || document.hidden) return
  if (reducedMotion?.matches && frameCount++ % 8 !== 0) return
  spring.x += (target.x - spring.x) * 0.08
  spring.y += (target.y - spring.y) * 0.08
  renderFrame()
}

onMounted(() => {
  const el = host.value
  const cv = canvas.value
  if (!el || !cv) return
  try {
    renderer = new LiquidGlassRenderer(cv, PASSES)
    dummyTexture = renderer.createTexture()
  } catch (error) {
    console.warn('液态玻璃背景初始化失败，已回退为纯 CSS 玻璃效果', error)
    renderer = null
    return
  }
  writeBlurWeights()
  resize()
  observer = new ResizeObserver(() => resize())
  observer.observe(el)
  window.addEventListener('pointermove', onPointerMove, { passive: true })
  raf = requestAnimationFrame(loop)
})

onBeforeUnmount(() => {
  disposed = true
  cancelAnimationFrame(raf)
  observer?.disconnect()
  window.removeEventListener('pointermove', onPointerMove)
  renderer?.dispose()
  renderer = null
  dummyTexture = null
})
</script>

<template>
  <div ref="host" class="liquid-glass-world" aria-hidden="true">
    <canvas ref="canvas"></canvas>
  </div>
</template>
