<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useUiStore } from '@/stores/ui.store'
import { useDomainStore } from '@/stores/domain.store'
import { fetchReviewerCandidates } from '@/services/auth/candidate-user.service'
import { reviewFlowService, type ReviewAccountRole } from '@/services/review-flow.service'
import type { UserRole } from '@/types/domain.types'

defineOptions({
  name: 'DemoModal',
})

const uiStore = useUiStore()
const domainStore = useDomainStore()
const formReason = ref('')
const selectedVersion = ref('')
const userAccount = ref('')
const userName = ref('')
const userPassword = ref('')
const userRoles = ref<UserRole[]>(['designer'])
const resetPassword = ref('')

interface FlowNodeConfig {
  name: string
  role: ReviewAccountRole
  assignedUserId: string
  assignedName: string
  required: boolean
}

const DEFAULT_FLOW_NODES: FlowNodeConfig[] = [
  { name: '设计自检', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true },
  { name: '校对复核', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true },
  { name: '专业审核', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true },
  { name: '工艺会签', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true },
  { name: '标准化审查', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: false },
  { name: '主管批准', role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true },
]

const flowName = ref('企业标准图纸审核流程')
const flowNodes = ref<FlowNodeConfig[]>([])

const candidateUsersByRole = ref<Record<string, Array<{ id: string; name: string }>>>({
  reviewer: [],
  designer: [],
  admin: [],
})

async function loadCandidateUsers() {
  const [reviewers, designers, admins] = await Promise.all([
    fetchReviewerCandidates('reviewer'),
    fetchReviewerCandidates('designer'),
    fetchReviewerCandidates('admin'),
  ])
  candidateUsersByRole.value = {
    reviewer: reviewers.map((i) => ({ id: i.id, name: i.name })),
    designer: designers.map((i) => ({ id: i.id, name: i.name })),
    admin: admins.map((i) => ({ id: i.id, name: i.name })),
  }
}

function resetFlowForm() {
  flowName.value = '企业标准图纸审核流程'
  flowNodes.value = DEFAULT_FLOW_NODES.map((node) => ({ ...node }))
}

async function loadFlowForm(flowId?: string) {
  if (!flowId) {
    resetFlowForm()
    return
  }
  const flow = await reviewFlowService.get(flowId)
  flowName.value = flow.name
  flowNodes.value = flow.nodes.map((node, index) => ({
    name: node.name?.trim() || DEFAULT_FLOW_NODES[index]?.name || `审核节点 ${index + 1}`,
    role: node.candidateRole || 'reviewer',
    assignedUserId: node.assignedUserId || '',
    assignedName: node.assignedName || '待定',
    required: node.required,
  }))
}

const modal = computed(() => uiStore.modal)

watch(modal, (current) => {
  if (current?.type === 'add-user') {
    resetUserForm()
  } else if (current?.type === 'reset-user') {
    resetPassword.value = ''
  } else if (current?.type === 'edit-flow') {
    loadCandidateUsers()
    loadFlowForm(current.payload?.flowId).catch((error) => {
      uiStore.toast(error instanceof Error ? error.message : '读取审核流程失败', 'warn')
    })
  }
})

function close() {
  uiStore.closeModal()
}

function resetUserForm() {
  userAccount.value = ''
  userName.value = ''
  userPassword.value = ''
  userRoles.value = ['designer']
  resetPassword.value = ''
}

