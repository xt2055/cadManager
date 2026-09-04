<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useThemeStore } from '@/stores/theme.store'
import { windowService } from '@/services/tauri/window.service'
import { useUserPreferenceStore } from '@/stores/user-preference.store'
import { useAuthStore } from '@/stores/auth.store'
import { readDebugMode, writeDebugMode } from '@/services/runtime-config.service'
import { appContainer } from '@/app/container'

defineOptions({
  name: 'LoginPage',
})

const router = useRouter()
const route = useRoute()
const themeStore = useThemeStore()
const preferenceStore = useUserPreferenceStore()
const authStore = useAuthStore()

const REMEMBERED_CREDENTIALS_KEY = 'cad:remembered-credentials:v1'

const account = ref('')
const password = ref('')
const rememberMe = ref(true)
const errorMessage = ref('')
const loading = ref(false)
const isSuccessAnimating = ref(false)
const isLoginEnded = ref(false)
const isLaserAnimating = ref(false)
const laserStatus = ref('认证成功 · 正在建立粒子通道')
const laserCanvas = ref<HTMLCanvasElement | null>(null)
const submitButton = ref<HTMLButtonElement | null>(null)
const debugMenuOpen = ref(false)
const debugMode = ref(false)
const debugModeLoading = ref(true)

interface RememberedCredentials {
  account: string
  password: string
}

function restoreRememberedCredentials() {
  try {
    const raw = window.localStorage.getItem(REMEMBERED_CREDENTIALS_KEY)
    if (!raw) return
    const value = JSON.parse(raw) as Partial<RememberedCredentials>
    if (typeof value.account !== 'string' || typeof value.password !== 'string') return
    account.value = value.account
    password.value = value.password
    rememberMe.value = true
  } catch {
    window.localStorage.removeItem(REMEMBERED_CREDENTIALS_KEY)
  }
}

function persistRememberedCredentials(nextAccount: string, nextPassword: string) {
  if (!rememberMe.value) {
    window.localStorage.removeItem(REMEMBERED_CREDENTIALS_KEY)
    return
  }

  const credentials: RememberedCredentials = {
    account: nextAccount,
    password: nextPassword,
  }
  window.localStorage.setItem(REMEMBERED_CREDENTIALS_KEY, JSON.stringify(credentials))
}

type LaserState = 'idle' | 'burst-warp' | 'converge' | 'tracing' | 'blooming'

interface LaserRect {
  x: number
  y: number
  w: number
  h: number
  isMain?: boolean
}

interface LaserPoint {
  x: number
  y: number
  isAnchor: boolean
}

interface LaserParticle {
  x: number
  y: number
  z: number
  pz: number
  vx: number
  vy: number
  vz: number
  targetX: number
  targetY: number
  isAnchor: boolean
  size: number
  twinkleFreq: number
  twinklePhase: number
  spring: number
  friction: number
  originX: number
  originY: number
}

const LASER_PARTICLE_COUNT = 900
let laserState: LaserState = 'idle'
let laserContext: CanvasRenderingContext2D | null = null
let laserFrameId: number | null = null
let laserTraceFrameId: number | null = null
let laserWidth = 0
let laserHeight = 0
let laserDpr = 1
let laserTime = 0
let laserLastFrameTime = 0
let laserTraceProgress = 0
let laserRects: LaserRect[] = []
let laserParticles: LaserParticle[] = []
let laserOrigin = { x: 0, y: 0 }

function createLaserParticle(originX: number, originY: number): LaserParticle {
  const particle: LaserParticle = {
    x: originX,
    y: originY,
    z: 1,
    pz: 1,
    vx: 0,
    vy: 0,
    vz: 0,
    targetX: originX,
    targetY: originY,
    isAnchor: false,
    size: Math.random() * 0.9 + 0.6,
    twinkleFreq: Math.random() * 0.15 + 0.05,
    twinklePhase: Math.random() * Math.PI * 2,
    spring: Math.random() * 0.012 + 0.024,
    friction: Math.random() * 0.04 + 0.89,
    originX,
    originY,
  }
  resetLaserParticle(particle, originX, originY)
  return particle
}

