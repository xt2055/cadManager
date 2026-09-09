<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'

import { appConfig } from '@/app/app.config'
import { editingService } from '@/app/container'
import DemoIcon from '@/components/common/DemoIcon.vue'
import type { ActiveEditSessionInfo } from '@/types/application.types'
import { checkForUpdates, resolveUpdateDownloadUrl, type UpdateCheckResult } from '@/services/update.service'
import {
  getSavedUpdateServer,
  hasCustomUpdateServer,
  normalizeServerInput,
  probeServer,
  saveUpdateServer,
  clearUpdateServer,
  type ProbeResult,
} from '@/services/server-address.service'
import { useThemeStore } from '@/stores/theme.store'
import { useUiStore } from '@/stores/ui.store'
import type { ThemeMode, ThemeSkin } from '@/types/theme.types'
import { useUserPreferenceStore, type LoginAnimation } from '@/stores/user-preference.store'
import { useSystemStatusStore } from '@/stores/system-status.store'
import { isTauri } from '@tauri-apps/api/core'
import { isRegistered } from '@tauri-apps/plugin-deep-link'

defineOptions({
  name: 'SettingsPage',
})

type SettingsTab = 'appearance' | 'workbench' | 'collab' | 'update'

const activeTab = ref<SettingsTab>('appearance')

const themeStore = useThemeStore()
const uiStore = useUiStore()
const preferenceStore = useUserPreferenceStore()
const systemStore = useSystemStatusStore()
const checkingUpdate = ref(false)
const updateResult = ref<UpdateCheckResult | null>(null)
const updateError = ref('')
const serverInput = ref(getSavedUpdateServer())
const serverSaved = ref(hasCustomUpdateServer())
const probing = ref(false)
const probeResult = ref<ProbeResult | null>(null)
const protocolRegistered = ref<boolean | null>(null)
const activeSessions = ref<ActiveEditSessionInfo[]>([])
const loadingSessions = ref(false)
const closingSessionId = ref<string | null>(null)
let sessionPollTimer: number | null = null

// 本地偏好开关状态（保留双向交互）
const autoFocusLibrary = ref(true)
const forkBranchProtect = ref(true)

const smbStatus = computed(() => systemStore.status?.smb)
const caxaStatus = computed(() => systemStore.status?.caxa)

function refreshSystemStatus() {
  void systemStore.fetchStatus()
  void loadGlobalActiveSessions()
}

async function loadGlobalActiveSessions() {
  loadingSessions.value = true
  try {
    const list = await editingService.listSessions()
    activeSessions.value = list
  } catch {
    activeSessions.value = []
  } finally {
    loadingSessions.value = false
  }
}

async function handleCloseSession(session: ActiveEditSessionInfo) {
  closingSessionId.value = session.id
  try {
    await editingService.closeSession(session.id)
    uiStore.toast(`已成功解锁图纸「${session.fileName || session.drawingNo}」`, 'ok')
    await loadGlobalActiveSessions()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '解锁失败', 'warn')
  } finally {
    closingSessionId.value = null
  }
}

async function checkProtocolRegistration() {
  if (!isTauri()) return
  try {
    protocolRegistered.value = await isRegistered('cadguanliq')
  } catch {
    protocolRegistered.value = false
  }
}

onMounted(() => {
  refreshSystemStatus()
  void checkProtocolRegistration()
  sessionPollTimer = window.setInterval(() => {
    void loadGlobalActiveSessions()
  }, 10_000)
})

onBeforeUnmount(() => {
  if (sessionPollTimer) {
    window.clearInterval(sessionPollTimer)
    sessionPollTimer = null
  }
})

interface SkinMeta {
  key: ThemeSkin
  title: string
  subtitle: string
  desc: string
  tag: string
  colors: {
    bg: string
    panel: string
    accent: string
    accent2: string
  }
}

const skins: SkinMeta[] = [
  {
    key: 'classic',
    title: '经典工程',
    subtitle: 'Classic Engineering',
    desc: '清晰明快、稳健高效的标准工程 CAD 界面风格',
    tag: '默认推荐',
    colors: { bg: '#0b1320', panel: '#111c2e', accent: '#3b82f6', accent2: '#60a5fa' },
  },
  {
    key: 'juli',
    title: '巨力数字工场',
    subtitle: 'Juli Digital Plant',
    desc: '三维液压机电美学、CAD 蓝图线条描绘与机械微光转场',
    tag: '液压定制',
    colors: { bg: '#030c16', panel: '#0a1d2b', accent: '#44e1f2', accent2: '#ffac57' },
  },
  {
    key: 'tech',
    title: '未来科技',
    subtitle: 'Cyber Tech Grid',
    desc: '带有工业网格光影与微光流光的未来感极客设计风格',
    tag: '高对比微光',
    colors: { bg: '#04070e', panel: '#0a1322', accent: '#22d3ee', accent2: '#818cf8' },
  },
  {
    key: 'elegant',
    title: '舒适护眼',
    subtitle: 'Warm Eye Care',
    desc: '温暖柔和、降低视觉刺激的舒适防眩光配色体系',
    tag: '温润防眩',
    colors: { bg: '#181512', panel: '#221e1a', accent: '#d97706', accent2: '#f59e0b' },
  },
  {
    key: 'sage',
    title: '豆沙超级护眼',
    subtitle: 'Paper Sage Green',
    desc: '豆沙绿纸张质感，极低蓝光与对比度，全天候长时审图护眼',
    tag: '极低蓝光',
    colors: { bg: '#151b16', panel: '#1a221c', accent: '#8ab892', accent2: '#a5cfab' },
  },
  {
    key: 'starry',
    title: '深邃星空',
    subtitle: 'Deep Galaxy Cosmos',
    desc: '深邃深蓝与紫青星芒交织的现代高对比度视觉体系',
    tag: '深空星芒',
    colors: { bg: '#0b0f19', panel: '#111827', accent: '#818cf8', accent2: '#c084fc' },
  },
  {
    key: 'bamboo',
    title: '东方墨竹',
    subtitle: 'Oriental Bamboo',
    desc: '清雅苍翠、融合东方美学与自然沉静的墨绿质感风格',
    tag: '苍翠沉静',
    colors: { bg: '#0c1410', panel: '#131d17', accent: '#2dd4bf', accent2: '#34d399' },
  },
]

