<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'

import { appConfig } from '@/app/app.config'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { dataManager } from '@/services/data-manager'
import type { ActiveEditSessionInfo } from '@/services/data-manager/data-provider'
import { checkForUpdates, type UpdateCheckResult } from '@/services/update.service'
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

const themeStore = useThemeStore()
const uiStore = useUiStore()
const preferenceStore = useUserPreferenceStore()
const systemStore = useSystemStatusStore()
const checkingUpdate = ref(false)
const updateResult = ref<UpdateCheckResult | null>(null)
const updateError = ref('')
const protocolRegistered = ref<boolean | null>(null)
const activeSessions = ref<ActiveEditSessionInfo[]>([])
const loadingSessions = ref(false)
const closingSessionId = ref<string | null>(null)
let sessionPollTimer: number | null = null

const smbStatus = computed(() => systemStore.status?.smb)
const caxaStatus = computed(() => systemStore.status?.caxa)

function refreshSystemStatus() {
  void systemStore.fetchStatus()
  void loadGlobalActiveSessions()
}

async function loadGlobalActiveSessions() {
  loadingSessions.value = true
  try {
    const list = await dataManager.listEditSessions()
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
    await dataManager.closeEditSession(session.id)
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

const skins: Array<{ key: ThemeSkin; title: string; desc: string; icon: string }> = [
  {
    key: 'classic',
    title: '经典主题 (默认)',
    desc: '清晰明快、稳健高效的标准工程 CAD 界面风格',
    icon: 'square',
  },
  {
    key: 'tech',
    title: '科技主题',
    desc: '带有网格动效与微光氛围的未来感工业设计风格',
    icon: 'zap',
  },
  {
    key: 'elegant',
    title: '护眼主题',
    desc: '温暖柔和、降低视觉疲劳的舒适防眩光配色',
    icon: 'sparkles',
  },
  {
    key: 'sage',
    title: '超级护眼主题',
    desc: '豆沙绿纸张质感，极低蓝光与对比度，全天候极致温润护眼',
    icon: 'leaf',
  },
  {
    key: 'starry',
    title: '星空主题',
    desc: '深邃深蓝与紫青星芒交织的现代高对比度视觉体系',
    icon: 'moon',
  },
  {
    key: 'bamboo',
    title: '墨竹主题',
    desc: '清雅苍翠、融合东方美学与自然沉静的墨绿质感风格',
    icon: 'layers',
  },
]

const modes: Array<{ key: ThemeMode; title: string; desc: string; icon: string }> = [
  {
    key: 'dark',
    title: '深色模式',
    desc: '降低屏幕刺眼度，适合长时间 CAD 审图与绘图作业',
    icon: 'moon',
  },
  {
    key: 'light',
    title: '浅色模式',
    desc: '明亮通透，适合强光环境或文档审阅对比',
    icon: 'sun',
  },
]

const loginAnimations: Array<{ key: LoginAnimation; title: string; desc: string; icon: string }> = [
  {
    key: 'laser',
    title: '左下角激光勾勒',
    desc: '粒子从登录按钮喷射，汇聚后从左下角描绘工作台轮廓并点亮边框。',
    icon: 'scan-line',
  },
  {
    key: 'burst',
    title: '粒子光环扩散',
    desc: '认证成功后显示青蓝光环与核心提示，快速进入工作台。',
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
  uiStore.toast('已恢复默认设置（经典主题 + 深色模式 + 激光登录动画）', 'ok')
}

async function handleCheckForUpdates() {
  if (checkingUpdate.value) return
  checkingUpdate.value = true
  updateError.value = ''
  try {
    updateResult.value = await checkForUpdates()
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
</script>

<template>
  <div class="page settings-page">
    <div class="settings-header">
      <div>
        <h1 class="settings-title">系统设置</h1>
        <p class="settings-subtitle">个性化工作空间偏好、外观风格及系统运行参数</p>
      </div>
      <div class="settings-actions">
        <button class="btn" type="button" @click="resetToDefault">
          <DemoIcon name="rotate-ccw" :size="14" />恢复默认设置
        </button>
      </div>
    </div>

    <div class="settings-body">
      <!-- 外观皮肤设置 -->
      <section class="card settings-section">
        <div class="section-title">
          <DemoIcon name="palette" :size="17" />
          <h2>界面主题皮肤</h2>
          <span class="sub-hint">默认经典主题，可按偏好自由切换</span>
        </div>

        <div class="skin-card-grid">
          <div
            v-for="item in skins"
            :key="item.key"
            class="theme-card"
            :class="{ active: themeStore.skin === item.key }"
            @click="selectSkin(item.key)"
          >
            <div class="theme-card-top">
              <div class="theme-icon">
                <DemoIcon :name="item.icon" :size="20" />
              </div>
              <div class="theme-radio">
                <span class="radio-indicator"></span>
              </div>
            </div>
            <div class="theme-card-body">
              <div class="theme-name">{{ item.title }}</div>
              <div class="theme-desc">{{ item.desc }}</div>
            </div>
            <div class="theme-preview-pill" :data-preview-skin="item.key">
              <span class="dot dot-accent"></span>
              <span class="dot dot-panel"></span>
              <span class="dot dot-line"></span>
            </div>
          </div>
        </div>
      </section>

      <!-- 明暗模式设置 -->
      <section class="card settings-section">
        <div class="section-title">
          <DemoIcon name="sun-moon" :size="17" />
          <h2>色彩模式（明 / 暗）</h2>
        </div>

        <div class="mode-card-grid">
          <div
            v-for="item in modes"
            :key="item.key"
            class="mode-card"
            :class="{ active: themeStore.mode === item.key }"
            @click="selectMode(item.key)"
          >
            <div class="mode-icon">
              <DemoIcon :name="item.icon" :size="22" />
            </div>
            <div class="mode-info">
              <div class="mode-title">{{ item.title }}</div>
              <div class="mode-desc">{{ item.desc }}</div>
            </div>
            <div class="theme-radio">
              <span class="radio-indicator"></span>
            </div>
          </div>
        </div>
      </section>

      <!-- 登录动画设置 -->
      <section class="card settings-section">
        <div class="section-title">
          <DemoIcon name="log-in" :size="17" />
          <h2>登录切换动画</h2>
          <span class="sub-hint">选择后下次登录生效</span>
        </div>

        <div class="login-animation-grid">
          <button
            v-for="item in loginAnimations"
            :key="item.key"
            class="login-animation-card"
            :class="{ active: preferenceStore.loginAnimation === item.key }"
            type="button"
            @click="selectLoginAnimation(item.key)"
          >
            <span class="login-animation-icon">
              <DemoIcon :name="item.icon" :size="22" />
            </span>
            <span class="login-animation-copy">
              <strong>{{ item.title }}</strong>
              <small>{{ item.desc }}</small>
            </span>
            <span class="theme-radio"><span class="radio-indicator"></span></span>
          </button>
        </div>
      </section>

      <!-- 客户端更新 -->
      <section class="card settings-section update-section">
        <div class="section-title">
          <DemoIcon name="download" :size="17" />
          <h2>客户端更新</h2>
          <span class="sub-hint">从 Go 更新服务获取最新版本信息</span>
        </div>
        <div class="update-panel">
          <div class="update-version">
            <span class="update-version__label">当前版本</span>
            <strong>v{{ appConfig.version }}</strong>
          </div>
          <div class="update-result" :class="{ 'update-result--error': updateError }">
            <template v-if="updateError">
              <DemoIcon name="alert-circle" :size="16" />
              <span>{{ updateError }}</span>
            </template>
            <template v-else-if="updateResult?.updateAvailable">
              <DemoIcon name="sparkles" :size="16" />
              <span>发现新版本 v{{ updateResult.latestVersion }}<small>{{ updateResult.notes }}</small></span>
            </template>
            <template v-else-if="updateResult">
              <DemoIcon name="check-circle-2" :size="16" />
              <span>当前已是最新版本<small>最近检查版本 v{{ updateResult.latestVersion }}</small></span>
            </template>
            <template v-else>
              <DemoIcon name="info" :size="16" />
              <span>尚未检查更新<small>点击右侧按钮获取服务器上的最新版本信息</small></span>
            </template>
          </div>
          <button class="btn primary update-button" type="button" :disabled="checkingUpdate" @click="handleCheckForUpdates">
            <DemoIcon :name="checkingUpdate ? 'loader' : 'refresh-cw'" :size="14" />
            {{ checkingUpdate ? '检查中…' : '检查更新' }}
          </button>
        </div>
      </section>

      <!-- 桌面协同与 CAD 引擎管理 -->
      <section class="card settings-section collab-section">
        <div class="section-title">
          <DemoIcon name="share-2" :size="17" />
          <h2>桌面协同与 CAD 引擎</h2>
          <span class="sub-hint">连接 CAXA / AutoCAD 本地协同，提供实时文件独占锁与版本落盘保护</span>
          <button class="btn sm" type="button" :disabled="systemStore.loading || loadingSessions" @click="refreshSystemStatus">
            <DemoIcon name="refresh-cw" :size="13" />刷新状态
          </button>
        </div>

        <div v-if="!smbStatus" class="collab-empty-alert">
          <DemoIcon name="server-off" :size="18" />
          <span>暂时无法读取协同引擎状态，请确认服务端进程正在运行。</span>
        </div>
        <template v-else>
          <!-- 四宫格引擎健康看板 -->
          <div class="engine-cards-grid">
            <div class="engine-card">
              <div class="engine-card-icon" :class="smbStatus.serverRunning ? 'status-ok-bg' : 'status-err-bg'">
                <DemoIcon :name="smbStatus.serverRunning ? 'check-circle' : 'alert-circle'" :size="18" />
              </div>
              <div class="engine-card-content">
                <span class="engine-card-label">协同共享引擎</span>
                <strong :class="smbStatus.serverRunning ? 'text-ok' : 'text-danger'">
                  {{ smbStatus.serverRunning ? '正常运行' : '未启动' }}
                </strong>
                <small>{{ smbStatus.configuredShareExists ? `已挂载共享 \\\\${smbStatus.shareName}` : '共享未就绪' }}</small>
              </div>
            </div>

            <div class="engine-card">
              <div class="engine-card-icon" :class="caxaStatus?.available ? 'status-ok-bg' : 'status-warn-bg'">
                <DemoIcon name="monitor" :size="18" />
              </div>
              <div class="engine-card-content">
                <span class="engine-card-label">CAXA 软件宿主</span>
                <strong :class="caxaStatus?.available ? 'text-ok' : 'text-warn'">
                  {{ caxaStatus?.available ? '已检测就绪' : '未自动关联' }}
                </strong>
                <small :title="caxaStatus?.path || '未找到安装路径'">
                  {{ caxaStatus?.path ? '可直接调起绘图' : '支持手动从共享打开' }}
                </small>
              </div>
            </div>

            <div class="engine-card">
              <div class="engine-card-icon" :class="protocolRegistered ? 'status-ok-bg' : 'status-info-bg'">
                <DemoIcon name="link-2" :size="18" />
              </div>
              <div class="engine-card-content">
                <span class="engine-card-label">深度协同协议</span>
                <strong :class="protocolRegistered ? 'text-ok' : 'text-muted'">
                  {{ !isTauri() ? '浏览器网页模式' : protocolRegistered ? '已注册 (cadguanliq://)' : '就绪' }}
                </strong>
                <small>支持一键免密票据传递唤起</small>
              </div>
            </div>

            <div class="engine-card">
              <div class="engine-card-icon status-ok-bg">
                <DemoIcon name="folder-check" :size="18" />
              </div>
              <div class="engine-card-content">
                <span class="engine-card-label">工作区存储</span>
                <strong class="text-ok">实时同步就绪</strong>
                <small :title="smbStatus.localRoot">{{ smbStatus.uncRoot || '已启用物理映射' }}</small>
              </div>
            </div>
          </div>

          <!-- 全局活动编辑会话监控表格 -->
          <div class="active-sessions-box">
            <div class="box-head">
              <div class="box-title">
                <DemoIcon name="users" :size="15" />
                <span>全系统活动协同编辑图纸 ({{ activeSessions.length }})</span>
              </div>
              <span class="box-hint">显示当前被各工程师打开中的 CAD 图纸，可手动释放长时间锁定的文件</span>
            </div>

            <div v-if="activeSessions.length === 0" class="sessions-empty">
              <DemoIcon name="check-circle" :size="20" />
              <span>当前无被占用的图纸，所有 CAD 图纸均处于可随时编辑状态。</span>
            </div>

            <div v-else class="sessions-table-wrap">
              <table class="tbl compact-tbl">
                <thead>
                  <tr>
                    <th>文件名称</th>
                    <th>所属图号</th>
                    <th>编辑人</th>
                    <th>锁定时间</th>
                    <th>状态</th>
                    <th style="width: 110px; text-align: right">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="session in activeSessions" :key="session.id">
                    <td class="file-name-cell">
                      <DemoIcon name="file" :size="14" />
                      <b>{{ session.fileName || 'CAD 图纸' }}</b>
                    </td>
                    <td class="num mono">{{ session.drawingNo || session.partNo || '-' }}</td>
                    <td>
                      <span class="user-tag">
                        <DemoIcon name="user" :size="12" />
                        {{ session.userName || session.userAccount }}
                        <small v-if="session.isCurrent">(我)</small>
                      </span>
                    </td>
                    <td class="num mono text-muted">{{ new Date(session.startedAt).toLocaleTimeString() }}</td>
                    <td>
                      <span class="badge-collab active">
                        <span class="pulse-dot"></span>编辑中
                      </span>
                    </td>
                    <td style="text-align: right">
                      <button
                        v-if="session.canClose"
                        class="btn sm danger"
                        type="button"
                        :disabled="closingSessionId === session.id"
                        title="强制释放此文件的独占编辑锁"
                        @click="handleCloseSession(session)"
                      >
                        {{ closingSessionId === session.id ? '解锁中...' : '解除锁定' }}
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>
      </section>

      <!-- 工作台偏好说明 -->
      <section class="card settings-section">
        <div class="section-title">
          <DemoIcon name="sliders" :size="17" />
          <h2>CAD 工作台偏好</h2>
        </div>
        <div class="pref-list">
          <div class="pref-item">
            <div class="pref-text">
              <b>启动时自动聚焦图纸库</b>
              <span>打开应用后直接呈现最近项目与图纸列表</span>
            </div>
            <input type="checkbox" checked />
          </div>
          <div class="pref-item">
            <div class="pref-text">
              <b>零件图借用独立分支保护</b>
              <span>修改借用件时默认创建项目独立副本，防止误修改原图</span>
            </div>
            <input type="checkbox" checked />
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 4px 2px 26px;
}

.settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--line);
}

.settings-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 17px;
  font-weight: 800;
}

.settings-subtitle {
  margin: 3px 0 0;
  color: var(--text-3);
  font-size: 11.5px;
}

.settings-body {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  align-items: start;
}

/* 宽区块横跨两列：客户端更新与协同引擎内容较多 */
.settings-section.update-section,
.settings-section.collab-section {
  grid-column: 1 / -1;
}

.settings-section {
  padding: 16px 18px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--line);
}