function resetLaserParticle(particle: LaserParticle, originX: number, originY: number) {
  const theta = Math.random() * Math.PI * 2
  const speed = Math.random() * 5 + 2.5
  particle.x = originX
  particle.y = originY
  particle.z = Math.random() * 1200 + 200
  particle.pz = particle.z
  particle.vx = Math.cos(theta) * speed
  particle.vy = Math.sin(theta) * speed
  particle.vz = -(Math.random() * 4 + 2.4)
  particle.originX = originX
  particle.originY = originY
}

function resizeLaserCanvas(): boolean {
  const canvas = laserCanvas.value
  if (!canvas) return false

  laserWidth = window.innerWidth
  laserHeight = window.innerHeight
  laserDpr = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.round(laserWidth * laserDpr)
  canvas.height = Math.round(laserHeight * laserDpr)
  laserContext = canvas.getContext('2d')
  laserContext?.setTransform(laserDpr, 0, 0, laserDpr, 0, 0)
  return Boolean(laserContext)
}

function handleLaserViewportResize() {
  if (!isLaserAnimating.value) return
  if (!resizeLaserCanvas()) return
  laserRects = createLaserRects()
  const targets = createLaserTargets()
  laserParticles.forEach((particle, index) => {
    const target = targets[index]
    if (!target) return
    particle.targetX = target.x
    particle.targetY = target.y
    particle.isAnchor = target.isAnchor
  })
}

function createLaserRects(): LaserRect[] {
  const padding = Math.max(18, Math.min(laserWidth, laserHeight) * 0.045)
  const main: LaserRect = {
    x: padding,
    y: padding,
    w: Math.max(120, laserWidth - padding * 2),
    h: Math.max(180, laserHeight - padding * 2),
    isMain: true,
  }
  const gap = Math.max(10, Math.min(laserWidth, laserHeight) * 0.025)
  const headerHeight = Math.min(54, Math.max(34, main.h * 0.1))
  const bodyY = main.y + headerHeight + gap
  const bodyHeight = main.h - headerHeight - gap
  const leftWidth = Math.max(86, (main.w - gap * 2) * 0.27)
  const rightWidth = Math.max(86, (main.w - gap * 2) * 0.27)
  const centerWidth = Math.max(100, main.w - leftWidth - rightWidth - gap * 2)

  return [
    main,
    { x: main.x + 12, y: main.y + 12, w: main.w - 24, h: headerHeight },
    { x: main.x + 12, y: bodyY, w: leftWidth, h: bodyHeight - 12 },
    { x: main.x + 12 + leftWidth + gap, y: bodyY, w: centerWidth, h: bodyHeight - 12 },
    { x: main.x + 12 + leftWidth + gap + centerWidth + gap, y: bodyY, w: rightWidth - 12, h: bodyHeight - 12 },
  ]
}

function appendLaserRectPoints(points: LaserPoint[], rect: LaserRect, count: number) {
  const perimeter = 2 * (rect.w + rect.h)
  for (let index = 0; index < count; index += 1) {
    const distance = (index / count) * perimeter
    let x = rect.x
    let y = rect.y
    if (distance < rect.w) {
      x = rect.x + distance
      y = rect.y
    } else if (distance < rect.w + rect.h) {
      x = rect.x + rect.w
      y = rect.y + distance - rect.w
    } else if (distance < rect.w * 2 + rect.h) {
      x = rect.x + rect.w - (distance - rect.w - rect.h)
      y = rect.y + rect.h
    } else {
      x = rect.x
      y = rect.y + rect.h - (distance - rect.w * 2 - rect.h)
    }
    points.push({ x, y, isAnchor: index % 32 === 0 })
  }
}

function createLaserTargets(): LaserPoint[] {
  const points: LaserPoint[] = []
  const main = laserRects[0]
  if (!main) return points

  appendLaserRectPoints(points, main, 430)
  laserRects.slice(1).forEach((rect) => appendLaserRectPoints(points, rect, 100))
  while (points.length < LASER_PARTICLE_COUNT) {
    points.push({
      x: main.x + Math.random() * main.w,
      y: main.y + Math.random() * main.h,
      isAnchor: false,
    })
  }
  return points.slice(0, LASER_PARTICLE_COUNT)
}