const modes: Array<{ key: ThemeMode; title: string; desc: string; icon: string }> = [
  {
    key: 'dark',
    title: '深色模式',
    desc: '降低屏幕刺眼度，适合长时间 CAD 审图与精密绘图作业',
    icon: 'moon',
  },
  {
    key: 'light',
    title: '浅色模式',
    desc: '明亮通透清晰，适合强光环境、纸质打印对比或文档审阅',
    icon: 'sun',
  },
]

const loginAnimations: Array<{ key: LoginAnimation; title: string; desc: string; icon: string }> = [
  {
    key: 'laser',
    title: '激光粒子勾勒',
    desc: '粒子从登录按钮喷射汇聚，从左下角线框勾勒工作台轮廓并点亮边框。',
    icon: 'scan-line',
  },
  {
    key: 'burst',
    title: '粒子光环扩散',
    desc: '认证成功后瞬间呈现扩散青蓝光环与核心状态，快速进入工作台。',
    icon: 'sparkles',
  },
]

function selectSkin(skin: ThemeSkin) {
  themeStore.setSkin(skin)
  uiStore.toast(`已切换为「${skins.find((s) => s.key === skin)?.title}」`, 'ok')
}

function selectMode(mode: ThemeMode) {
  themeStore.setMode(mode)
  uiStore.toast(`已切换为「${mode === 'dark' ? '深色模式' : '浅色模式'}」`, 'ok')
}

function selectLoginAnimation(animation: LoginAnimation) {
  preferenceStore.setLoginAnimation(animation)
  const title = loginAnimations.find((item) => item.key === animation)?.title ?? '登录动画'
  uiStore.toast(`下次登录将使用「${title}」`, 'ok')
}

function resetToDefault() {
  themeStore.setSkin('classic')
  themeStore.setMode('dark')
  preferenceStore.setLoginAnimation('laser')
  autoFocusLibrary.value = true
  forkBranchProtect.value = true
  uiStore.toast('已恢复默认设置（经典主题 + 深色模式 + 激光勾勒动画）', 'ok')
}

async function handleCheckForUpdates() {
  if (checkingUpdate.value) return
  checkingUpdate.value = true
  updateError.value = ''
  try {
    const custom = serverInput.value.trim()
    if (custom) {
      const normalized = normalizeServerInput(custom)
      if (!normalized) {
        updateError.value = '服务器地址格式无效（示例：192.168.1.50:8080）'
        uiStore.toast(updateError.value, 'warn')
        return
      }
      saveUpdateServer(normalized)
      serverSaved.value = true
      serverInput.value = normalized
    }
    updateResult.value = await checkForUpdates(custom || undefined)
    uiStore.toast(
      updateResult.value.updateAvailable ? `发现新版本 v${updateResult.value.latestVersion}` : '当前已是最新版本',
      updateResult.value.updateAvailable ? 'info' : 'ok',
    )
  } catch (error) {
    updateResult.value = null
    updateError.value = error instanceof Error ? error.message : '检查更新失败，请稍后重试'
    uiStore.toast(updateError.value, 'warn')
  } finally {
    checkingUpdate.value = false
  }
}

async function handleProbeServer() {
  if (probing.value) return
  const normalized = normalizeServerInput(serverInput.value)
  if (!normalized) {
    uiStore.toast('请输入有效的服务器地址（示例：192.168.1.50:8080）', 'warn')
    return
  }
  serverInput.value = normalized
  probing.value = true
  probeResult.value = null
  try {
    probeResult.value = await probeServer(normalized)
  } finally {
    probing.value = false
  }
}

function handleSaveServer() {
  const normalized = normalizeServerInput(serverInput.value)
  if (!normalized) {
    uiStore.toast('地址格式无效（示例：192.168.1.50:8080）', 'warn')
    return
  }
  serverInput.value = normalized
  saveUpdateServer(normalized)
  serverSaved.value = true
  uiStore.toast('更新服务器地址已记住，下次启动将优先从该地址检查更新', 'ok')
}

function handleClearServer() {
  clearUpdateServer()
  serverSaved.value = false
  serverInput.value = ''
  probeResult.value = null
  uiStore.toast('已恢复默认更新服务器地址', 'ok')
}

function handleDownloadUpdate() {
  const url = updateResult.value ? resolveUpdateDownloadUrl(updateResult.value) : ''
  if (!url) {
    uiStore.toast('该更新未提供下载地址', 'warn')
    return
  }
  window.open(url, '_blank')
}
</script>

