<script setup lang="ts">
import { ref } from 'vue'

import { appConfig } from '@/app/app.config'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { checkForUpdates, type UpdateCheckResult } from '@/services/update.service'
import { useThemeStore } from '@/stores/theme.store'
import { useUiStore } from '@/stores/ui.store'
import type { ThemeMode, ThemeSkin } from '@/types/theme.types'
import { useUserPreferenceStore, type LoginAnimation } from '@/stores/user-preference.store'

defineOptions({
  name: 'SettingsPage',
})

const themeStore = useThemeStore()
const uiStore = useUiStore()
const preferenceStore = useUserPreferenceStore()
const checkingUpdate = ref(false)
const updateResult = ref<UpdateCheckResult | null>(null)
const updateError = ref('')

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
  gap: 18px;
  padding: 6px 4px 30px;
}

.settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.settings-title {
  margin: 0;
  font-family: var(--font-display);
  font-size: 20px;
  font-weight: 800;
}

.settings-subtitle {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.settings-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.settings-section {
  padding: 20px;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--line);
}

.section-title svg {
  color: var(--accent);
}

.section-title h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
}

.sub-hint {
  margin-left: auto;
  color: var(--text-3);
  font-size: 11.5px;
}

.skin-card-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 14px;
}

.theme-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
  border: 1.5px solid var(--line);
  border-radius: 12px;
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
  width: 36px;
  height: 36px;
  border-radius: 9px;
  background: var(--accent-soft);
  color: var(--accent);
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
  gap: 14px;
}

.mode-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px;
  border: 1.5px solid var(--line);
  border-radius: 12px;
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
  gap: 14px;
}

.login-animation-card {
  display: flex;
  align-items: center;
  gap: 13px;
  width: 100%;
  padding: 15px;
  border: 1.5px solid var(--line);
  border-radius: 12px;
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
  gap: 16px;
  padding: 14px 16px;
  border: 1px solid var(--line);
  border-radius: 11px;
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

@keyframes update-spin {
  to { transform: rotate(360deg); }
}

.login-animation-icon {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  flex: none;
  border-radius: 11px;
  background: var(--accent-soft);
  color: var(--accent);
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
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  flex: none;
}

.mode-info {
  flex: 1;
}

.mode-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--text-1);
}

.mode-desc {
  margin-top: 3px;
  color: var(--text-3);
  font-size: 11.5px;
}

.pref-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.pref-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
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