function updateLaserParticle(particle: LaserParticle, frameScale: number) {
  if (laserState === 'burst-warp') {
    particle.pz = particle.z
    particle.x += particle.vx * frameScale
    particle.y += particle.vy * frameScale
    particle.z += particle.vz * frameScale
    if (particle.z <= 15) resetLaserParticle(particle, laserOrigin.x, laserOrigin.y)
    return
  }

  if (laserState === 'converge' || laserState === 'tracing' || laserState === 'blooming') {
    const dx = particle.targetX - particle.x
    const dy = particle.targetY - particle.y
    particle.vx = (particle.vx + dx * particle.spring * frameScale) * particle.friction
    particle.vy = (particle.vy + dy * particle.spring * frameScale) * particle.friction
    particle.x += particle.vx * frameScale
    particle.y += particle.vy * frameScale
  }
}

function drawLaserParticle(particle: LaserParticle) {
  const context = laserContext
  if (!context || laserState === 'idle') return

  if (laserState === 'burst-warp') {
    const currentScale = 420 / Math.max(particle.z, 15)
    const previousScale = 420 / Math.max(particle.pz, 15)
    const currentX = laserOrigin.x + (particle.x - laserOrigin.x) * currentScale
    const currentY = laserOrigin.y + (particle.y - laserOrigin.y) * currentScale
    const previousX = laserOrigin.x + (particle.x - particle.vx - laserOrigin.x) * previousScale
    const previousY = laserOrigin.y + (particle.y - particle.vy - laserOrigin.y) * previousScale
    context.beginPath()
    context.moveTo(previousX, previousY)
    context.lineTo(currentX, currentY)
    context.strokeStyle = '#00f0ff'
    context.lineWidth = Math.min(3, Math.max(0.4, (1 - particle.z / 1200) * 3))
    context.stroke()
    return
  }

  const shimmer = 0.65 + 0.35 * Math.sin(laserTime * particle.twinkleFreq + particle.twinklePhase)
  context.beginPath()
  context.arc(particle.x, particle.y, particle.size * 2.5, 0, Math.PI * 2)
  context.fillStyle = `rgba(0, 240, 255, ${0.22 * shimmer})`
  context.fill()
  context.beginPath()
  context.arc(particle.x, particle.y, particle.size * 1.3, 0, Math.PI * 2)
  context.fillStyle = `rgba(2, 132, 199, ${0.82 * shimmer})`
  context.fill()
  context.beginPath()
  context.arc(particle.x, particle.y, particle.size * 0.6, 0, Math.PI * 2)
  context.fillStyle = `rgba(255, 255, 255, ${shimmer})`
  context.fill()

  if (particle.isAnchor) {
    const spikeLength = particle.size * 4
    context.beginPath()
    context.moveTo(particle.x - spikeLength, particle.y)
    context.lineTo(particle.x + spikeLength, particle.y)
    context.moveTo(particle.x, particle.y - spikeLength)
    context.lineTo(particle.x, particle.y + spikeLength)
    context.strokeStyle = `rgba(0, 240, 255, ${0.7 * shimmer})`
    context.lineWidth = 0.6
    context.stroke()
  }
}

function drawLaserRect(rect: LaserRect) {
  const context = laserContext
  if (!context) return

  const totalPerimeter = 2 * (rect.w + rect.h)
  const currentLength = totalPerimeter * laserTraceProgress
  const startX = rect.x
  const startY = rect.y + rect.h
  let headX = startX
  let headY = startY

  context.save()
  context.beginPath()
  const gradient = context.createLinearGradient(rect.x, rect.y + rect.h, rect.x + rect.w, rect.y)
  gradient.addColorStop(0, '#00f0ff')
  gradient.addColorStop(0.5, '#f59e0b')
  gradient.addColorStop(1, '#00f0ff')
  context.strokeStyle = gradient
  context.lineWidth = rect.isMain ? 3 : 1.6
  context.shadowColor = '#f59e0b'
  context.shadowBlur = rect.isMain ? 18 : 10
  context.moveTo(startX, startY)

  if (currentLength <= rect.h) {
    headY = startY - currentLength
    context.lineTo(headX, headY)
  } else if (currentLength <= rect.h + rect.w) {
    context.lineTo(rect.x, rect.y)
    headX = rect.x + currentLength - rect.h
    headY = rect.y
    context.lineTo(headX, headY)
  } else if (currentLength <= rect.h * 2 + rect.w) {
    context.lineTo(rect.x, rect.y)
    context.lineTo(rect.x + rect.w, rect.y)
    headX = rect.x + rect.w
    headY = rect.y + currentLength - rect.h - rect.w
    context.lineTo(headX, headY)
  } else {
    context.lineTo(rect.x, rect.y)
    context.lineTo(rect.x + rect.w, rect.y)
    context.lineTo(rect.x + rect.w, rect.y + rect.h)
    headX = rect.x + rect.w - (currentLength - rect.h * 2 - rect.w)
    headY = rect.y + rect.h
    context.lineTo(headX, headY)
  }
  context.stroke()

  if (laserTraceProgress < 1) {
    context.beginPath()
    context.arc(headX, headY, 4.5, 0, Math.PI * 2)
    context.fillStyle = '#fff'
    context.shadowColor = '#f59e0b'
    context.shadowBlur = 20
    context.fill()
    context.beginPath()
    context.arc(headX, headY, 2, 0, Math.PI * 2)
    context.fillStyle = '#f59e0b'
    context.fill()
  }
  context.restore()
}