<template>
  <div class="page settings-page">
    <!-- 顶部标题与快速重置 -->
    <header class="settings-head-bar">
      <div class="settings-head-meta">
        <div class="settings-title-row">
          <div class="settings-badge-icon">
            <DemoIcon name="sliders" :size="18" />
          </div>
          <h1 class="settings-title">系统设置</h1>
          <span class="settings-version-tag">CAD PDM · v{{ appConfig.version }}</span>
        </div>
        <p class="settings-subtitle">个性化工作空间偏好、外观微调、桌面协同引擎及系统更新维护</p>
      </div>

      <div class="settings-head-actions">
        <button class="btn secondary reset-btn" type="button" @click="resetToDefault" title="重置全部外观与通用参数为默认值">
          <DemoIcon name="rotate-ccw" :size="13" />
          <span>恢复默认</span>
        </button>
      </div>
    </header>

    <!-- 分类 Segmented Tabs 导航栏 -->
    <nav class="settings-nav-tabs" role="tablist">
      <button
        class="nav-tab-btn"
        :class="{ active: activeTab === 'appearance' }"
        role="tab"
        type="button"
        @click="activeTab = 'appearance'"
      >
        <DemoIcon name="palette" :size="15" />
        <span>外观个性化</span>
        <span class="tab-indicator-count">{{ skins.length }} 款</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: activeTab === 'workbench' }"
        role="tab"
        type="button"
        @click="activeTab = 'workbench'"
      >
        <DemoIcon name="layout" :size="15" />
        <span>工作台偏好</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: activeTab === 'collab' }"
        role="tab"
        type="button"
        @click="activeTab = 'collab'"
      >
        <DemoIcon name="share-2" :size="15" />
        <span>桌面协同引擎</span>
        <span v-if="activeSessions.length > 0" class="tab-warn-badge">{{ activeSessions.length }} 图锁定</span>
      </button>

      <button
        class="nav-tab-btn"
        :class="{ active: activeTab === 'update' }"
        role="tab"
        type="button"
        @click="activeTab = 'update'"
      >
        <DemoIcon name="download" :size="15" />
        <span>客户端更新</span>
        <span v-if="updateResult?.updateAvailable" class="tab-new-badge">新版</span>
      </button>
    </nav>

    <!-- 主体内容卡片区 -->
    <main class="settings-tab-content">
      <!-- 1. 外观个性化 Tab -->
      <div v-show="activeTab === 'appearance'" class="tab-panel appearance-panel">
        <!-- 界面主题皮肤 -->
        <section class="card settings-card">
          <div class="card-head">
            <div class="card-head-title">
              <DemoIcon name="sparkles" :size="16" />
              <h3>界面主题风格</h3>
              <span class="card-head-hint">实时切换全系统设计语言与色彩光效</span>
            </div>
            <span class="current-theme-pill">当前生效：{{ skins.find(s => s.key === themeStore.skin)?.title }}</span>
          </div>

          <div class="skin-gallery-grid">
            <div
              v-for="item in skins"
              :key="item.key"
              class="skin-gallery-card"
              :class="{ active: themeStore.skin === item.key }"
              @click="selectSkin(item.key)"
            >
              <!-- 顶部微缩设计预览画布 -->
              <div class="skin-mockup" :style="{ background: item.colors.bg }">
                <div class="mock-topbar" :style="{ background: item.colors.panel }">
                  <span class="mock-dot" :style="{ background: item.colors.accent }"></span>
                  <span class="mock-line" :style="{ background: item.colors.accent, opacity: 0.3 }"></span>
                </div>
                <div class="mock-body">
                  <div class="mock-sidebar" :style="{ background: item.colors.panel }"></div>
                  <div class="mock-canvas">
                    <div class="mock-card" :style="{ borderColor: item.colors.accent, background: item.colors.panel }">
                      <span class="mock-block" :style="{ background: item.colors.accent }"></span>
                      <span class="mock-sub-block" :style="{ background: item.colors.accent2 }"></span>
                    </div>
                  </div>
                </div>
                <div v-if="themeStore.skin === item.key" class="skin-active-check">
                  <DemoIcon name="check" :size="12" />
                </div>
              </div>

              <!-- 卡片信息说明 -->
              <div class="skin-meta">
                <div class="skin-meta-header">
                  <strong class="skin-title">{{ item.title }}</strong>
                  <span class="skin-tag">{{ item.tag }}</span>
                </div>
                <p class="skin-desc">{{ item.desc }}</p>
                <div class="skin-palette-bar">
                  <span class="palette-swatch" :style="{ background: item.colors.bg }" title="背景基色"></span>
                  <span class="palette-swatch" :style="{ background: item.colors.panel }" title="面板底色"></span>
                  <span class="palette-swatch" :style="{ background: item.colors.accent }" title="核心强调色"></span>
                  <span class="palette-swatch" :style="{ background: item.colors.accent2 }" title="辅助对比色"></span>
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- 双列排布：明暗模式 & 登录动效 -->
        <div class="appearance-sub-grid">
          <!-- 色彩明暗模式 -->
          <section class="card settings-card">
            <div class="card-head">
              <div class="card-head-title">
                <DemoIcon name="sun-moon" :size="16" />
                <h3>色彩模式（明 / 暗）</h3>
              </div>
            </div>

            <div class="mode-selection-group">
              <div
                v-for="item in modes"
                :key="item.key"
                class="mode-pill-card"
                :class="{ active: themeStore.mode === item.key }"
                @click="selectMode(item.key)"
              >
                <div class="mode-pill-icon">
                  <DemoIcon :name="item.icon" :size="20" />
                </div>
                <div class="mode-pill-text">
                  <strong>{{ item.title }}</strong>
                  <p>{{ item.desc }}</p>
                </div>
                <div class="custom-radio">
                  <span class="radio-dot"></span>
                </div>
              </div>
            </div>
          </section>

          <!-- 登录转场动效 -->
          <section class="card settings-card">
            <div class="card-head">
              <div class="card-head-title">
                <DemoIcon name="log-in" :size="16" />
                <h3>登录过渡动效</h3>
                <span class="card-head-hint">认证成功进入系统的动画</span>
              </div>
            </div>

            <div class="animation-selection-group">
              <div
                v-for="item in loginAnimations"
                :key="item.key"
                class="animation-pill-card"
                :class="{ active: preferenceStore.loginAnimation === item.key }"
                @click="selectLoginAnimation(item.key)"
              >
                <div class="animation-pill-icon">
                  <DemoIcon :name="item.icon" :size="20" />
                </div>
                <div class="animation-pill-text">
                  <strong>{{ item.title }}</strong>
                  <p>{{ item.desc }}</p>
                </div>
                <div class="custom-radio">
                  <span class="radio-dot"></span>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>

      <!-- 2. 工作台偏好 Tab -->
      <div v-show="activeTab === 'workbench'" class="tab-panel workbench-panel">
        <section class="card settings-card">
          <div class="card-head">
            <div class="card-head-title">
              <DemoIcon name="sliders" :size="16" />
              <h3>CAD 工作台运行参数与交互行为</h3>
            </div>
          </div>

          <div class="switch-settings-list">
            <div class="switch-row" @click="autoFocusLibrary = !autoFocusLibrary">
              <div class="switch-icon-box">
                <DemoIcon name="folder-kanban" :size="18" />
              </div>
              <div class="switch-text">
                <div class="switch-title-wrap">
                  <strong>启动时自动聚焦图纸库</strong>
                  <span class="badge-tag">常用</span>
                </div>
                <p>启动应用并在认证通过后，首屏直接呈现图纸库与最近项目，减少一级跳转交互。</p>
              </div>
              <label class="modern-switch" @click.stop>
                <input v-model="autoFocusLibrary" type="checkbox" />
                <span class="switch-slider"></span>
              </label>
            </div>

            <div class="switch-row" @click="forkBranchProtect = !forkBranchProtect">
              <div class="switch-icon-box">
                <DemoIcon name="git-branch" :size="18" />
              </div>
              <div class="switch-text">
                <div class="switch-title-wrap">
                  <strong>零件图借用独立分支保护</strong>
                  <span class="badge-tag protect">防篡改保护</span>
                </div>
                <p>修改跨项目借用的零件图时，系统强制引导创建独立项目分支副本，防止影响原始基准图纸。</p>
              </div>
              <label class="modern-switch" @click.stop>
                <input v-model="forkBranchProtect" type="checkbox" />
                <span class="switch-slider"></span>
              </label>
            </div>
          </div>
        </section>
      </div>

      <!-- 3. 桌面协同引擎 Tab -->
      <div v-show="activeTab === 'collab'" class="tab-panel collab-panel">
        <section class="card settings-card">
          <div class="card-head">
            <div class="card-head-title">
              <DemoIcon name="cpu" :size="16" />
              <h3>协同服务与宿主运行状态</h3>
              <span class="card-head-hint">CAXA / AutoCAD 双向无感协同与文件排他锁</span>
            </div>
            <button
              class="btn secondary sm"
              type="button"
              :disabled="systemStore.loading || loadingSessions"
              @click="refreshSystemStatus"
            >
              <DemoIcon :name="(systemStore.loading || loadingSessions) ? 'loader' : 'refresh-cw'" :size="13" />
              <span>刷新运行看板</span>
            </button>
          </div>

          <div v-if="!smbStatus" class="collab-alert-box">
            <DemoIcon name="alert-triangle" :size="18" />
            <span>未能检测到协同共享服务状态，请确保本地后端服务正常监听。</span>
          </div>

          <template v-else>
            <!-- 仪表盘状态四宫格 -->
            <div class="engine-metrics-grid">
              <div class="metric-card">
                <div class="metric-top">
                  <span class="metric-label">协同共享服务 (SMB)</span>
                  <span class="status-indicator-badge" :class="smbStatus.serverRunning ? 'status-ok' : 'status-err'">
                    <span class="indicator-dot"></span>
                    {{ smbStatus.serverRunning ? '服务运行中' : '未启动' }}
                  </span>
                </div>
                <div class="metric-body">
                  <div class="metric-icon" :class="smbStatus.serverRunning ? 'icon-ok' : 'icon-err'">
                    <DemoIcon :name="smbStatus.serverRunning ? 'check-circle' : 'alert-circle'" :size="20" />
                  </div>
                  <div class="metric-detail">
                    <strong>{{ smbStatus.serverRunning ? '实时排他锁已就绪' : '共享未连接' }}</strong>
                    <small>{{ smbStatus.configuredShareExists ? `已挂载 \\\\${smbStatus.shareName}` : '未找到共享目录' }}</small>
                  </div>
                </div>
              </div>

              <div class="metric-card">
                <div class="metric-top">
                  <span class="metric-label">CAXA CAD 宿主环境</span>
                  <span class="status-indicator-badge" :class="caxaStatus?.available ? 'status-ok' : 'status-warn'">
                    <span class="indicator-dot"></span>
                    {{ caxaStatus?.available ? '已关联宿主' : '未检测到程序' }}
                  </span>
                </div>
                <div class="metric-body">
                  <div class="metric-icon" :class="caxaStatus?.available ? 'icon-ok' : 'icon-warn'">
                    <DemoIcon name="monitor" :size="20" />
                  </div>
                  <div class="metric-detail">
                    <strong :title="caxaStatus?.path || '未找到安装路径'">
                      {{ caxaStatus?.available ? 'CAD 2022+ 原生插件' : '缺少本地安装' }}
                    </strong>
                    <small>{{ caxaStatus?.path ? '支持一键调起绘图' : '支持浏览器在线查看' }}</small>
                  </div>
                </div>
              </div>

              <div class="metric-card">
                <div class="metric-top">
                  <span class="metric-label">桌面深度协议唤起</span>
                  <span class="status-indicator-badge" :class="protocolRegistered ? 'status-ok' : 'status-info'">
                    <span class="indicator-dot"></span>
                    {{ !isTauri() ? 'Web 浏览器模式' : protocolRegistered ? '已注册协议' : '准备就绪' }}
                  </span>
                </div>
                <div class="metric-body">
                  <div class="metric-icon icon-info">
                    <DemoIcon name="link-2" :size="20" />
                  </div>
                  <div class="metric-detail">
                    <strong>cadguanliq:// 协议</strong>
                    <small>支持浏览器与桌面端免密票据传递唤起</small>
                  </div>
                </div>
              </div>

              <div class="metric-card">
                <div class="metric-top">
                  <span class="metric-label">本地缓存与落盘</span>
                  <span class="status-indicator-badge status-ok">
                    <span class="indicator-dot"></span>
                    正常可用
                  </span>
                </div>
                <div class="metric-body">
                  <div class="metric-icon icon-ok">
                    <DemoIcon name="folder-check" :size="20" />
                  </div>
                  <div class="metric-detail">
                    <strong :title="smbStatus.localRoot">{{ smbStatus.uncRoot || '物理存储映射已挂载' }}</strong>
                    <small>双重防冲突落盘保护已激活</small>
                  </div>
                </div>
              </div>
            </div>

            <!-- 全局活动协同会话表格 -->
            <div class="sessions-monitor-card">
              <div class="monitor-header">
                <div class="monitor-title-wrap">
                  <DemoIcon name="users" :size="16" />
                  <h4>当前被锁定的 CAD 图纸会话</h4>
                  <span class="monitor-count-pill">{{ activeSessions.length }} 个活跃文件</span>
                </div>
                <span class="monitor-tip">多端协同编辑时，同一张图纸仅允许单个工程师持有写锁</span>
              </div>

              <div v-if="activeSessions.length === 0" class="sessions-blank-state">
                <div class="blank-icon-circle">
                  <DemoIcon name="check-circle-2" :size="28" />
                </div>
                <div class="blank-text">
                  <strong>当前全库图纸均处于空闲状态</strong>
                  <p>没有正在独占编辑中的图纸，工程师可自由签出或打开修改。</p>
                </div>
              </div>

              <div v-else class="sessions-table-scroller">
                <table class="modern-table">
                  <thead>
                    <tr>
                      <th>图纸文件</th>
                      <th>关联图号 / 零件</th>
                      <th>当前编辑人</th>
                      <th>签出锁定时间</th>
                      <th>协同状态</th>
                      <th class="cell-action">管理操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="session in activeSessions" :key="session.id">
                      <td class="cell-file">
                        <DemoIcon name="file-text" :size="14" />
                        <span class="file-name" :title="session.fileName">{{ session.fileName || 'CAD 图纸' }}</span>
                      </td>
                      <td class="cell-no font-mono">{{ session.drawingNo || session.partNo || '-' }}</td>
                      <td>
                        <span class="user-chip">
                          <DemoIcon name="user" :size="12" />
                          <span>{{ session.userName || session.userAccount }}</span>
                          <small v-if="session.isCurrent" class="me-tag">(本机)</small>
                        </span>
                      </td>
                      <td class="cell-time font-mono">{{ new Date(session.startedAt).toLocaleTimeString() }}</td>
                      <td>
                        <span class="active-lock-tag">
                          <span class="pulse-beacon"></span>
                          <span>排他编辑中</span>
                        </span>
                      </td>
                      <td class="cell-action">
                        <button
                          v-if="session.canClose"
                          class="btn sm danger unlock-btn"
                          type="button"
                          :disabled="closingSessionId === session.id"
                          @click="handleCloseSession(session)"
                        >
                          <DemoIcon :name="closingSessionId === session.id ? 'loader' : 'unlock'" :size="12" />
                          <span>{{ closingSessionId === session.id ? '释放中…' : '强制释放锁' }}</span>
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </template>
        </section>
      </div>

      <!-- 4. 客户端更新 Tab -->
      <div v-show="activeTab === 'update'" class="tab-panel update-panel-view">
        <section class="card settings-card">
          <div class="card-head">
            <div class="card-head-title">
              <DemoIcon name="download-cloud" :size="16" />
              <h3>客户端版本检测与更新源管理</h3>
              <span class="card-head-hint">配置局域网或云端更新地址，保持软件处于最新演进版本</span>
            </div>
          </div>

          <!-- 版本看板 -->
          <div class="version-banner">
            <div class="version-badge-big">
              <DemoIcon name="layers" :size="24" />
            </div>
            <div class="version-details">
              <div class="version-title-wrap">
                <h3>图枢 CAD·PDM 桌面客户端</h3>
                <span class="current-tag">当前版本 v{{ appConfig.version }}</span>
              </div>
              <p class="version-state-desc">
                <template v-if="updateError">
                  <span class="text-danger">{{ updateError }}</span>
                </template>
                <template v-else-if="updateResult?.updateAvailable">
                  <span class="text-ok">✨ 发现新版本 <strong>v{{ updateResult.latestVersion }}</strong>！请及时更新以获取最新特性与性能优化。</span>
                </template>
                <template v-else-if="updateResult">
                  <span class="text-ok">✓ 当前已是最新版本（最新检查版本 v{{ updateResult.latestVersion }}）</span>
                </template>
                <template v-else>
                  <span>点击右侧按钮发起版本检查。</span>
                </template>
              </p>
            </div>
            <div class="version-actions">
              <button
                class="btn primary check-update-btn"
                type="button"
                :disabled="checkingUpdate"
                @click="handleCheckForUpdates"
              >
                <DemoIcon :name="checkingUpdate ? 'loader' : 'refresh-cw'" :size="14" />
                <span>{{ checkingUpdate ? '检查中…' : '检查更新' }}</span>
              </button>
              <button
                v-if="updateResult?.updateAvailable"
                class="btn primary download-btn"
                type="button"
                @click="handleDownloadUpdate"
              >
                <DemoIcon name="download" :size="14" />
                <span>下载更新包</span>
              </button>
            </div>
          </div>

          <!-- 更新服务器自定义配置 -->
          <div class="server-config-card">
            <div class="server-config-title">
              <DemoIcon name="server" :size="15" />
              <strong>更新服务器地址（可选）</strong>
              <span class="server-hint">内网服务器 IP 或端口变动时自定义，不影响主服务数据通信</span>
            </div>

            <div class="server-input-group">
              <input
                v-model="serverInput"
                class="server-input"
                type="text"
                placeholder="例如：192.168.1.50:8080 或 http://update.company.lan"
                @keyup.enter="handleProbeServer"
              />
              <button class="btn secondary sm" type="button" :disabled="probing" @click="handleProbeServer">
                <DemoIcon :name="probing ? 'loader' : 'zap'" :size="13" />
                <span>{{ probing ? '探测中…' : '连接测试' }}</span>
              </button>
              <button class="btn secondary sm" type="button" @click="handleSaveServer">
                <DemoIcon name="save" :size="13" />
                <span>记住地址</span>
              </button>
              <button v-if="serverSaved" class="btn secondary sm" type="button" @click="handleClearServer">
                <span>恢复默认</span>
              </button>
            </div>

            <div v-if="probeResult" class="probe-feedback-box" :class="{ fail: !probeResult.reachable }">
              <DemoIcon :name="probeResult.reachable ? 'check-circle-2' : 'alert-circle'" :size="15" />
              <span v-if="probeResult.reachable">
                服务器通信正常 · 响应服务版本 v{{ probeResult.version || '最新' }}
              </span>
              <span v-else>连接测试失败：{{ probeResult.message || '目标主机不可达或拒绝连接' }}</span>
            </div>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px 20px 24px;
}

