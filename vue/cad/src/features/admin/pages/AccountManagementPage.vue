<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import AdminTabs from '../components/AdminTabs.vue'
import { useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AccountManagementPage' })

const demoStore = useDemoStore()
const uiStore = useUiStore()
</script>

<template>
  <div class="page admin-page">
    <div class="section-head">
      <h3>后台管理</h3><span class="lib-count">管理员账号同时继承普通用户全部功能</span>
      <button class="btn primary admin-action" type="button" @click="uiStore.openModal('add-user', '分配账号')"><DemoIcon name="user-plus" :size="14" />分配账号</button>
    </div>
    <AdminTabs active="users" />
    <div class="note admin-note"><DemoIcon name="user-plus" :size="14" /><div>系统不开放自行注册，账号仅由管理员在此分配；同一用户可拥有多个角色，管理员账号同时继承普通用户全部功能。</div></div>
    <div class="card">
      <table class="tbl">
        <thead><tr><th>账号</th><th>姓名</th><th>角色</th><th>状态</th><th>最近活跃</th><th>操作</th></tr></thead>
        <tbody>
          <tr v-for="(user, index) in demoStore.users" :key="user.acc">
            <td class="num">{{ user.acc }}</td><td class="user-name"><span class="mini-avatar">{{ user.name[0] }}</span>{{ user.name }}</td>
            <td><span v-for="role in user.role.split(' · ')" :key="role" class="tag plain role-tag">{{ role }}</span></td>
            <td><span class="tag" :class="user.status === '正常' ? 'ok' : 'danger'">{{ user.status }}</span></td><td class="num updated">{{ user.last }}</td>
            <td class="admin-row-actions"><button class="btn sm" :class="{ danger: user.status === '正常' }" type="button" @click="demoStore.toggleUser(index)">{{ user.status === '正常' ? '禁用' : '启用' }}</button><button class="btn sm" type="button" @click="uiStore.toast('密码已重置并通知用户')">重置密码</button></td>
          </tr>
          <tr v-if="!demoStore.users.length"><td colspan="6"><div class="empty"><DemoIcon name="user-plus" :size="34" /><div class="t">暂无账号数据</div></div></td></tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.section-head { display: flex; align-items: center; gap: 10px; margin: 4px 0 6px; }
.section-head h3 { font-family: var(--font-display); font-size: 15.5px; font-weight: 900; }
.admin-action { margin-left: auto; }
.admin-note { margin-bottom: 14px; }
.mini-avatar { display: inline-grid; width: 22px; height: 22px; margin-right: 8px; place-items: center; border-radius: 7px; background: linear-gradient(135deg, var(--accent), var(--accent-2)); color: var(--accent-ink); font-size: 10.5px; vertical-align: -6px; }
.user-name { font-weight: 500; }
.role-tag { margin-right: 5px; }
.admin-row-actions { white-space: nowrap; }
</style>