function renderLaserFrame() {
  const context = laserContext
  if (!context || laserState === 'idle') {
    laserFrameId = null
    return
  }

  const currentTime = performance.now()
  const frameScale = laserLastFrameTime
    ? Math.min((currentTime - laserLastFrameTime) / 16.67, 2.5)
    : 1
  laserLastFrameTime = currentTime
  laserTime += frameScale
  context.clearRect(0, 0, laserWidth, laserHeight)
  laserParticles.forEach((particle) => {
    updateLaserParticle(particle, frameScale)
    drawLaserParticle(particle)
  })
  if (laserState === 'tracing' || laserState === 'blooming') {
    laserRects.forEach(drawLaserRect)
  }
  laserFrameId = requestAnimationFrame(renderLaserFrame)
}

function stopLaserRender() {
  if (laserFrameId !== null) cancelAnimationFrame(laserFrameId)
  if (laserTraceFrameId !== null) cancelAnimationFrame(laserTraceFrameId)
  laserFrameId = null
  laserTraceFrameId = null
  laserLastFrameTime = 0
  laserState = 'idle'
  laserContext?.clearRect(0, 0, laserWidth, laserHeight)
  document.documentElement.classList.remove('login-laser-active')
}

function animateLaserTrace(duration: number): Promise<void> {
  return new Promise((resolve) => {
    const startTime = performance.now()
    const step = (currentTime: number) => {
      if (laserState !== 'tracing') {
        laserTraceFrameId = null
        resolve()
        return
      }
      laserTraceProgress = Math.min(1, (currentTime - startTime) / duration)
      if (laserTraceProgress < 1) {
        laserTraceFrameId = requestAnimationFrame(step)
      } else {
        laserTraceFrameId = null
        resolve()
      }
    }
    laserTraceFrameId = requestAnimationFrame(step)
  })
}

async function playLaserLoginAnimation() {
  isLaserAnimating.value = true
  laserStatus.value = '认证成功 · 正在建立粒子通道'
  document.documentElement.classList.add('login-laser-active')
  await nextTick()
  window.addEventListener('resize', handleLaserViewportResize)

  if (!resizeLaserCanvas()) {
    await wait(1000)
    isLaserAnimating.value = false
    document.documentElement.classList.remove('login-laser-active')
    window.removeEventListener('resize', handleLaserViewportResize)
    return
  }

  const buttonRect = submitButton.value?.getBoundingClientRect()
  laserOrigin = {
    x: buttonRect ? buttonRect.left + buttonRect.width / 2 : laserWidth / 2,
    y: buttonRect ? buttonRect.top + buttonRect.height / 2 : laserHeight / 2,
  }
  laserRects = createLaserRects()
  const targets = createLaserTargets()
  laserParticles = Array.from({ length: LASER_PARTICLE_COUNT }, () => createLaserParticle(laserOrigin.x, laserOrigin.y))
  laserParticles.forEach((particle, index) => {
    const target = targets[index]
    if (!target) return
    particle.targetX = target.x
    particle.targetY = target.y
    particle.isAnchor = target.isAnchor
  })
  laserState = 'burst-warp'
  laserTime = 0
  laserLastFrameTime = 0
  laserTraceProgress = 0
  laserFrameId = requestAnimationFrame(renderLaserFrame)

  await wait(520)
  laserStatus.value = '粒子喷射 · 认证核心已锁定'
  await wait(1100)

  laserState = 'converge'
  laserStatus.value = '结构汇聚 · 工作台轮廓生成中'
  await wait(1300)

  laserState = 'tracing'
  laserStatus.value = '激光勾勒 · 正在固化工程界面'
  laserTraceProgress = 0
  await animateLaserTrace(700)

  laserState = 'blooming'
  laserStatus.value = '边框点亮 · 工作台即将载入'
  await wait(350)
  stopLaserRender()
  isLaserAnimating.value = false
  window.removeEventListener('resize', handleLaserViewportResize)
}