/* 顶部标题栏 */
.settings-head-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.settings-head-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.settings-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.settings-badge-icon {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
}

.settings-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0.3px;
}

.settings-version-tag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-3);
  background: var(--panel-2);
  border: 1px solid var(--line);
  padding: 2px 8px;
  border-radius: 6px;
}

.settings-subtitle {
  margin: 0;
  color: var(--text-3);
  font-size: 12px;
}

.reset-btn {
  padding: 7px 14px;
  font-size: 12px;
}

/* 分类 Segmented Tabs 胶囊切换 */
.settings-nav-tabs {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 4px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 11px;
  width: fit-content;
}

.nav-tab-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 16px;
  border-radius: 8px;
  color: var(--text-2);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  background: transparent;
  border: none;
  transition: all 0.22s ease;
}

.nav-tab-btn:hover {
  color: var(--text-1);
  background: var(--hover);
}

.nav-tab-btn.active {
  color: var(--accent);
  background: var(--panel);
  box-shadow: 0 2px 8px rgb(0 0 0 / 15%), 0 0 0 1px var(--line);
}

.tab-indicator-count {
  font-size: 10.5px;
  font-weight: 500;
  color: var(--text-3);
  background: var(--panel-2);
  padding: 1px 6px;
  border-radius: 99px;
}

