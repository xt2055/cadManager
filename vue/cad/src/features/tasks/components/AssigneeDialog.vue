<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { DrawingTaskCandidate, DrawingTaskHistoryEntry, DrawingTaskRow } from '@/services/drawing-task.service'
import { roleLabels } from '../task.helpers'

defineOptions({ name: 'AssigneeDialog' })

const props = defineProps<{
  row: DrawingTaskRow | null
  candidates: readonly DrawingTaskCandidate[]
  history: readonly DrawingTaskHistoryEntry[]
  loadingCandidates: boolean
  loadingHistory: boolean
  busy: boolean
  error: string
}>()

const emit = defineEmits<{
  (event: 'submit', payload: { assigneeId: string; note: string; dueDate: string; reason: string }): void
  (event: 'close'): void
}>()

const assigneeId = ref('')
const note = ref('')
const dueDate = ref('')
const reason = ref('')

/** 已有负责人时是改派：多一个「改派说明」字段，用于留下为什么换人的记录。 */
const isReassign = computed(() => Boolean(props.row?.assignment))

watch(() => props.row, (row) => {
  // 打开对话框时带出当前指派，避免计划员重复输入备注与截止日期。
  assigneeId.value = row?.assignment?.assigneeId ?? ''
  note.value = row?.assignment?.note ?? ''
  dueDate.value = row?.assignment?.dueDate ?? ''
  reason.value = ''
  if (!assigneeId.value) assigneeId.value = props.candidates[0]?.userId ?? ''
}, { immediate: true })

watch(() => props.candidates, (items) => {
  if (!assigneeId.value && items.length) assigneeId.value = items[0]?.userId ?? ''
})

const candidateHint = computed(() => {
  const item = props.candidates.find((candidate) => candidate.userId === assigneeId.value)
  if (!item) return ''
  const load = item.activeTasks ? `当前在办 ${item.activeTasks} 张` : '当前没有在办图纸'
  return `${roleLabels(item.roles)} · ${load}`
})

const canSubmit = computed(() => Boolean(props.row) && Boolean(assigneeId.value) && !props.busy)

function submit() {
  if (!canSubmit.value) return
  emit('submit', { assigneeId: assigneeId.value, note: note.value.trim(), dueDate: dueDate.value, reason: reason.value.trim() })
}

function statusLabel(status: DrawingTaskHistoryEntry['status']): string {
  if (status === 'active') return '当前负责'
  if (status === 'replaced') return '已改派'
  return '已取消'
}
</script>

