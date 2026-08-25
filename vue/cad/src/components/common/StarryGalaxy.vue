<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import * as THREE from 'three'

defineOptions({
  name: 'StarryGalaxy',
})

/**
 * 星空主题 · 璀璨银河动态背景
 * - 差速旋转螺旋银河（9 万粒子，GPU 着色器闪烁）
 * - 环绕远景星域（6 千星辰，各自独立相位呼吸）
 * - 漂浮星云团（加法混合光晕，缓慢漂移呼吸）
 * - 随机流星划破夜空
 * - 鼠标视差跟随
 */

const containerRef = ref<HTMLDivElement | null>(null)

let renderer: THREE.WebGLRenderer | null = null
let scene: THREE.Scene | null = null
let camera: THREE.PerspectiveCamera | null = null
let clock: THREE.Clock | null = null
let frameId = 0
let galaxyMaterial: THREE.ShaderMaterial | null = null
let starfieldMaterial: THREE.ShaderMaterial | null = null
let nebulae: Array<{ sprite: THREE.Sprite; baseOpacity: number; baseY: number; seed: number }> = []
let meteors: Array<{
  sprite: THREE.Sprite
  active: boolean
  dir: THREE.Vector3
  speed: number
  life: number
  nextAt: number
}> = []

const disposables: Array<{ dispose: () => void }> = []
let mouseX = 0
let mouseY = 0

/* ---------------- 着色器 ---------------- */

const GALAXY_VERTEX = /* glsl */ `
uniform float uTime;
uniform float uSize;
uniform float uPixelRatio;
attribute float aScale;
attribute float aPhase;
varying vec3 vColor;
varying float vAlpha;

void main() {
  vec3 pos = position;

  // 差速旋转：越靠近银心旋转越快，形成流动旋涡
  float radius = length(pos.xz);
  float angleOffset = (1.0 / max(radius, 0.2)) * uTime * 0.12;
  float angle = atan(pos.x, pos.z) + angleOffset;
  pos.x = sin(angle) * radius;
  pos.z = cos(angle) * radius;

  vec4 viewPosition = viewMatrix * modelMatrix * vec4(pos, 1.0);
  gl_Position = projectionMatrix * viewPosition;

  float twinkle = sin(uTime * (0.8 + aPhase * 2.2) + aPhase * 6.28318) * 0.5 + 0.5;
  vAlpha = mix(0.3, 1.0, twinkle);

  gl_PointSize = uSize * aScale * uPixelRatio * (0.65 + 0.7 * twinkle);
  gl_PointSize *= (1.0 / -viewPosition.z);
  vColor = color;
}
`

const GALAXY_FRAGMENT = /* glsl */ `
varying vec3 vColor;
varying float vAlpha;

void main() {
  float d = distance(gl_PointCoord, vec2(0.5));
  float strength = pow(1.0 - smoothstep(0.0, 0.5, d), 2.5);
  gl_FragColor = vec4(vColor, strength * vAlpha);
}
`

const STARFIELD_VERTEX = /* glsl */ `
uniform float uTime;
uniform float uSize;
uniform float uPixelRatio;
attribute float aScale;
attribute float aPhase;
varying vec3 vColor;
varying float vAlpha;

void main() {
  vec4 viewPosition = modelViewMatrix * vec4(position, 1.0);
  gl_Position = projectionMatrix * viewPosition;

  float twinkle = sin(uTime * (1.0 + aPhase * 3.0) + aPhase * 40.0) * 0.5 + 0.5;
  vAlpha = mix(0.12, 1.0, twinkle);

  gl_PointSize = uSize * aScale * uPixelRatio * (0.5 + 0.9 * twinkle);
  gl_PointSize *= (1.0 / -viewPosition.z);
  vColor = color;
}
`

const STARFIELD_FRAGMENT = GALAXY_FRAGMENT

/* ---------------- 纹理生成 ---------------- */

function makeGlowTexture(hex: string): THREE.CanvasTexture {
  const size = 128
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D
  const gradient = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
  gradient.addColorStop(0, `${hex}cc`)
  gradient.addColorStop(0.35, `${hex}55`)
  gradient.addColorStop(1, `${hex}00`)
  ctx.fillStyle = gradient
  ctx.fillRect(0, 0, size, size)
  const texture = new THREE.CanvasTexture(canvas)
  disposables.push(texture)
  return texture
}

