<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useThemeStore } from '@/stores/theme.store'
import { useUiStore } from '@/stores/ui.store'
import type { ThemeMode, ThemeSkin } from '@/types/theme.types'

defineOptions({
  name: 'SettingsPage',
})

const themeStore = useThemeStore()
const uiStore = useUiStore()

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
    title: '轻奢主题',
    desc: '雅致沉稳、注重层次对比的高级质感配色',
    icon: 'sparkles',
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

function selectSkin(skin: ThemeSkin) {
  themeStore.setSkin(skin)
  uiStore.toast(`已切换为「${skins.find((s) => s.key === skin)?.title}」`, 'ok')
}

function selectMode(mode: ThemeMode) {
  themeStore.setMode(mode)
  uiStore.toast(`已切换为「${mode === 'dark' ? '深色模式' : '浅色模式'}」`, 'ok')
}

function resetToDefault() {
  themeStore.setSkin('classic')
  themeStore.setMode('dark')
  uiStore.toast('已恢复默认设置（经典主题 + 深色模式）', 'ok')
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
}
</style>
