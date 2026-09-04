<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { useAuthStore } from '@/stores/auth.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AdminSidebar' })

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const uiStore = useUiStore()

const items = [
  { name: RouteName.AdminAccounts, label: '账号管理', icon: 'user-plus' },
  { name: RouteName.AdminReviewFlows, label: '审核流程', icon: 'workflow' },
  { name: RouteName.AdminDrawings, label: '图纸管理', icon: 'layers' },
  { name: RouteName.AdminLogs, label: '操作日志', icon: 'scroll-text' },
  { name: RouteName.AdminAttributes, label: '图纸属性', icon: 'sliders-horizontal' },
  { name: RouteName.AdminSystemLogs, label: '系统日志', icon: 'file-terminal' },
  { name: RouteName.AdminUpdates, label: '更新管理', icon: 'rocket' },
] as const

const currentUser = computed(() => authStore.currentUser)

function go(name: string) {
  router.push({ name })
}

function backToWorkspace() {
  router.push({ name: RouteName.Dashboard })
}

function logout() {
  authStore.logout()
  uiStore.toast('已退出登录', 'info')
  router.push({ name: RouteName.Login })
}
</script>

<template>
  <aside class="admin-sidebar">
    <div class="admin-sidebar__brand">
      <div class="admin-sidebar__mark"><DemoIcon name="shield" :size="18" /></div>
      <div><strong>后台管理</strong><span>系统控制中心</span></div>
    </div>
    <div class="admin-sidebar__label">管理模块</div>
    <nav class="admin-sidebar__nav" aria-label="后台管理导航">
      <button v-for="item in items" :key="item.name" class="admin-sidebar__item" :class="{ active: route.name === item.name }" type="button" @click="go(item.name)">
        <DemoIcon :name="item.icon" :size="16" /><span>{{ item.label }}</span>
      </button>
    </nav>
    <div class="admin-sidebar__foot">
      <button class="admin-sidebar__back" type="button" @click="backToWorkspace"><DemoIcon name="arrow-left" :size="15" /><span>返回普通工作台</span></button>
      <div class="admin-sidebar__user">
        <span class="admin-sidebar__avatar">{{ currentUser?.displayName?.slice(0, 1) || '管' }}</span>
        <span class="admin-sidebar__user-info"><strong>{{ currentUser?.displayName || '系统管理员' }}</strong><small>{{ currentUser?.account || 'admin' }}</small></span>
        <button class="admin-sidebar__logout" type="button" title="退出登录" @click="logout"><DemoIcon name="log-out" :size="14" /></button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.admin-sidebar { display: flex; flex: none; width: 238px; flex-direction: column; padding: 18px 14px; background: var(--panel-top); border-right: 1px solid var(--line); }
.admin-sidebar__brand { display: flex; align-items: center; gap: 10px; padding: 4px 8px 22px; }
.admin-sidebar__mark { display: grid; width: 34px; height: 34px; place-items: center; border: 1px solid var(--accent); border-radius: 10px; color: var(--accent); background: var(--accent-soft); }
.admin-sidebar__brand strong, .admin-sidebar__brand span, .admin-sidebar__user-info strong, .admin-sidebar__user-info small { display: block; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.admin-sidebar__brand strong { font-size: 13px; }
.admin-sidebar__brand span { margin-top: 3px; color: var(--text-3); font-size: 10px; }
.admin-sidebar__label { padding: 0 10px 8px; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 10px; letter-spacing: 1.5px; }
.admin-sidebar__nav { display: flex; flex-direction: column; gap: 3px; }
.admin-sidebar__item, .admin-sidebar__back { display: flex; align-items: center; gap: 10px; width: 100%; border-radius: 9px; color: var(--text-2); text-align: left; transition: all 0.2s; }
.admin-sidebar__item { padding: 10px 11px; font-size: 12.5px; }
.admin-sidebar__item:hover, .admin-sidebar__item.active { color: var(--accent); background: var(--active); }
.admin-sidebar__item.active { box-shadow: inset 3px 0 var(--accent); }
.admin-sidebar__foot { margin-top: auto; }
.admin-sidebar__back { margin-bottom: 10px; padding: 9px 11px; color: var(--text-3); font-size: 11.5px; }
.admin-sidebar__back:hover { color: var(--text-1); background: var(--hover); }
.admin-sidebar__user { display: flex; align-items: center; gap: 8px; padding: 9px; border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }
.admin-sidebar__avatar { display: grid; width: 28px; height: 28px; flex: none; place-items: center; border-radius: 8px; background: linear-gradient(135deg, var(--accent), var(--accent-2)); color: var(--accent-ink); font-size: 12px; font-weight: 700; }
.admin-sidebar__user-info { min-width: 0; flex: 1; }
.admin-sidebar__user-info strong { font-size: 11.5px; }
.admin-sidebar__user-info small { margin-top: 2px; color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 9.5px; }
.admin-sidebar__logout { display: grid; width: 25px; height: 25px; flex: none; place-items: center; border-radius: 6px; color: var(--text-3); }
.admin-sidebar__logout:hover { color: var(--danger); background: var(--hover); }
@media (max-width: 760px) {
  .admin-sidebar { width: 64px; padding: 14px 8px; }
  .admin-sidebar__brand > div:last-child, .admin-sidebar__label, .admin-sidebar__item span, .admin-sidebar__back span, .admin-sidebar__user-info, .admin-sidebar__logout { display: none; }
  .admin-sidebar__brand { justify-content: center; padding: 4px 0 22px; }
  .admin-sidebar__item, .admin-sidebar__back { justify-content: center; padding-right: 0; padding-left: 0; }
  .admin-sidebar__user { justify-content: center; padding: 7px 0; }
}
</style>