function toggleRole(role: UserRole) {
  if (userRoles.value.includes(role)) {
    userRoles.value = userRoles.value.filter((item) => item !== role)
  } else {
    userRoles.value = [...userRoles.value, role]
  }
}

  async function submit() {
  const current = modal.value
  if (!current) return

  try {
    if (current.type === 'confirm') {
      const callback = current.onConfirm
      close()
      if (callback) await callback()
      return
    } else if (current.type === 'borrow-drawing') {
    uiStore.toast('借用关系已建立 · 已记录借用人/时间/版本')
    } else if (current.type === 'revert') {
    uiStore.toast(`已回退：以 ${selectedVersion.value || '目标版本'} 内容生成新版本 · 历史版本原样保留 · 全程留痕`)
    } else if (current.type === 'exit') {
    uiStore.toast('窗口关闭请求已提交', 'info')
    } else if (current.type === 'add-user') {
      await domainStore.createUser(userAccount.value, userName.value, userPassword.value, userRoles.value)
      uiStore.toast('账号已分配 · 初始密码已保存')
    } else if (current.type === 'reset-user') {
      const userId = current.payload?.userId
      if (!userId) throw new Error('未找到目标账号')
      await domainStore.resetUserPassword(userId, resetPassword.value)
      uiStore.toast('密码已重置 · 请通过内部渠道通知用户')
    } else if (current.type === 'edit-flow') {
      const flowId = current.payload?.flowId
      const input = {
        name: flowName.value.trim(),
        description: '自定义审核流程',
        enabled: true,
        nodes: flowNodes.value.map((node, index) => ({
          name: node.name.trim(),
          candidateRole: node.role,
          assignedUserId: node.assignedUserId,
          assignedName: node.assignedName || '待定',
          required: node.required,
          order: index + 1,
        })),
      }
      if (!input.name) throw new Error('审核流程名称不能为空')
      if (input.nodes.some((node) => !node.name)) throw new Error('审核节点名称不能为空')
      if (flowId) await reviewFlowService.update(flowId, input)
      else await reviewFlowService.create(input)
      uiStore.toast('审核流程已保存到数据库')
    }

    close()
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '操作失败，请稍后重试', 'warn')
  }
}
</script>