.section-title svg {
  color: var(--accent);
}

.section-title h2 {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
}

.sub-hint {
  margin-left: auto;
  color: var(--text-3);
  font-size: 11px;
}

.skin-card-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}

.theme-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 9px;
  padding: 12px;
  border: 1.5px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.25s ease;
}

.theme-card:hover {
  border-color: var(--accent);
  transform: translateY(-2px);
}

.theme-card.active {
  border-color: var(--accent);
  background: var(--panel);
  box-shadow: 0 0 0 1px var(--accent), 0 8px 24px var(--glow);
}

.theme-card-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.theme-icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--accent-soft);
  color: var(--accent);
}

.theme-icon svg {
  width: 16px;
  height: 16px;
}

.radio-indicator {
  display: inline-block;
  width: 18px;
  height: 18px;
  border: 1.5px solid var(--line);
  border-radius: 50%;
  position: relative;
  transition: all 0.2s ease;
}

.theme-card.active .radio-indicator,
.mode-card.active .radio-indicator {
  border-color: var(--accent);
  background: var(--accent);
}

.theme-card.active .radio-indicator::after,
.mode-card.active .radio-indicator::after {
  content: '';
  position: absolute;
  top: 4px;
  left: 4px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--accent-ink);
}

.theme-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-1);
}

