<script setup lang="ts">
import { onMounted } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAdminStore } from '@/stores/admin.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'AccountManagementPage' })

const adminStore = useAdminStore()
const uiStore = useUiStore()

// 进入页面即重拉一次账号列表：initialize 是一次性快照，
// 初始化时机早于角色恢复或他人新建账号时，快照会漏掉最新数据。
onMounted(() => {
  void adminStore.loadUsers()
})

async function toggleUser(index: number) {
  try {
    const user = adminStore.users[index]
    if (!user) return
    await adminStore.toggleUser(user.id)
  } catch (error) {
    console.error('保存账号状态失败', error)
    uiStore.toast(error instanceof Error ? error.message : '账号状态保存失败，请稍后重试', 'warn')
  }
}

function roleLabel(role: 'admin' | 'designer' | 'reviewer'): string {
  return role === 'admin' ? '管理员' : role === 'reviewer' ? '审核人员' : '设计人员'
}

function openResetPassword(user: { id: string; account: string }) {
  uiStore.openModal('reset-user', '重置密码', { userId: user.id, account: user.account })
}
</script>

<template>
  <div class="page admin-page">
    <div class="section-head">
      <h3>账号管理</h3>
      <button class="btn primary admin-action" type="button" @click="uiStore.openModal('add-user', '分配账号')"><DemoIcon name="user-plus" :size="14" />分配账号</button>
    </div>
    <div class="card">
      <table class="tbl">
        <thead><tr><th>账号</th><th>姓名</th><th>角色</th><th>状态</th><th>最近活跃</th><th>操作</th></tr></thead>
        <tbody>
              <tr v-for="(user, index) in adminStore.users" :key="user.id">
            <td class="num">{{ user.account }}</td><td class="user-name"><span class="mini-avatar">{{ user.displayName[0] }}</span>{{ user.displayName }}</td>
            <td><span v-for="role in user.roles" :key="role" class="tag plain role-tag">{{ roleLabel(role) }}</span></td>
            <td><span class="tag" :class="user.status === 'active' ? 'ok' : 'danger'">{{ user.status === 'active' ? '正常' : '已禁用' }}</span></td><td class="num updated">{{ user.lastLoginAt ? new Date(user.lastLoginAt).toLocaleString() : '从未登录' }}</td>
            <td class="admin-row-actions"><button class="btn sm" :class="{ danger: user.status === 'active' }" type="button" @click="toggleUser(index)">{{ user.status === 'active' ? '禁用' : '启用' }}</button><button class="btn sm" type="button" @click="openResetPassword(user)">重置密码</button></td>
          </tr>
              <tr v-if="!adminStore.users.length"><td colspan="6"><div class="empty"><DemoIcon name="user-plus" :size="34" /><div class="t">暂无账号数据</div></div></td></tr>
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