<template>
  <div v-if="modal" id="modalWrap" class="show">
    <button class="mask" type="button" aria-label="关闭弹窗" @click="close"></button>
    <section class="modal" :class="{ 'modal-flow': modal.type === 'edit-flow' }">
      <header class="modal-head">
        <h3>{{ modal.title }}</h3>
        <button class="icon-btn" type="button" @click="close"><DemoIcon name="x" /></button>
      </header>

      <div class="modal-body">
        <template v-if="modal.type === 'confirm'">
          <div class="confirm-body" :class="{ danger: modal.payload?.danger }">
            <DemoIcon :name="modal.payload?.danger ? 'alert-triangle' : 'info'" :size="30" />
            <p>{{ modal.payload?.message }}</p>
          </div>
        </template>

        <template v-else-if="modal.type === 'borrow-drawing'">
          <div class="field"><label>借用到项目</label><select class="inp"><option>请选择目标项目</option><option>智能回转减速传动装置</option><option>伺服液压动力单元</option></select></div>
        </template>

        <template v-else-if="modal.type === 'revert'">
          <div class="field"><label>回退至版本</label><select v-model="selectedVersion" class="inp"><option value="">请选择历史版本</option><option value="v1.0">v1.0 (初始设计发布)</option></select></div>
          <div class="field"><label for="revert-reason">回退原因</label><textarea id="revert-reason" v-model="formReason" class="inp" placeholder="请输入回退原因与技术说明"></textarea></div>
          <div class="note"><DemoIcon name="shield-check" :size="14" /><div>回退将以目标版本内容生成<b>新版本</b>；历史版本<b>原样保留</b>，操作留痕，可再次回退撤销。</div></div>
        </template>

        <template v-else-if="modal.type === 'exit'">
          <div class="exit-confirm"><DemoIcon name="power" :size="36" /><p>确定要关闭窗口吗？<br />所有变更已实时同步至服务器。</p></div>
        </template>

         <template v-else-if="modal.type === 'add-user'">
           <div class="note"><DemoIcon name="user-plus" :size="14" /><div>系统不开放自行注册，账号仅能由管理员在此分配。</div></div>
           <div class="modal-form-grid"><div class="field"><label for="user-account">登录账号 *</label><input id="user-account" v-model="userAccount" class="inp" placeholder="如 zhang" autocomplete="off" /></div><div class="field"><label for="user-name">姓名 *</label><input id="user-name" v-model="userName" class="inp" placeholder="如 张工" autocomplete="off" /></div></div>
           <div class="field"><label for="user-password">初始密码 *</label><input id="user-password" v-model="userPassword" class="inp" type="password" placeholder="请输入初始密码" autocomplete="new-password" /></div>
           <div class="field field-last"><label>角色 *</label><div class="role-checks"><label v-for="role in ([['designer', '设计人员'], ['reviewer', '审核人员'], ['admin', '管理员']] as const)" :key="role[0]" class="role-check"><input type="checkbox" :checked="userRoles.includes(role[0])" @change="toggleRole(role[0])" /><span>{{ role[1] }}</span></label></div></div>
         </template>

         <template v-else-if="modal.type === 'reset-user'">
           <div class="note"><DemoIcon name="lock" :size="14" /><div>正在重置账号「{{ modal.payload?.account || '未知账号' }}」的登录密码。</div></div>
           <div class="field field-last"><label for="reset-password">新密码 *</label><input id="reset-password" v-model="resetPassword" class="inp" type="password" placeholder="请输入新密码" autocomplete="new-password" /></div>
        </template>

        <template v-else-if="modal.type === 'edit-flow'">
          <div class="field"><label>流程名称</label><input v-model="flowName" class="inp" placeholder="请输入流程名称" /></div>
          <div class="field field-last">
            <label>审核节点（每个节点默认分配给「审核人员」角色，可根据需要调整身份或指定人员）</label>
            <div v-for="(node, index) in flowNodes" :key="index" class="flow-edit-row">
              <input v-model="node.name" class="inp flow-node-name" placeholder="节点名称" />
              <select v-model="node.role" class="inp flow-node-role" title="候选身份角色">
                <option value="reviewer">审核人员 (默认)</option>
                <option value="designer">设计人员</option>
                <option value="admin">管理员</option>
              </select>
              <select v-model="node.assignedUserId" class="inp flow-node-assignee" title="指定审核人（可选）" @change="node.assignedName = (candidateUsersByRole[node.role] || []).find((item) => item.id === node.assignedUserId)?.name || '待定'">
                <option value="">待定 (动态按角色匹配)</option>
                <option v-for="user in (candidateUsersByRole[node.role] || [])" :key="user.id" :value="user.id">{{ user.name }}</option>
              </select>
              <label class="flow-req-check"><input v-model="node.required" type="checkbox" />必需</label>
              <button class="icon-btn" type="button" @click="flowNodes.splice(index, 1)"><DemoIcon name="trash-2" :size="14" /></button>
            </div>
            <button class="btn sm" type="button" @click="flowNodes.push({ name: `新审核节点 ${flowNodes.length + 1}`, role: 'reviewer', assignedUserId: '', assignedName: '待定', required: true })"><DemoIcon name="plus" :size="14" />添加节点</button>
          </div>
          <div class="note"><DemoIcon name="info" :size="14" /><div>流程节点身份可自定义，系统默认采用 reviewer 审核身份候选，也可指定特定设计或管理岗位。</div></div>
        </template>
      </div>

      <footer class="modal-foot">
        <button class="btn" type="button" @click="close">取消</button>
         <button class="btn primary" :class="{ danger: modal.type === 'confirm' && modal.payload?.danger }" type="button" @click="submit"><DemoIcon :name="modal.type === 'confirm' && modal.payload?.danger ? 'alert-triangle' : 'check'" :size="14" />{{ modal.type === 'confirm' ? (modal.payload?.confirmText || '确定') : modal.type === 'exit' ? '退出' : modal.type === 'borrow-drawing' ? '建立借用' : modal.type === 'revert' ? '执行回退' : modal.type === 'add-user' ? '创建账号' : modal.type === 'reset-user' ? '重置密码' : '保存流程' }}</button>
      </footer>
    </section>
  </div>
</template>