async function handleMinimize() {
  try {
    await windowService.minimize()
  } catch (e) {
    // 忽略
  }
}

async function handleClose() {
  try {
    await windowService.close()
  } catch (e) {
    // 忽略
  }
}

function wait(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds))
}

function waitForViewportStable(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  })
}

async function applyDebugMode() {
  try {
    await writeDebugMode(debugMode.value)
    appContainer.resetDataProvider()
    authStore.resetProvider()
  } catch (error) {
    debugMode.value = !debugMode.value
    errorMessage.value = '运行模式保存失败，请重试'
    console.error('保存运行模式配置失败', error)
  }
}

async function prepareWorkspaceWindow() {
  try {
    await windowService.setWorkspaceWindowSize()
    await waitForViewportStable()
  } catch (error) {
    console.error('登录后调整工作台窗口失败', error)
  }
}

async function handleLogin() {
  errorMessage.value = ''
  const acc = account.value.trim()
  const pwd = password.value.trim()

  if (!acc) {
    errorMessage.value = '请输入登录账号'
    return
  }
  if (!pwd) {
    errorMessage.value = '请输入密码'
    return
  }

  loading.value = true

  try {
    await wait(350)
    await authStore.login({ account: acc, password: pwd, rememberMe: rememberMe.value })
    persistRememberedCredentials(acc, pwd)
    loading.value = false
    const selectedAnimation = preferenceStore.loginAnimation
    isSuccessAnimating.value = selectedAnimation === 'burst'

    // 先在紧凑窗口内完整播放认证动画，避免主窗口放大时再次看到登录表单。
    if (selectedAnimation === 'laser') {
      await prepareWorkspaceWindow()
      await playLaserLoginAnimation()
    } else {
      await wait(850)
    }
    isLoginEnded.value = true

    if (selectedAnimation === 'burst') {
      await prepareWorkspaceWindow()
    }

    await wait(120)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : authStore.defaultPath()
    await router.push(redirect)
  } catch (error) {
    loading.value = false
    errorMessage.value = error instanceof Error ? error.message : '登录失败，请稍后重试'
  }
}

onMounted(async () => {
  themeStore.applyTheme()
  restoreRememberedCredentials()
  try {
    debugMode.value = await readDebugMode()
  } catch (error) {
    console.error('读取运行模式配置失败', error)
  } finally {
    debugModeLoading.value = false
  }
  try {
    await windowService.setLoginWindowSize()
    await waitForViewportStable()
  } catch (error) {
    console.error('初始化登录窗口失败', error)
  }
})

onBeforeUnmount(() => {
  stopLaserRender()
  window.removeEventListener('resize', handleLaserViewportResize)
})
</script>

