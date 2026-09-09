<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { extractTitleFields, type TitleSpace, type TitleField } from '../../detail-tabs/preview/cad-title-block'

const props = defineProps<{ spaces: TitleSpace[]; activeSpaceId: string; fileName: string }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const selectedSpaceId = ref('')
const fields = ref<TitleField[]>([])
const message = ref('')
const space = computed(() => props.spaces.find(item => item.id === selectedSpaceId.value))
const recognizedCount = computed(() => fields.value.filter(field => field.source !== '未识别').length)
watch(() => props.spaces, () => {
  selectedSpaceId.value = props.spaces.find(item => item.id === props.activeSpaceId)?.id ?? props.spaces[0]?.id ?? ''
}, { immediate: true })
watch(space, value => {
  fields.value = extractTitleFields(value?.texts ?? [])
  message.value = ''
}, { immediate: true })
async function copy() {
  const text = [`文件：${props.fileName}`, `空间：${space.value?.name ?? ''}`, ...fields.value.map(field => `${field.label}：${field.value || '未填写'}`)].join('\n')
  try {
    await navigator.clipboard.writeText(text)
    message.value = '已复制当前表单；未修改原图及系统记录。'
  } catch {
    message.value = '复制失败，请直接选择输入框中的文字进行复制。'
  }
}
</script>

<template>
  <aside class="drawing-info-panel" aria-label="图纸信息提取结果">
    <header>
      <strong>图纸信息提取</strong>
      <button type="button" class="btn" aria-label="关闭图纸信息面板" @click="emit('close')">关闭</button>
    </header>
    <div class="info-content">
      <p class="source-file">{{ fileName }}</p>
      <p class="info-hint">从当前 CAD 文字和块属性提取。请对照标题栏核对；修改仅作用于本面板，不会覆盖图号或审批记录。</p>
      <label class="info-field">
        <span>图纸空间</span>
        <select v-model="selectedSpaceId">
          <option v-for="item in spaces" :key="item.id" :value="item.id">{{ item.name }}（{{ item.texts.length }} 条文字）</option>
        </select>
      </label>
      <p v-if="!space?.texts.length" class="info-warning">当前空间没有可读取的文字。可切换布局；若文字已转成线条或图片，本次无法直接识别。</p>
      <p v-else-if="!recognizedCount" class="info-warning">读取到文字，但未定位到受支持的标题栏。请核对图纸空间，或手动填写。</p>
      <p v-for="warning in space?.warnings" :key="warning" class="info-warning">{{ warning }}</p>
      <label v-for="field in fields" :key="field.key" class="info-field">
        <span>{{ field.label }}</span>
        <input v-model="field.value" :list="`title-candidates-${field.key}`" :placeholder="field.candidates.length > 1 ? '请选择候选或手动填写' : '未识别，可手动填写'" @input="message = ''; field.source = '人工修改（仅本面板）'" />
        <datalist :id="`title-candidates-${field.key}`">
          <option v-for="candidate in field.candidates" :key="candidate" :value="candidate" />
        </datalist>
        <small>{{ field.source }}</small>
      </label>
    </div>
    <footer>
      <button type="button" class="btn primary" @click="copy">复制当前信息</button>
      <p role="status">{{ message || '识别结果需人工核对，未使用 OCR。' }}</p>
    </footer>
  </aside>
</template>

<style scoped>
.drawing-info-panel { width: 340px; max-width: 85vw; flex-shrink: 0; display: flex; flex-direction: column; min-height: 0; background: var(--panel); border-left: 1px solid var(--line); color: var(--text-1); }
header { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; border-bottom: 1px solid var(--line); }
.info-content { overflow-y: auto; padding: 12px 16px; }
.source-file { margin: 0 0 8px; overflow-wrap: anywhere; font-size: 13px; }
.info-hint, footer p { color: var(--text-2); font-size: 12px; line-height: 1.6; }
.info-warning { font-size: 13px; line-height: 1.6; color: var(--text-1); padding: 10px; border: 1px solid var(--line); border-radius: 6px; }
.info-field { display: flex; flex-direction: column; gap: 5px; margin: 14px 0; font-size: 13px; }
.info-field input, .info-field select { width: 100%; box-sizing: border-box; padding: 8px 10px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel); color: var(--text-1); }
.info-field small { color: var(--text-2); font-size: 11px; }
footer { padding: 12px 16px; border-top: 1px solid var(--line); }
footer p { margin: 8px 0 0; }
</style>