.tab-warn-badge {
  font-size: 10px;
  font-weight: 600;
  background: rgba(239, 68, 68, 0.15);
  color: var(--danger);
  border: 1px solid rgba(239, 68, 68, 0.3);
  padding: 1px 6px;
  border-radius: 99px;
}

.tab-new-badge {
  font-size: 10px;
  font-weight: 600;
  background: var(--accent-soft);
  color: var(--accent);
  border: 1px solid var(--accent);
  padding: 1px 6px;
  border-radius: 99px;
}

/* 卡片统一规范 */
.settings-card {
  padding: 16px 18px;
  border-radius: 12px;
  border: 1px solid var(--line);
  background: var(--panel);
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--line);
}

.card-head-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-head-title svg {
  color: var(--accent);
}

.card-head-title h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
}

.card-head-hint {
  color: var(--text-3);
  font-size: 11.5px;
}

.current-theme-pill {
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  border: 1px solid var(--line-strong, var(--line));
  padding: 2px 9px;
  border-radius: 99px;
}

/* 皮肤画廊微缩模型卡片 */
.skin-gallery-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 12px;
}

.skin-gallery-card {
  display: flex;
  flex-direction: column;
  border: 1.5px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  overflow: hidden;
  cursor: pointer;
  transition: all 0.25s ease;
}