.theme-desc {
  margin-top: 4px;
  color: var(--text-3);
  font-size: 11.5px;
  line-height: 1.5;
}

.theme-preview-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
  padding-top: 10px;
  border-top: 1px solid var(--line);
}

.dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  border: 1px solid var(--line);
}

.dot-accent { background: var(--accent); }
.dot-panel { background: var(--panel-2); }
.dot-line { background: var(--hover); }

.mode-card-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}

.mode-card {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 12px;
  border: 1.5px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.25s ease;
}

.mode-card:hover {
  border-color: var(--accent);
}

.mode-card.active {
  border-color: var(--accent);
  background: var(--panel);
  box-shadow: 0 0 0 1px var(--accent);
}

.login-animation-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}

.login-animation-card {
  display: flex;
  align-items: center;
  gap: 11px;
  width: 100%;
  padding: 11px 12px;
  border: 1.5px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
  color: var(--text-1);
  text-align: left;
  cursor: pointer;
  transition: border-color 0.25s ease, background-color 0.25s ease, box-shadow 0.25s ease;
}

.login-animation-card:hover,
.login-animation-card.active {
  border-color: var(--accent);
  background: var(--panel);
}

.login-animation-card.active {
  box-shadow: 0 0 0 1px var(--accent), 0 8px 24px var(--glow);
}