<template>
  <div v-if="row" class="assign-dialog-backdrop" role="dialog" aria-modal="true" :aria-label="isReassign ? '改派负责人' : '指派负责人'">
    <div class="card assign-dialog">
      <header class="assign-dialog-head">
        <div>
          <h2>{{ isReassign ? '改派负责人' : '指派负责人' }}</h2>
          <p class="assign-dialog-meta">{{ row.drawing.no }} · {{ row.drawing.name }}</p>
        </div>
        <button class="btn icon-only" type="button" aria-label="关闭" :disabled="busy" @click="emit('close')">
          <DemoIcon name="x" :size="16" />
        </button>
      </header>

      <div class="assign-dialog-body">
        <p class="assign-dialog-current">
          <DemoIcon name="info" :size="13" />
          <span v-if="isReassign">当前由 <b>{{ row.assignment?.assignee }}</b> 负责；指派给他人即完成改派，原负责人会收到通知并失去这张图纸的控制权。</span>
          <span v-else>指派后负责人将获得这张图纸的决定控制权：可编辑、上传文件、发起审核，并在自己的首页看到任务与进度。</span>
        </p>

        <label class="assign-field">
          <span>负责人 *</span>
          <select v-model="assigneeId" class="inp" :disabled="busy || loadingCandidates">
            <option value="" disabled>{{ loadingCandidates ? '正在加载人员…' : '请选择负责人' }}</option>
            <option v-for="candidate in candidates" :key="candidate.userId" :value="candidate.userId">
              {{ candidate.name }}（{{ candidate.account }}）
            </option>
          </select>
          <small v-if="candidateHint" class="assign-hint">{{ candidateHint }}</small>
          <small v-else-if="!loadingCandidates && !candidates.length" class="assign-hint warn">
            没有可指派的在职人员。请先在后台管理中为账号添加「设计人员」「计划员」或「管理员」身份。
          </small>
        </label>

        <label class="assign-field">
          <span>任务说明</span>
          <input v-model="note" class="inp" maxlength="200" placeholder="例如：先出总图，零件图本周内补齐" :disabled="busy" />
        </label>

        <label class="assign-field">
          <span>截止日期</span>
          <input v-model="dueDate" class="inp" type="date" :disabled="busy" />
        </label>

        <label v-if="isReassign" class="assign-field">
          <span>改派说明</span>
          <input v-model="reason" class="inp" maxlength="200" placeholder="例如：原负责人休假，转交他人" :disabled="busy" />
        </label>

        <section class="assign-history">
          <h3>指派历史</h3>
          <p v-if="loadingHistory" class="assign-hint">正在加载指派历史…</p>
          <p v-else-if="!history.length" class="assign-hint">这张图纸还没有指派记录。</p>
          <ul v-else>
            <li v-for="item in history" :key="item.taskId">
              <span class="tag" :class="item.status === 'active' ? 'ok' : 'plain'">{{ statusLabel(item.status) }}</span>
              <span class="assign-history-name">{{ item.assignee }}</span>
              <span class="assign-history-time">{{ item.assignedAt ? new Date(item.assignedAt).toLocaleString() : '—' }}</span>
              <span v-if="item.assignedByName" class="assign-history-by">指派人：{{ item.assignedByName }}</span>
              <span v-if="item.endReason" class="assign-history-reason">{{ item.endReason }}</span>
            </li>
          </ul>
        </section>

        <p v-if="error" class="assign-error" role="alert">{{ error }}</p>
      </div>

      <footer class="assign-dialog-actions">
        <button class="btn" type="button" :disabled="busy" @click="emit('close')">取消</button>
        <button class="btn primary" type="button" :disabled="!canSubmit" @click="submit">
          <span v-if="busy" class="button-spinner" aria-hidden="true"></span>
          <DemoIcon v-else name="user-check" :size="14" />
          {{ busy ? '提交中…' : isReassign ? '确认改派' : '确认指派' }}
        </button>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.assign-dialog-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  z-index: 60;
}
.assign-dialog {
  width: min(560px, 100%);
  max-height: 88vh;
  display: flex;
  flex-direction: column;
}
.assign-dialog-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 18px 10px;
  border-bottom: 1px solid var(--border);
}
.assign-dialog-head h2 {
  font-family: var(--font-display);
  font-size: 15px;
  font-weight: 800;
}
.assign-dialog-meta {
  color: var(--text-3);
  font-size: 11.5px;
  margin-top: 2px;
}
.assign-dialog-body {
  padding: 14px 18px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.assign-dialog-current {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  font-size: 12px;
  color: var(--text-2);
  background: var(--surface-2, rgba(148, 163, 184, 0.12));
  border-radius: 8px;
  padding: 9px 11px;
  line-height: 1.6;
}
.assign-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  font-size: 12px;
  color: var(--text-2);
}
.assign-hint {
  color: var(--text-3);
  font-size: 11.5px;
}
.assign-hint.warn {
  color: var(--danger, #dc2626);
}
.assign-history h3 {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-2);
  margin-bottom: 6px;
}
.assign-history ul {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.assign-history li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 11.5px;
  color: var(--text-3);
}
.assign-history-name {
  color: var(--text-2);
  font-weight: 600;
}
.assign-history-reason {
  width: 100%;
  color: var(--text-3);
}
.assign-error {
  color: var(--danger, #dc2626);
  font-size: 12px;
}
.assign-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 18px 16px;
  border-top: 1px solid var(--border);
}
</style>