<template>
  <div
    class="login-standalone-window"
    :class="{
      'expanding-burst': isSuccessAnimating,
      'laser-window-active': isLaserAnimating,
      'login-ended': isLoginEnded,
    }"
  >
    <canvas v-if="isLaserAnimating" ref="laserCanvas" class="laser-login-canvas" aria-hidden="true"></canvas>
    <div v-if="isLaserAnimating" class="laser-login-status">{{ laserStatus }}</div>
    <!-- 顶部可拖拽条与最小化/关闭按钮 -->
    <div v-if="!isLoginEnded" class="login-window-titlebar" data-tauri-drag-region>
      <div class="brand-mini-tag">
        <svg viewBox="0 0 24 24" fill="none" class="brand-mini-icon" aria-hidden="true">
          <rect x="2.5" y="2.5" width="19" height="19" rx="5" stroke="var(--accent)" stroke-width="2" />
          <path d="M7 16 L12 7 L17 16" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" />
        </svg>
        <span>图枢 · 安全认证</span>
      </div>
      <div class="window-mini-controls">
        <div class="debug-settings-wrap">
          <button class="win-mini-btn" type="button" title="运行设置" @click.stop="debugMenuOpen = !debugMenuOpen">
            <DemoIcon name="settings" :size="13" />
          </button>
          <div v-if="debugMenuOpen" class="debug-settings-menu" @click.stop>
            <div class="debug-settings-title">运行设置</div>
            <label class="debug-mode-option">
              <input
                v-model="debugMode"
                type="checkbox"
                :disabled="debugModeLoading"
                @change="applyDebugMode"
              />
              <span>调试模式</span>
            </label>
            <p>开启后使用本地 JSON；关闭后请求正式 API。</p>
          </div>
        </div>
        <button class="win-mini-btn" type="button" title="最小化" @click="handleMinimize">
          <DemoIcon name="minus" :size="13" />
        </button>
        <button class="win-mini-btn close" type="button" title="关闭" @click="handleClose">
          <DemoIcon name="x" :size="13" />
        </button>
      </div>
    </div>

    <!-- 登录主体内容卡片（贴合窗口大小，无多余大片空白） -->
    <div class="login-inner-container">
      <div class="login-grid-texture"></div>
      <div class="login-glow-spot"></div>

      <!-- 炫酷全屏扩散粒子光环（登录成功触发变大时播放） -->
      <div v-if="isSuccessAnimating" class="login-burst-overlay">
        <div class="burst-ring r1"></div>
        <div class="burst-ring r2"></div>
        <div class="burst-core-icon">
          <DemoIcon name="check-circle-2" :size="48" />
          <span>认证成功 · 正在载入工作台</span>
        </div>
      </div>

      <div v-if="!isLoginEnded" class="login-body-content">
        <div class="login-brand-header">
          <div class="brand-logo-icon">
            <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <rect x="2.5" y="2.5" width="19" height="19" rx="5" stroke="var(--accent)" stroke-width="2" />
              <path d="M7 16 L12 7 L17 16" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
              <circle cx="12" cy="12" r="1.6" fill="var(--accent-2)" />
            </svg>
          </div>
          <h1 class="brand-name">图枢 <span class="brand-badge">CAD·PDM</span></h1>
          <p class="brand-desc">工程图纸协同 · 全生命周期管理</p>
        </div>

        <form class="login-form" @submit.prevent="handleLogin">
          <div v-if="errorMessage" class="login-alert error">
            <DemoIcon name="alert-circle" :size="15" />
            <span>{{ errorMessage }}</span>
          </div>

          <div class="login-field">
            <label for="login-account">登录账号</label>
            <div class="input-with-icon">
              <DemoIcon name="user" :size="15" />
              <input
                id="login-account"
                v-model="account"
                type="text"
                class="inp login-input"
                placeholder="请输入工号或账号"
                autocomplete="username"
              />
            </div>
          </div>

          <div class="login-field">
            <div class="field-label-row">
              <label for="login-password">登录密码</label>
            </div>
            <div class="input-with-icon">
              <DemoIcon name="lock" :size="15" />
              <input
                id="login-password"
                v-model="password"
                type="password"
                class="inp login-input"
                placeholder="请输入密码"
                autocomplete="current-password"
              />
            </div>
          </div>

          <div class="login-options-row">
            <label class="remember-label">
              <input v-model="rememberMe" type="checkbox" />
              <span>记住账号和密码</span>
            </label>
          </div>

          <button ref="submitButton" class="btn primary submit-btn" type="submit" :disabled="loading || isSuccessAnimating || isLaserAnimating">
            <DemoIcon v-if="!loading" name="log-in" :size="15" />
            <DemoIcon v-else name="loader" :size="15" class="spin" />
            <span>{{ loading ? '身份验证中…' : '登 录 系 统' }}</span>
          </button>
        </form>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-scene {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

:global(html.login-laser-active),
:global(html.login-laser-active body),
:global(html.login-laser-active #app),
:global(html.login-laser-active .auth-window-root) {
  background: transparent !important;
  border-color: transparent !important;
  box-shadow: none !important;
}

.login-scene-ended {
  align-items: stretch;
  justify-content: stretch;
}

.laser-login-canvas {
  position: fixed;
  inset: 0;
  z-index: 80;
  width: 100vw;
  height: 100vh;
  pointer-events: none;
}

.login-standalone-window.laser-window-active {
  width: 100%;
  height: 100%;
  max-width: none;
  max-height: none;
  border: none;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
  overflow: visible;
}

.login-standalone-window.laser-window-active .login-window-titlebar,
.login-standalone-window.laser-window-active .login-inner-container {
  visibility: hidden;
  opacity: 0;
}

.laser-login-status {
  position: fixed;
  right: 24px;
  bottom: 22px;
  z-index: 81;
  padding: 7px 12px;
  border: 1px solid rgb(0 240 255 / 45%);
  border-radius: 6px;
  background: rgb(2 6 23 / 72%);
  color: #dffcff;
  box-shadow: 0 0 22px rgb(0 240 255 / 22%);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  letter-spacing: 0.3px;
  pointer-events: none;
}

.login-standalone-window {
  position: relative;
   width: min(440px, 100vw);
   height: min(600px, 100vh);
   max-width: 100vw;
   max-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--panel);
   border: 1px solid color-mix(in srgb, var(--line) 78%, transparent);
   border-radius: 14px;
   box-shadow: 0 18px 48px rgb(0 0 0 / 34%);
  overflow: hidden;
  user-select: none;
  transition: all 0.6s cubic-bezier(0.16, 1, 0.3, 1);
}

.login-standalone-window.expanding-burst {
  transform: scale(1.04);
  box-shadow: 0 0 80px var(--glow), 0 30px 90px rgb(0 0 0 / 60%);
  border-color: var(--accent);
}

.login-standalone-window.login-ended {
  width: 100%;
  height: 100%;
  max-width: none;
  max-height: none;
  border: none;
  border-radius: 0;
  box-shadow: none;
  transform: none;
}

.login-window-titlebar {
  display: flex;
  align-items: center;
  justify-content: space-between;
   height: 46px;
   padding: 0 8px 0 16px;
   background: color-mix(in srgb, var(--panel-top) 88%, var(--panel));
   border-bottom: 1px solid color-mix(in srgb, var(--line) 72%, transparent);
  -webkit-app-region: drag;
  flex: none;
  z-index: 20;
}

.brand-mini-tag {
  display: flex;
  align-items: center;
  gap: 6px;
   font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}

.brand-mini-icon {
  width: 16px;
  height: 16px;
}

.window-mini-controls {
  display: flex;
  align-items: center;
   gap: 2px;
  -webkit-app-region: no-drag;
}

.debug-settings-wrap {
  position: relative;
  -webkit-app-region: no-drag;
}

.debug-settings-menu {
  position: absolute;
  z-index: 100;
  top: 31px;
  right: -4px;
  width: 190px;
  padding: 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  box-shadow: var(--shadow);
  color: var(--text-1);
}

.debug-settings-title {
  margin-bottom: 9px;
  color: var(--text-1);
  font-size: 12px;
  font-weight: 700;
}

.debug-mode-option {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--text-2);
  cursor: pointer;
  font-size: 11.5px;
}