.update-panel {
  display: grid;
  grid-template-columns: minmax(110px, 0.25fr) minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
}

.update-version {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.update-version__label {
  color: var(--text-3);
  font-size: 11px;
}

.update-version strong {
  color: var(--accent);
  font-family: 'JetBrains Mono', monospace;
  font-size: 16px;
}

.update-result {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  min-width: 0;
  color: var(--text-2);
  font-size: 12px;
  line-height: 1.5;
}

.update-result svg {
  flex: none;
  margin-top: 1px;
  color: var(--accent);
}

.update-result span {
  min-width: 0;
}

.update-result small {
  display: block;
  margin-top: 2px;
  overflow-wrap: anywhere;
  color: var(--text-3);
  font-size: 11px;
}

.update-result--error,
.update-result--error svg {
  color: var(--danger);
}

.update-button {
  min-width: 112px;
  justify-content: center;
}

.update-button:disabled svg {
  animation: update-spin 1s linear infinite;
}

.collab-section .section-title {
  flex-wrap: wrap;
}

.collab-section .section-title .btn {
  margin-left: auto;
}

.collab-empty-alert {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 9px;
  background: var(--panel-2);
  color: var(--text-3);
  font-size: 12.5px;
}

.engine-cards-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}

.engine-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 11px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
}