function makeMeteorTexture(): THREE.CanvasTexture {
  const canvas = document.createElement('canvas')
  canvas.width = 256
  canvas.height = 16
  const ctx = canvas.getContext('2d') as CanvasRenderingContext2D
  const gradient = ctx.createLinearGradient(0, 0, 256, 0)
  gradient.addColorStop(0, 'rgba(255,255,255,0)')
  gradient.addColorStop(0.6, 'rgba(165,180,252,0.45)')
  gradient.addColorStop(0.92, 'rgba(230,235,255,0.95)')
  gradient.addColorStop(1, 'rgba(255,255,255,1)')
  ctx.fillStyle = gradient
  ctx.fillRect(0, 0, 256, 16)
  const texture = new THREE.CanvasTexture(canvas)
  disposables.push(texture)
  return texture
}

/* ---------------- 银河 ---------------- */

function buildGalaxy(): THREE.Points {
  const count = 90000
  const radius = 5
  const branches = 4
  const spin = 1.1
  const randomness = 0.45
  const randomnessPower = 2.8
  const insideColor = new THREE.Color('#ffd9a0')
  const midColor = new THREE.Color('#c084fc')
  const outsideColor = new THREE.Color('#4f6bff')

  const positions = new Float32Array(count * 3)
  const colors = new Float32Array(count * 3)
  const scales = new Float32Array(count)
  const phases = new Float32Array(count)

  const mixed = new THREE.Color()

  for (let i = 0; i < count; i++) {
    const i3 = i * 3
    const r = Math.random() * radius
    const branchAngle = ((i % branches) / branches) * Math.PI * 2
    const spinAngle = r * spin

    const randomX = Math.pow(Math.random(), randomnessPower) * (Math.random() < 0.5 ? 1 : -1) * randomness * r
    const randomY = Math.pow(Math.random(), randomnessPower) * (Math.random() < 0.5 ? 1 : -1) * randomness * r * 0.45
    const randomZ = Math.pow(Math.random(), randomnessPower) * (Math.random() < 0.5 ? 1 : -1) * randomness * r

    positions[i3] = Math.cos(branchAngle + spinAngle) * r + randomX
    positions[i3 + 1] = randomY
    positions[i3 + 2] = Math.sin(branchAngle + spinAngle) * r + randomZ

    const t = r / radius
    if (t < 0.45) {
      mixed.copy(insideColor).lerp(midColor, t / 0.45)
    } else {
      mixed.copy(midColor).lerp(outsideColor, (t - 0.45) / 0.55)
    }
    colors[i3] = mixed.r
    colors[i3 + 1] = mixed.g
    colors[i3 + 2] = mixed.b

    scales[i] = Math.random() < 0.08 ? 1.5 + Math.random() * 1.6 : 0.4 + Math.random() * 0.9
    phases[i] = Math.random()
  }

  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))
  geometry.setAttribute('aScale', new THREE.BufferAttribute(scales, 1))
  geometry.setAttribute('aPhase', new THREE.BufferAttribute(phases, 1))
  disposables.push(geometry)

  galaxyMaterial = new THREE.ShaderMaterial({
    vertexShader: GALAXY_VERTEX,
    fragmentShader: GALAXY_FRAGMENT,
    uniforms: {
      uTime: { value: 0 },
      uSize: { value: 26 },
      uPixelRatio: { value: Math.min(window.devicePixelRatio, 2) },
    },
    vertexColors: true,
    transparent: true,
    depthWrite: false,
    depthTest: false,
    blending: THREE.AdditiveBlending,
  })
  disposables.push(galaxyMaterial)

  return new THREE.Points(geometry, galaxyMaterial)
}

/* ---------------- 远景星域 ---------------- */