.skin-gallery-card:hover {
  border-color: var(--accent);
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgb(0 0 0 / 20%);
}

.skin-gallery-card.active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent), 0 8px 24px var(--glow);
  background: var(--panel);
}

/* 微缩界面 Mockup */
.skin-mockup {
  position: relative;
  height: 74px;
  padding: 7px;
  display: flex;
  flex-direction: column;
  gap: 5px;
  border-bottom: 1px solid var(--line);
}

.mock-topbar {
  height: 12px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px;
}

.mock-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
}

.mock-line {
  width: 24px;
  height: 3px;
  border-radius: 2px;
}

.mock-body {
  flex: 1;
  display: flex;
  gap: 5px;
}

.mock-sidebar {
  width: 28px;
  border-radius: 4px;
}

.mock-canvas {
  flex: 1;
  border-radius: 4px;
  padding: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.mock-card {
  width: 100%;
  height: 100%;
  border-radius: 3px;
  border: 1px solid;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px;
}

.mock-block {
  width: 14px;
  height: 6px;
  border-radius: 2px;
}

.mock-sub-block {
  width: 22px;
  height: 4px;
  border-radius: 2px;
}

.skin-active-check {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--accent);
  color: var(--accent-ink);
  display: grid;
  place-items: center;
  box-shadow: 0 2px 6px rgb(0 0 0 / 40%);
}