.debug-settings-menu p {
  margin: 8px 0 0;
  color: var(--text-3);
  font-size: 10px;
  line-height: 1.5;
}

.win-mini-btn {
  appearance: none;
  display: grid;
  place-items: center;
   width: 30px;
   height: 30px;
  padding: 0;
   border-radius: 7px;
  color: var(--text-3);
  background: transparent;
  border: none;
  outline: none;
  box-shadow: none;
  cursor: pointer;
  text-decoration: none;
  -webkit-tap-highlight-color: transparent;
  transition: all 0.2s;
}

.win-mini-btn:focus,
.win-mini-btn:focus-visible,
.win-mini-btn:active {
  border: none;
  outline: none;
  box-shadow: none;
  text-decoration: none;
}

.win-mini-btn:hover {
   background: color-mix(in srgb, var(--hover) 82%, transparent);
  color: var(--text-1);
}

.win-mini-btn.close:hover {
  background: var(--danger);
  color: #fff;
}

.login-inner-container {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
   padding: 28px 30px 20px;
  overflow: hidden;
}

.login-grid-texture {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--grid) 1px, transparent 1px);
  background-size: 32px 32px;
  opacity: 0.65;
  pointer-events: none;
}

.login-glow-spot {
  position: absolute;
  top: -80px;
  right: -80px;
  width: 260px;
  height: 260px;
  border-radius: 50%;
  background: var(--accent);
  filter: blur(80px);
  opacity: 0.22;
  pointer-events: none;
}