function buildStarfield(): THREE.Points {
  const count = 6000
  const positions = new Float32Array(count * 3)
  const colors = new Float32Array(count * 3)
  const scales = new Float32Array(count)
  const phases = new Float32Array(count)

  const palette = [
    new THREE.Color('#ffffff'),
    new THREE.Color('#e0e7ff'),
    new THREE.Color('#c7d2fe'),
    new THREE.Color('#bae6fd'),
    new THREE.Color('#fbcfe8'),
  ]
  const picked = new THREE.Color()

  for (let i = 0; i < count; i++) {
    const i3 = i * 3
    const r = 9 + Math.random() * 8
    const theta = Math.random() * Math.PI * 2
    const phi = Math.acos(2 * Math.random() - 1)
    positions[i3] = r * Math.sin(phi) * Math.cos(theta)
    positions[i3 + 1] = r * Math.cos(phi) * 0.6
    positions[i3 + 2] = r * Math.sin(phi) * Math.sin(theta)

    picked.copy(palette[Math.floor(Math.random() * palette.length)] ?? picked.set('#ffffff'))
    colors[i3] = picked.r
    colors[i3 + 1] = picked.g
    colors[i3 + 2] = picked.b

    scales[i] = Math.random() < 0.05 ? 2.2 + Math.random() * 2 : 0.5 + Math.random() * 1.2
    phases[i] = Math.random()
  }

  const geometry = new THREE.BufferGeometry()
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3))
  geometry.setAttribute('color', new THREE.BufferAttribute(colors, 3))
  geometry.setAttribute('aScale', new THREE.BufferAttribute(scales, 1))
  geometry.setAttribute('aPhase', new THREE.BufferAttribute(phases, 1))
  disposables.push(geometry)

  starfieldMaterial = new THREE.ShaderMaterial({
    vertexShader: STARFIELD_VERTEX,
    fragmentShader: STARFIELD_FRAGMENT,
    uniforms: {
      uTime: { value: 0 },
      uSize: { value: 15 },
      uPixelRatio: { value: Math.min(window.devicePixelRatio, 2) },
    },
    vertexColors: true,
    transparent: true,
    depthWrite: false,
    depthTest: false,
    blending: THREE.AdditiveBlending,
  })
  disposables.push(starfieldMaterial)

  return new THREE.Points(geometry, starfieldMaterial)
}

/* ---------------- 星云 ---------------- */

function buildNebulae(targetScene: THREE.Scene) {
  const configs = [
    { color: '#6366f1', pos: [-2.2, 0.6, -2.5], scale: 5.2, opacity: 0.16 },
    { color: '#c084fc', pos: [2.4, -0.3, -2.0], scale: 4.4, opacity: 0.14 },
    { color: '#38bdf8', pos: [0.4, 1.4, -3.5], scale: 5.8, opacity: 0.12 },
    { color: '#f472b6', pos: [-1.2, -1.0, -1.8], scale: 3.6, opacity: 0.12 },
    { color: '#818cf8', pos: [1.6, 0.9, -4.0], scale: 6.4, opacity: 0.1 },
  ]

  nebulae = configs.map((cfg, index) => {
    const texture = makeGlowTexture(cfg.color)
    const material = new THREE.SpriteMaterial({
      map: texture,
      transparent: true,
      opacity: cfg.opacity,
      depthWrite: false,
      depthTest: false,
      blending: THREE.AdditiveBlending,
    })
    disposables.push(material)
    const sprite = new THREE.Sprite(material)
    const [px = 0, py = 0, pz = 0] = cfg.pos
    sprite.position.set(px, py, pz)
    sprite.scale.set(cfg.scale, cfg.scale, 1)
    targetScene.add(sprite)
    return { sprite, baseOpacity: cfg.opacity, baseY: py, seed: index * 1.7 }
  })
}

/* ---------------- 流星 ---------------- */

function buildMeteors(targetScene: THREE.Scene) {
  const texture = makeMeteorTexture()
  meteors = []

  for (let i = 0; i < 4; i++) {
    const material = new THREE.SpriteMaterial({
      map: texture,
      transparent: true,
      opacity: 0,
      depthWrite: false,
      depthTest: false,
      blending: THREE.AdditiveBlending,
    })
    disposables.push(material)
    const sprite = new THREE.Sprite(material)
    sprite.scale.set(1.7, 0.055, 1)
    sprite.visible = false
    targetScene.add(sprite)
    meteors.push({
      sprite,
      active: false,
      dir: new THREE.Vector3(-0.5, -1, 0).normalize(),
      speed: 7,
      life: 0,
      nextAt: 2 + i * 2.5 + Math.random() * 5,
    })
  }
}