.skin-meta {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.skin-meta-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.skin-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.skin-tag {
  font-size: 10px;
  font-weight: 500;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 5px;
  border-radius: 4px;
}

.skin-desc {
  margin: 0;
  font-size: 11px;
  color: var(--text-3);
  line-height: 1.45;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.skin-palette-bar {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: 4px;
  padding-top: 6px;
  border-top: 1px solid var(--line);
}

.palette-swatch {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 1px solid rgb(255 255 255 / 15%);
}

/* 明暗模式 & 登录动效双列 */
.appearance-sub-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.mode-selection-group,
.animation-selection-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mode-pill-card,
.animation-pill-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1.5px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.22s ease;
}

.mode-pill-card:hover,
.animation-pill-card:hover {
  border-color: var(--accent);
  background: var(--panel);
}

.mode-pill-card.active,
.animation-pill-card.active {
  border-color: var(--accent);
  background: var(--panel);
  box-shadow: 0 0 0 1px var(--accent), 0 4px 14px var(--glow);
}

.mode-pill-icon,
.animation-pill-icon {
  width: 34px;
  height: 34px;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
  display: grid;
  place-items: center;
  flex: none;
}

.mode-pill-text,
.animation-pill-text {
  flex: 1;
  min-width: 0;
}

.mode-pill-text strong,
.animation-pill-text strong {
  display: block;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.mode-pill-text p,
.animation-pill-text p {
  margin: 2px 0 0;
  font-size: 11px;
  color: var(--text-3);
  line-height: 1.4;
}

.custom-radio {
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 1.5px solid var(--line);
  display: grid;
  place-items: center;
  flex: none;
  transition: all 0.2s;
}

.mode-pill-card.active .custom-radio,
.animation-pill-card.active .custom-radio {
  border-color: var(--accent);
  background: var(--accent);
}

.radio-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent-ink);
  opacity: 0;
  transition: opacity 0.2s;
}

.mode-pill-card.active .radio-dot,
.animation-pill-card.active .radio-dot {
  opacity: 1;
}

/* 工作台偏好 Switch 开关行 */
.switch-settings-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.switch-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 16px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.22s ease;
}

.switch-row:hover {
  background: var(--panel);
  border-color: var(--line-strong, var(--line));
}

.switch-icon-box {
  width: 36px;
  height: 36px;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
  display: grid;
  place-items: center;
  flex: none;
}

.switch-text {
  flex: 1;
  min-width: 0;
}

