<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { lifecycleApi, type LifecycleTree, type LifecycleSubmission } from '@/services/lifecycle.service'
import { RouteName } from '@/router/route-names'
import { useDrawingStore } from '@/stores/drawing.store'

const props = defineProps<{ drawingNo: string; submissionId: string }>()
const emit = defineEmits<{ ready: [ready: boolean] }>()
const router = useRouter()
const drawingStore = useDrawingStore()
const cadPattern = /\.(exb|dwg|dxf)$/i
const submission = ref<LifecycleSubmission | null>(null)
const reason = ref('')
const error = ref('')
let generation = 0

void drawingStore.load().catch(() => undefined)

const fileOwners = computed(() => {
  const map = new Map<string, string>()
  for (const drawing of drawingStore.drawings) {
    for (const file of [...drawing.files, ...drawing.otherFiles]) map.set(file.id, drawing.no)
  }
  for (const part of drawingStore.parts) {
    for (const file of [...part.files, ...part.otherFiles]) map.set(file.id, part.no)
  }
  return map
})

async function load() {
  const seq = ++generation
  emit('ready', false)
  error.value = ''
  submission.value = null
  try {
    const result = await lifecycleApi<LifecycleTree>(`/lifecycle-tree?drawingNo=${encodeURIComponent(props.drawingNo)}`)
    if (seq !== generation) return
    const change = result.changes.find((c) => c.submissions.some((s) => s.id === props.submissionId))
    const snapshot = change?.submissions.find((s) => s.id === props.submissionId)
    if (!change || !snapshot) throw new Error('未找到本轮冻结提交，暂不能签署，请刷新。')
    submission.value = snapshot
    reason.value = change.reason
    emit('ready', true)
  } catch (e) {
    if (seq === generation) error.value = (e as Error).message
  }
}

watch(() => [props.drawingNo, props.submissionId], load, { immediate: true })

function isCad(name: string) {
  return cadPattern.test(name)
}

function browse(versionId: string | undefined, attachmentId: string) {
  if (!versionId) return
  void router.push({ name: RouteName.DrawingViewer, params: { drawingId: props.drawingNo }, query: { fileId: attachmentId, versionId, reviewCaseId: submission.value?.review?.id, from: 'review' } })
}

function compare(file: LifecycleSubmission['files'][number]) {
  if (!file.baseVersionId || !file.submittedVersionId) return
  const reviewCaseId = submission.value?.review?.id
  void router.push({
    name: RouteName.DrawingCompare,
    params: { drawingId: props.drawingNo },
    query: { fileId: file.attachmentId, submissionId: props.submissionId, from: 'review', ...(reviewCaseId ? { reviewCaseId } : {}) },
  })
}
</script>

<template>
  <section class="change-evidence card">
    <div class="evidence-head">
      <h3>本次变更审核</h3>
      <span v-if="submission" class="evidence-tag">冻结提交 · 第 {{ submission.round }} 轮</span>
    </div>
    <p class="evidence-note">请确认下列变更要求后签署。图纸库中的正式在用文件在全部审核通过前保持原版本。</p>
    <p v-if="error" role="alert" class="evidence-error">
      {{ error }} <button class="btn sm" type="button" @click="load">重新读取</button>
    </p>
    <template v-if="submission">
      <p class="evidence-time">{{ new Date(submission.createdAt).toLocaleString() }}</p>
      <dl class="evidence-fields">
        <div><dt>变更原因</dt><dd>{{ reason || '未填写' }}</dd></div>
        <div><dt>实际修改</dt><dd>{{ submission.actualChanges || '未填写' }}</dd></div>
        <div v-for="(value, key) in submission.proposedAttributes" :key="key">
          <dt>{{ ({ name: '名称', material: '材料', vendor: '供应商' } as Record<string, string>)[key] || key }}</dt>
          <dd>{{ value }}</dd>
        </div>
      </dl>
      <div v-if="submission.files.length" class="evidence-files">
        <div class="evidence-files-label">涉及图纸文件（{{ submission.files.length }}）</div>
        <div v-for="file in submission.files" :key="file.attachmentId" class="file">
          <span v-if="fileOwners.get(file.attachmentId)" class="file-owner">{{ fileOwners.get(file.attachmentId) }}</span>
          <strong>{{ file.name }}</strong>
          <template v-if="isCad(file.name)">
            <button class="btn sm" type="button" :disabled="!file.submittedVersionId" @click="browse(file.submittedVersionId, file.attachmentId)">浏览并批注</button>
            <button class="btn sm" type="button" :disabled="!file.baseVersionId || !file.submittedVersionId" @click="compare(file)">变更前后对比</button>
          </template>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.change-evidence { padding: 18px 20px; margin: 14px 0; border: 1px solid #a4bdda; }
.evidence-head { display: flex; align-items: center; gap: 10px; margin-bottom: 6px; }
.evidence-head h3 { margin: 0; font-size: 15px; }
.evidence-tag { padding: 3px 9px; border-radius: 999px; background: var(--accent-soft); color: var(--accent); font-size: 11.5px; font-weight: 700; }
.evidence-note { margin: 0; color: var(--text-3); font-size: 12px; }
.evidence-error { margin: 8px 0 0; color: var(--danger); font-size: 12.5px; }
.evidence-time { margin: 8px 0 0; color: var(--text-3); font-size: 12px; }
.evidence-fields { display: grid; gap: 8px; margin: 12px 0 0; }
.evidence-fields div { display: flex; gap: 10px; }
.evidence-fields dt { width: 72px; flex: none; color: var(--text-3); font-size: 12px; }
.evidence-fields dd { margin: 0; font-size: 12.5px; white-space: pre-wrap; }
.evidence-files { display: flex; flex-direction: column; gap: 8px; margin-top: 14px; }
.evidence-files-label { color: var(--text-3); font-size: 12px; font-weight: 600; }
.file { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.file-owner { padding: 2px 8px; border-radius: 999px; background: var(--panel-2); color: var(--text-2); font-size: 11.5px; font-weight: 600; font-family: 'JetBrains Mono', monospace; }
.file strong { font-size: 13px; }
</style>