function spawnMeteor(meteor: (typeof meteors)[number]) {
  meteor.active = true
  meteor.life = 1
  meteor.speed = 6 + Math.random() * 5
  const x = -1 + Math.random() * 5
  const y = 1.6 + Math.random() * 2.2
  meteor.sprite.position.set(x, y, -1 - Math.random() * 2)
  const dirX = -(0.35 + Math.random() * 0.45)
  meteor.dir.set(dirX, -1, 0).normalize()
  ;(meteor.sprite.material as THREE.SpriteMaterial).rotation = Math.atan2(meteor.dir.y, meteor.dir.x)
  meteor.sprite.visible = true
}

/* ---------------- 事件 ---------------- */

function handleMouseMove(event: MouseEvent) {
  mouseX = event.clientX / window.innerWidth - 0.5
  mouseY = event.clientY / window.innerHeight - 0.5
}

function handleResize() {
  const container = containerRef.value
  if (!container || !renderer || !camera) return
  const width = container.clientWidth
  const height = container.clientHeight
  camera.aspect = width / height
  camera.updateProjectionMatrix()
  renderer.setSize(width, height)
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
}

/* ---------------- 生命周期 ---------------- */

onMounted(() => {
  const container = containerRef.value
  if (!container) return

  clock = new THREE.Clock()
  scene = new THREE.Scene()
  camera = new THREE.PerspectiveCamera(55, container.clientWidth / container.clientHeight, 0.1, 60)
  camera.position.set(0, 1.5, 3.4)
  camera.lookAt(0, 0.05, 0)

  renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true, powerPreference: 'high-performance' })
  renderer.setSize(container.clientWidth, container.clientHeight)
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2))
  container.appendChild(renderer.domElement)

  scene.add(buildGalaxy())
  scene.add(buildStarfield())
  buildNebulae(scene)
  buildMeteors(scene)

  const tick = () => {
    if (!renderer || !scene || !camera || !clock) return
    const dt = Math.min(clock.getDelta(), 0.05)
    const t = clock.elapsedTime

    if (galaxyMaterial?.uniforms.uTime) galaxyMaterial.uniforms.uTime.value = t
    if (starfieldMaterial?.uniforms.uTime) starfieldMaterial.uniforms.uTime.value = t

    // 星云缓慢呼吸与漂移
    for (const nebula of nebulae) {
      const material = nebula.sprite.material as THREE.SpriteMaterial
      material.opacity = nebula.baseOpacity * (0.7 + 0.3 * Math.sin(t * 0.35 + nebula.seed))
      nebula.sprite.position.y = nebula.baseY + Math.sin(t * 0.18 + nebula.seed * 2.1) * 0.25
    }

    // 流星状态机
    for (const meteor of meteors) {
      const material = meteor.sprite.material as THREE.SpriteMaterial
      if (!meteor.active) {
        if (t >= meteor.nextAt) spawnMeteor(meteor)
        continue
      }
      meteor.sprite.position.addScaledVector(meteor.dir, meteor.speed * dt)
      meteor.life -= dt * 1.1
      material.opacity = Math.max(meteor.life, 0) * 0.95
      if (meteor.life <= 0) {
        meteor.active = false
        meteor.sprite.visible = false
        material.opacity = 0
        meteor.nextAt = t + 4 + Math.random() * 9
      }
    }

    // 鼠标视差
    camera.position.x += (mouseX * 0.55 - camera.position.x) * 0.035
    camera.position.y += (1.5 - mouseY * 0.45 - camera.position.y) * 0.035
    camera.lookAt(0, 0.05, 0)

    renderer.render(scene, camera)
    frameId = requestAnimationFrame(tick)
  }

  tick()
  window.addEventListener('mousemove', handleMouseMove)
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frameId)
  window.removeEventListener('mousemove', handleMouseMove)
  window.removeEventListener('resize', handleResize)
  disposables.forEach((item) => item.dispose())
  if (renderer) {
    renderer.dispose()
    renderer.domElement.remove()
    renderer = null
  }
  scene = null
  camera = null
  clock = null
  nebulae = []
  meteors = []
})
</script>

<template>
  <div ref="containerRef" class="starry-galaxy"></div>
</template>