.engine-card-icon {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  flex: none;
}

.engine-card-icon svg {
  width: 15px;
  height: 15px;
}

.status-ok-bg {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15));
  color: var(--ok, #16a34a);
}

.status-warn-bg {
  background: var(--warn-soft, rgba(245, 158, 11, 0.15));
  color: var(--warn, #d97706);
}

.status-err-bg {
  background: rgba(239, 68, 68, 0.15);
  color: var(--danger, #ef4444);
}

.status-info-bg {
  background: var(--accent-soft, rgba(59, 130, 246, 0.15));
  color: var(--accent, #3b82f6);
}

.engine-card-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.engine-card-label {
  color: var(--text-3);
  font-size: 11px;
}

.engine-card-content strong {
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.engine-card-content small {
  color: var(--text-3);
  font-size: 10.5px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.text-ok { color: var(--ok, #16a34a) !important; }
.text-warn { color: var(--warn, #d97706) !important; }
.text-danger { color: var(--danger, #ef4444) !important; }
.text-muted { color: var(--text-2) !important; }

.active-sessions-box {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
}

.box-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.box-title {
  display: flex;
  align-items: center;
  gap: 7px;
  font-weight: 700;
  font-size: 13px;
  color: var(--text-1);
}

.box-title svg {
  color: var(--accent);
}

.box-hint {
  color: var(--text-3);
  font-size: 11.5px;
}

.sessions-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px 16px;
  color: var(--ok);
  font-size: 12.5px;
}

.sessions-empty svg {
  color: var(--ok);
}

.sessions-table-wrap {
  overflow-x: auto;
}

.user-tag {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: var(--text-1);
}

.user-tag svg {
  color: var(--accent);
}

.badge-collab {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 1px 7px;
  border-radius: 99px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  white-space: nowrap;
}

.badge-collab.active {
  background: var(--ok-soft, rgba(34, 197, 94, 0.15));
  color: var(--ok, #16a34a);
  border: 1px solid rgba(34, 197, 94, 0.3);
}

.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ok, #22c55e);
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  animation: pulse-ring 1.8s infinite cubic-bezier(0.66, 0, 0, 1);
  flex: none;
}

@keyframes pulse-ring {
  0% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  }
  70% {
    box-shadow: 0 0 0 6px rgba(34, 197, 94, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0);
  }
}

@media (max-width: 1024px) {
  .engine-cards-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .engine-cards-grid {
    grid-template-columns: 1fr;
  }
}

@keyframes update-spin {
  to { transform: rotate(360deg); }
}

.login-animation-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex: none;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
}

.login-animation-icon svg {
  width: 17px;
  height: 17px;
}

.login-animation-copy {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.login-animation-copy strong {
  font-size: 13px;
}

.login-animation-copy small {
  color: var(--text-3);
  font-size: 11px;
  line-height: 1.5;
}

.mode-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
  flex: none;
}

.mode-icon svg {
  width: 17px;
  height: 17px;
}

.mode-info {
  flex: 1;
}

.mode-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.mode-desc {
  margin-top: 2px;
  color: var(--text-3);
  font-size: 11px;
}

.pref-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.pref-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel-2);
}

.pref-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pref-text b {
  font-size: 13px;
  color: var(--text-1);
}

.pref-text span {
  color: var(--text-3);
  font-size: 11.5px;
}

@media (max-width: 1100px) {
  .settings-body {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 900px) {
  .skin-card-grid {
    grid-template-columns: 1fr;
  }
  .mode-card-grid {
    grid-template-columns: 1fr;
  }

  .login-animation-grid {
    grid-template-columns: 1fr;
  }

  .update-panel {
    grid-template-columns: 1fr auto;
  }

  .update-result {
    grid-column: 1 / -1;
    grid-row: 2;
  }
}
</style>