.switch-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-title-wrap strong {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.badge-tag {
  font-size: 10px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 6px;
  border-radius: 4px;
}

.badge-tag.protect {
  color: var(--ok);
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
}

.switch-text p {
  margin: 3px 0 0;
  font-size: 11.5px;
  color: var(--text-3);
  line-height: 1.45;
}

/* 高质感现代 Switch */
.modern-switch {
  position: relative;
  display: inline-block;
  width: 44px;
  height: 24px;
  flex: none;
}

.modern-switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.switch-slider {
  position: absolute;
  inset: 0;
  cursor: pointer;
  background-color: var(--line);
  border-radius: 24px;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.switch-slider:before {
  position: absolute;
  content: "";
  height: 18px;
  width: 18px;
  left: 3px;
  bottom: 3px;
  background-color: #fff;
  border-radius: 50%;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: 0 1px 4px rgb(0 0 0 / 30%);
}

.modern-switch input:checked + .switch-slider {
  background-color: var(--accent);
}

.modern-switch input:checked + .switch-slider:before {
  transform: translateX(20px);
  background-color: var(--accent-ink);
}

/* 协同引擎仪表盘网格 */
.engine-metrics-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}

.metric-card {
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.metric-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.metric-label {
  font-size: 11px;
  color: var(--text-3);
  font-weight: 500;
}

.status-indicator-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 1px 6px;
  border-radius: 99px;
  font-size: 10px;
  font-weight: 600;
}

.indicator-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

.status-ok { background: var(--ok-soft, rgba(34, 197, 94, 0.12)); color: var(--ok, #22c55e); }
.status-warn { background: var(--warn-soft, rgba(245, 158, 11, 0.12)); color: var(--warn, #f59e0b); }
.status-err { background: rgba(239, 68, 68, 0.12); color: var(--danger, #ef4444); }
.status-info { background: var(--accent-soft); color: var(--accent); }

.metric-body {
  display: flex;
  align-items: center;
  gap: 10px;
}

.metric-icon {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  flex: none;
}

.icon-ok { background: var(--ok-soft, rgba(34, 197, 94, 0.14)); color: var(--ok, #22c55e); }
.icon-warn { background: var(--warn-soft, rgba(245, 158, 11, 0.14)); color: var(--warn, #f59e0b); }
.icon-err { background: rgba(239, 68, 68, 0.14); color: var(--danger, #ef4444); }
.icon-info { background: var(--accent-soft); color: var(--accent); }

.metric-detail {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.metric-detail strong {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-detail small {
  font-size: 10.5px;
  color: var(--text-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 协同排他锁监控表格 */
.sessions-monitor-card {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.monitor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.monitor-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.monitor-title-wrap svg {
  color: var(--accent);
}

.monitor-title-wrap h4 {
  margin: 0;
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.monitor-count-pill {
  font-size: 10.5px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 1px 7px;
  border-radius: 99px;
}

.monitor-tip {
  font-size: 11px;
  color: var(--text-3);
}

.sessions-blank-state {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 14px;
  border-radius: 8px;
  background: var(--panel);
  border: 1px dashed var(--line);
}

.blank-icon-circle {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok, #22c55e);
  display: grid;
  place-items: center;
  flex: none;
}

.blank-text strong {
  display: block;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.blank-text p {
  margin: 2px 0 0;
  font-size: 11.5px;
  color: var(--text-3);
}

.sessions-table-scroller {
  overflow-x: auto;
}

.modern-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  text-align: left;
}

.modern-table th {
  padding: 8px 12px;
  font-weight: 600;
  color: var(--text-3);
  border-bottom: 1px solid var(--line);
  font-size: 11px;
}

.modern-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--line);
  color: var(--text-1);
}

.cell-file {
  display: flex;
  align-items: center;
  gap: 7px;
  font-weight: 600;
}

.cell-file svg {
  color: var(--accent);
}

.file-name {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cell-no {
  color: var(--text-2);
}

.user-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 7px;
  border-radius: 5px;
  background: var(--panel);
  border: 1px solid var(--line);
  font-size: 11.5px;
}

.user-chip svg {
  color: var(--accent);
}

.me-tag {
  color: var(--accent);
  font-weight: 600;
}

.active-lock-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 8px;
  border-radius: 99px;
  font-size: 10.5px;
  font-weight: 600;
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok, #22c55e);
  border: 1px solid rgba(34, 197, 94, 0.25);
}

.pulse-beacon {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ok, #22c55e);
  box-shadow: 0 0 6px var(--ok, #22c55e);
}

.cell-action {
  text-align: right;
}

.unlock-btn {
  padding: 3px 8px;
  font-size: 11px;
}

/* 客户端更新看板 */
.version-banner {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px 18px;
  border-radius: 10px;
  border: 1px solid var(--line);
  background: var(--panel-2);
}

.version-badge-big {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
  display: grid;
  place-items: center;
  flex: none;
}

.version-details {
  flex: 1;
  min-width: 0;
}

.version-title-wrap {
  display: flex;
  align-items: center;
  gap: 10px;
}

.version-title-wrap h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
  color: var(--text-1);
}

.current-tag {
  font-family: 'JetBrains Mono', monospace;
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
  background: var(--accent-soft);
  padding: 2px 8px;
  border-radius: 6px;
}

.version-state-desc {
  margin: 4px 0 0;
  font-size: 12px;
  color: var(--text-2);
}

.version-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

.check-update-btn,
.download-btn {
  padding: 8px 16px;
  font-size: 12.5px;
}

/* 更新服务器配置卡片 */
.server-config-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;
  border-radius: 10px;
  border: 1px solid var(--line);
  background: var(--panel-2);
}

.server-config-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
  color: var(--text-1);
}

.server-config-title svg {
  color: var(--accent);
}

.server-hint {
  font-size: 11px;
  color: var(--text-3);
  font-weight: 400;
}

.server-input-group {
  display: flex;
  align-items: center;
  gap: 8px;
}

.server-input {
  flex: 1;
  padding: 7px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
  color: var(--text-1);
  font-size: 12.5px;
  outline: none;
  transition: border-color 0.2s;
}

.server-input:focus {
  border-color: var(--accent);
}

.probe-feedback-box {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 8px 12px;
  border-radius: 7px;
  background: var(--ok-soft, rgba(34, 197, 94, 0.1));
  color: var(--ok, #22c55e);
  font-size: 11.5px;
}

.probe-feedback-box.fail {
  background: rgba(239, 68, 68, 0.1);
  color: var(--danger, #ef4444);
}

.font-mono {
  font-family: 'JetBrains Mono', monospace;
}

/* 响应式断点适配 */
@media (max-width: 1080px) {
  .engine-metrics-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 820px) {
  .appearance-sub-grid {
    grid-template-columns: 1fr;
  }
  .version-banner {
    flex-direction: column;
    align-items: flex-start;
  }
  .version-actions {
    width: 100%;
    justify-content: flex-start;
  }
  .server-input-group {
    flex-wrap: wrap;
  }
  .server-input {
    min-width: 100%;
  }
}

@media (max-width: 640px) {
  .engine-metrics-grid {
    grid-template-columns: 1fr;
  }
  .settings-nav-tabs {
    width: 100%;
    overflow-x: auto;
  }
}
</style>
