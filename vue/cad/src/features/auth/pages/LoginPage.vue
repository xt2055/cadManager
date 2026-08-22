<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useThemeStore } from '@/stores/theme.store'
import { windowService } from '@/services/tauri/window.service'

defineOptions({
  name: 'LoginPage',
})

const router = useRouter()
const route = useRoute()
const themeStore = useThemeStore()

const account = ref('zhang')
const password = ref('123456')
const rememberMe = ref(true)
const errorMessage = ref('')
const loading = ref(false)
const isSuccessAnimating = ref(false)
const isLoginEnded = ref(false)

const quickAccounts = [
  { acc: 'zhang', name: '张工 (设计主管)', role: '设计', pass: '123456' },
  { acc: 'li', name: '李工 (审核专员)', role: '审核', pass: '123456' },
  { acc: 'admin', name: '管理员 (系统管理)', role: '管理员', pass: '123456' },
]

function fillQuick(item: (typeof quickAccounts)[number]) {
  account.value = item.acc
  password.value = item.pass
  errorMessage.value = ''
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

  await wait(350)
  loading.value = false
  isSuccessAnimating.value = true

  const token = `cad_token_${Date.now()}`
  const userName = acc === 'zhang' ? '张工' : acc === 'li' ? '李工' : acc === 'admin' ? '管理员' : acc
  const userRole = acc === 'admin' ? '系统管理员' : acc === 'li' ? '审核主管' : '设计工程师'

  localStorage.setItem('cad_access_token', token)
  localStorage.setItem(
    'cad_current_user',
    JSON.stringify({
      account: acc,
      name: userName,
      role: userRole,
    }),
  )

  // 先在紧凑窗口内完整播放认证动画，避免主窗口放大时再次看到登录表单。
  await wait(850)
  isLoginEnded.value = true

  try {
    await windowService.setSizeConstraints(1100, 700)
    await windowService.setResizable(true)
    await windowService.setSize(1440, 900)
    await windowService.center()
  } catch (error) {
    console.error('登录后调整工作台窗口失败', error)
  }

  await wait(120)
  const redirect = (route.query.redirect as string) || '/dashboard'
  router.push(redirect)
}

onMounted(async () => {
  themeStore.applyTheme()
  try {
    await windowService.setSizeConstraints(360, 520)
    await windowService.setResizable(false)
    await windowService.setSize(440, 600)
    await windowService.center()
  } catch (error) {
    console.error('初始化登录窗口失败', error)
  }
})
</script>

<template>
  <div
    class="login-standalone-window"
    :class="{ 'expanding-burst': isSuccessAnimating, 'login-ended': isLoginEnded }"
  >
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
              <span>记住凭证</span>
            </label>
            <span class="safe-tip">国密算法协同通道</span>
          </div>

          <button class="btn primary submit-btn" type="submit" :disabled="loading || isSuccessAnimating">
            <DemoIcon v-if="!loading" name="log-in" :size="15" />
            <DemoIcon v-else name="loader" :size="15" class="spin" />
            <span>{{ loading ? '身份验证中…' : '登 录 系 统' }}</span>
          </button>
        </form>

        <div class="quick-login-section">
          <div class="quick-title">快捷登录通道</div>
          <div class="quick-btns">
            <button
              v-for="item in quickAccounts"
              :key="item.acc"
              type="button"
              class="quick-tag"
              :class="{ current: account === item.acc }"
              @click="fillQuick(item)"
            >
              {{ item.name }}
            </button>
          </div>
        </div>

        <div class="login-card-foot">
          <span>© 2026 图枢数字化协同工程</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-standalone-window {
  position: relative;
  width: 440px;
  height: 590px;
  max-width: 100vw;
  max-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 16px;
  box-shadow: 0 20px 60px rgb(0 0 0 / 50%);
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
  height: 38px;
  padding: 0 10px 0 14px;
  background: var(--panel-top);
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
  gap: 4px;
  -webkit-app-region: no-drag;
}

.win-mini-btn {
  appearance: none;
  display: grid;
  place-items: center;
  width: 26px;
  height: 24px;
  padding: 0;
  border-radius: 6px;
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
  background: var(--hover);
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
  padding: 24px 30px 20px;
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
