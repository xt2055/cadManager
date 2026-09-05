<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import * as THREE from 'three'
import { useThemeStore } from '@/stores/theme.store'
const theme = useThemeStore()

const host = ref<HTMLDivElement>()
const unavailable = ref(false)
let renderer: THREE.WebGLRenderer | undefined
let observer: ResizeObserver | undefined
let frame = 0
let scene: THREE.Scene | undefined
const reduced = window.matchMedia('(prefers-reduced-motion: reduce)')

onMounted(() => {
  if (!host.value) return
  try { renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true, powerPreference: 'low-power' }) }
  catch { unavailable.value = true; return }
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.5))
  renderer.setClearColor(0x000000, 0)
  host.value.appendChild(renderer.domElement)
  scene = new THREE.Scene()
  const camera = new THREE.PerspectiveCamera(36, 1, 0.1, 100)
  camera.position.set(8, 6, 13)
  camera.lookAt(0, 0, 0)
  scene.add(new THREE.AmbientLight(0x88bbdd, 2))
  const key = new THREE.DirectionalLight(0xc4f7ff, 5)
  key.position.set(2, 7, 5); scene.add(key)
  const rim = new THREE.PointLight(0xff9a3d, 100)
  rim.position.set(-4, 2, -2); scene.add(rim)
  const machine = new THREE.Group()
  machine.rotation.z = -0.3
  scene.add(machine)
  const blue = new THREE.MeshStandardMaterial({ color: 0x15485f, metalness: 0.8, roughness: 0.28 })
  const steel = new THREE.MeshStandardMaterial({ color: 0xbdd8e4, metalness: 0.95, roughness: 0.16 })
  const orange = new THREE.MeshStandardMaterial({ color: 0xff952f, metalness: 0.65, roughness: 0.35 })
  const glow = new THREE.LineBasicMaterial({ color: 0x43ddf1, transparent: true, opacity: 0.22 })
  function cylinder(radius: number, length: number, x: number, material: THREE.Material, parent = machine) {
    const geometry = new THREE.CylinderGeometry(radius, radius, length, 40)
    geometry.rotateZ(Math.PI / 2)
    const mesh = new THREE.Mesh(geometry, material)
    mesh.position.x = x; parent.add(mesh)
    const outline = new THREE.LineSegments(new THREE.EdgesGeometry(geometry), glow)
    mesh.add(outline)
    return mesh
  }
  cylinder(0.7, 3.8, -1.2, blue)
  cylinder(0.85, 0.3, -3.15, steel)
  cylinder(0.86, 0.42, 0.75, blue)
  cylinder(0.71, 0.12, 0.99, orange)
  cylinder(0.37, 0.2, -3.4, steel)
  const piston = new THREE.Group(); machine.add(piston)
  cylinder(0.28, 3.4, 0.6, steel, piston)
  cylinder(0.42, 0.2, 2.34, orange, piston)
  const eye = new THREE.Mesh(new THREE.TorusGeometry(0.4, 0.17, 16, 40), steel)
  eye.position.x = 2.7; piston.add(eye)
  for (let i = 0; i < 4; i++) {
    const angle = Math.PI / 4 + i * Math.PI / 2
    const rod = cylinder(0.045, 3.8, -1.2, steel)
    rod.position.y = Math.cos(angle) * 0.76; rod.position.z = Math.sin(angle) * 0.76
    for (const x of [-3.34, 1.03]) {
      const bolt = cylinder(0.105, 0.16, x, steel)
      bolt.position.y = rod.position.y; bolt.position.z = rod.position.z
    }
  }
  for (const x of [-2.5, 0.2]) {
    const port = cylinder(0.15, 0.45, x, orange)
    port.rotation.z = Math.PI / 2; port.position.y = 0.78
  }
  const grid = new THREE.GridHelper(24, 32, 0x1d647b, 0x113348)
  grid.position.y = -1.8; scene.add(grid)
  observer = new ResizeObserver(() => {
    if (!host.value || !renderer) return
    const { width, height } = host.value.getBoundingClientRect()
    renderer.setSize(width, height)
    camera.aspect = width / Math.max(height, 1); camera.updateProjectionMatrix()
  })
  observer.observe(host.value)
  let last = 0
  let strokeStart = 0
  let lastTrigger = 0
  const heroCamera = new THREE.Vector3(3, 3, 12)
  const ease = (v: number) => { const x = THREE.MathUtils.clamp(v, 0, 1); return x * x * (3 - 2 * x) }
  const animate = (time: number) => {
    frame = requestAnimationFrame(animate)
    if (document.hidden || time - last < (theme.hydraulicPhase === 'idle' ? 33 : 16)) return
    last = time
    const t = reduced.matches ? 0 : time / 1000
    piston.position.x = 0.45 + Math.sin(t * 0.65) * 0.9
    machine.rotation.y = Math.sin(t * 0.15) * 0.18
    machine.rotation.z = -0.3
    machine.position.set(0, 0, 0)
    camera.position.set(8, 6, 13)
    camera.lookAt(0, 0, 0)
    const lightMode = theme.mode === 'light'
    blue.color.setHex(lightMode ? 0x2877a0 : 0x15485f)
    glow.color.setHex(lightMode ? 0x1269b0 : 0x43ddf1)
    glow.opacity = lightMode ? 0.32 : 0.22
    key.intensity = lightMode ? 3.5 : 5
    rim.intensity = 100
    if (theme.hydraulicPhase !== 'idle' && !reduced.matches) {
      if (lastTrigger !== theme.hydraulicStartedAt) {
        strokeStart = piston.position.x
        lastTrigger = theme.hydraulicStartedAt
      }
      const elapsed = time - theme.hydraulicStartedAt
      // 缓慢回缩蓄力，随后快速冲程；与 UI 的冲击时刻共用一条时间轴。
      const retract = ease(elapsed / 1250)
      const thrust = 1 - Math.pow(1 - THREE.MathUtils.clamp((elapsed - 1420) / 180, 0, 1), 4)
      const settle = ease((elapsed - 1950) / 950)
      piston.position.x = THREE.MathUtils.lerp(strokeStart, -0.9, retract) + thrust * 2.8
      piston.position.x = THREE.MathUtils.lerp(piston.position.x, 0.45 + Math.sin(t * 0.65) * 0.9, settle)
      const hero = ease(elapsed / 420) * (1 - settle)
      machine.rotation.z = THREE.MathUtils.lerp(-0.3, 0, hero)
      machine.rotation.y *= 1 - hero
      camera.position.lerp(heroCamera, hero)
      const kickTime = Math.max(0, elapsed - 1500)
      const kick = elapsed >= 1500 ? Math.exp(-kickTime / 110) : 0
      machine.position.x = -0.17 * kick
      camera.position.y += Math.sin(kickTime * 0.08) * 0.12 * kick
      camera.lookAt(0, 0, 0)
      rim.intensity = 100 + 140 * retract * (1 - settle)
      key.intensity += 4 * kick
    }
    renderer!.render(scene!, camera)
  }
  frame = requestAnimationFrame(animate)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frame); observer?.disconnect()
  const geometries = new Set<THREE.BufferGeometry>()
  const materials = new Set<THREE.Material>()
  scene?.traverse((object) => {
    const mesh = object as THREE.Mesh
    if (mesh.geometry) geometries.add(mesh.geometry)
    if (mesh.material) (Array.isArray(mesh.material) ? mesh.material : [mesh.material]).forEach(m => materials.add(m))
  })
  geometries.forEach(g => g.dispose()); materials.forEach(m => m.dispose())
  renderer?.dispose(); renderer?.domElement.remove()
})
</script>

<template>
  <div class="juli-world" :class="{ 'is-hydraulic-transition': theme.hydraulicPhase !== 'idle' }" aria-hidden="true">
    <div ref="host" class="juli-machine"></div>
    <div class="juli-world-grid"></div>
    <div class="juli-orbit"></div>
    <div class="juli-telemetry">JULI HYDRAULICS <span>／</span> DIGITAL ENGINEERING<br>液压传动 · 精密制造 · 数字协同</div>
    <div class="juli-world-caption">{{ unavailable ? 'HYDRAULIC ENGINEERING / BLUEPRINT' : 'HYDRAULIC CYLINDER / MOTION STUDY' }}<br><span>三维运动示意 · 非实时设备数据</span></div>
  </div>
</template>