.login-body-content {
  position: relative;
  z-index: 5;
  display: flex;
  flex-direction: column;
  height: 100%;
}

.login-brand-header {
  text-align: center;
  margin-bottom: 18px;
}

.brand-logo-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin-bottom: 8px;
  border-radius: 12px;
  background: var(--accent-soft);
  box-shadow: 0 0 16px var(--glow);
}

.brand-logo-icon svg {
  width: 26px;
  height: 26px;
}

.brand-name {
  margin: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 900;
  letter-spacing: 0.5px;
}

.brand-badge {
  font-size: 10.5px;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  color: var(--accent);
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
}

.brand-desc {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 13px;
}

.login-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-radius: 8px;
  font-size: 11.5px;
}

.login-alert.error {
  background: rgb(239 68 68 / 15%);
  border: 1px solid var(--danger);
  color: var(--danger);
}

.login-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.login-field label {
  color: var(--text-2);
  font-size: 11.5px;
  font-weight: 500;
}

.input-with-icon {
  position: relative;
  display: flex;
  align-items: center;
}

.input-with-icon svg {
  position: absolute;
  left: 10px;
  color: var(--text-3);
  pointer-events: none;
}

.login-input {
  width: 100%;
  height: 36px;
  padding-left: 32px;
  font-size: 12.5px;
  border-radius: 8px;
}

.login-options-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: -2px;
  font-size: 11.5px;
}

.remember-label {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--text-2);
  cursor: pointer;
}

.safe-tip {
  color: var(--text-3);
  font-size: 10.5px;
}

.submit-btn {
  height: 38px;
  margin-top: 4px;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 1px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.quick-login-section {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--line);
}

.quick-title {
  color: var(--text-3);
  font-size: 10.5px;
  margin-bottom: 6px;
  text-align: center;
}

.quick-btns {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: center;
}

.quick-tag {
  padding: 3px 8px;
  border: 1px solid var(--line);
  border-radius: 5px;
  background: var(--panel-2);
  color: var(--text-2);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.2s;
}

.quick-tag:hover,
.quick-tag.current {
  border-color: var(--accent);
  color: var(--accent);
  background: var(--accent-soft);
}

.login-card-foot {
  margin-top: auto;
  padding-top: 8px;
  text-align: center;
  color: var(--text-3);
  font-size: 10px;
}

/* 炫酷登录后粒子光环展开动效 */
.login-burst-overlay {
  position: absolute;
  inset: 0;
  z-index: 50;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: var(--bg);
  backdrop-filter: blur(10px);
  animation: overlay-fade 0.3s ease-out forwards;
}

.burst-core-icon {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: var(--ok);
  z-index: 2;
  font-size: 13px;
  font-weight: 600;
  animation: core-scale 0.5s cubic-bezier(0.16, 1, 0.3, 1) forwards;
}

.burst-ring {
  position: absolute;
  width: 80px;
  height: 80px;
  border-radius: 50%;
  border: 2px solid var(--accent);
  opacity: 0.8;
  animation: ring-expand 0.65s cubic-bezier(0.16, 1, 0.3, 1) forwards;
}

.burst-ring.r2 {
  animation-delay: 0.1s;
  border-color: var(--accent-2);
}

@keyframes ring-expand {
  0% {
    transform: scale(0.3);
    opacity: 0.9;
  }
  100% {
    transform: scale(7.5);
    opacity: 0;
  }
}

@keyframes core-scale {
  0% {
    transform: scale(0.6);
    opacity: 0;
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

@keyframes overlay-fade {
  from { opacity: 0; }
  to { opacity: 1; }
}

.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
