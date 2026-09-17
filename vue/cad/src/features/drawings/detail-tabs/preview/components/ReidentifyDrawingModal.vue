<script setup lang="ts">
import { computed } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'ReidentifyDrawingModal' })

/**
 * 批量校正图号弹窗：只做展示与「勾选意图」收集。
 * 不引入任何 store / composable；行数据由父页面从 useDrawingReidentify 投影进来，
 * 勾选动作经 emit 回抛，由父页面交给持有状态的 composable 落账。
 */
const props = defineProps<{
  /** 待校正行（父页面：reidentifyList 的投影，id 为文件 id）。 */
  rows: { id: string; name: string; oldPartNo: string; newPartNo: string; checked: boolean }[]
  /** 未能从文件名识别出规范图号的文件说明（已忽略）。 */
  failures: string[]
  /** 正在执行校正：关闭与确认同时禁用。 */
  executing: boolean
}>()

const emit = defineEmits<{
  close: []
  confirm: []
  /** 勾选 / 取消勾选一行。 */
  toggle: [id: string, checked: boolean]
  /** 表头全选 / 全不选。 */
  toggleAll: [checked: boolean]
}>()

const allChecked = computed(() => props.rows.length > 0 && props.rows.every((row) => row.checked))
const checkedCount = computed(() => props.rows.filter((row) => row.checked).length)
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card reidentify-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="scan" :size="18" />
          <span>批量校正零件图号</span>
        </div>
        <button class="btn sm close-btn" type="button" :disabled="executing" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body reidentify-modal-body">
        <div class="reidentify-hint">
          <DemoIcon name="info" :size="14" />
          <span>系统已按图纸文件名识别图号，并已自动过滤明细表和非零件图文件。请核对并勾选需校正的项：</span>
        </div>

        <div class="reidentify-table-wrap">
          <table class="tbl compact-tbl">
            <thead>
              <tr>
                <th style="width: 40px; text-align: center">
                  <input
                    type="checkbox"
                    :checked="allChecked"
                    @change="emit('toggleAll', ($event.target as HTMLInputElement).checked)"
                  />
                </th>
                <th>文件名</th>
                <th>当前关联图号</th>
                <th>识别图号 (文件名)</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in rows" :key="row.id">
                <td style="text-align: center">
                  <input
                    type="checkbox"
                    :checked="row.checked"
                    @change="emit('toggle', row.id, ($event.target as HTMLInputElement).checked)"
                  />
                </td>
                <td class="file-name-cell">
                  <DemoIcon name="file" :size="14" />
                  <span>{{ row.name }}</span>
                </td>
                <td class="num mono text-muted">{{ row.oldPartNo }}</td>
                <td class="num mono bold text-accent">{{ row.newPartNo }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="failures.length" class="reidentify-fail-box">
          <div class="fail-title">
            <DemoIcon name="alert-triangle" :size="13" />
            <span>以下 {{ failures.length }} 个文件未能从文件名识别出规范图号（已忽略）：</span>
          </div>
          <ul>
            <li v-for="(msg, idx) in failures" :key="idx">{{ msg }}</li>
          </ul>
        </div>
      </div>

      <div class="modal-foot">
        <button class="btn" type="button" :disabled="executing" @click="emit('close')">取消</button>
        <button class="btn primary" type="button" :disabled="executing" @click="emit('confirm')">
          <DemoIcon name="check" :size="14" />
          {{ executing ? '校正中...' : `确认校正 (${checkedCount} 项)` }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 只放本弹窗专属样式；弹窗外壳来自上面的共享 modal-chrome。 */
.reidentify-modal {
  width: 680px;
  max-width: 92vw;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  max-height: 85vh;
}

.reidentify-modal-body {
  /* padding / gap 沿用共享 modal-chrome 的 .modal-body：此处原声明过 padding 16px 20px 与 gap 12px，
     但它们在旧结构里被父页面 :deep(.modal-body)（同优先级、源码在后）整条覆盖，从未生效；
     抽出组件时按「不改变渲染」处理而不恢复，恢复应当作独立的视觉调整另开一刀。 */
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  min-height: 0;
}

.reidentify-hint {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: 12.5px;
  color: var(--text-2);
  line-height: 1.5;
  background: var(--panel-2);
  padding: 8px 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.reidentify-table-wrap {
  border: 1px solid var(--line);
  border-radius: 6px;
  overflow: hidden;
  max-height: 320px;
  overflow-y: auto;
}

.compact-tbl th,
.compact-tbl td {
  padding: 8px 10px;
  font-size: 12.5px;
}

.text-muted {
  color: var(--text-3);
}

.reidentify-fail-box {
  background: rgba(234, 179, 8, 0.08);
  border: 1px dashed rgba(234, 179, 8, 0.35);
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  color: var(--text-2);
}

.reidentify-fail-box .fail-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--warn, #eab308);
  margin-bottom: 6px;
}

.reidentify-fail-box ul {
  margin: 0;
  padding-left: 18px;
  color: var(--text-3);
  max-height: 80px;
  overflow-y: auto;
}

/* .file-name-cell 与父页面文件表（DrawingPreviewTab）共用，父页面那份带的是它的 scope id、
   作用不到本组件，因此这里保留一份内容相同的副本。 */
.file-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

/* 图标由 DemoIcon 渲染（根节点是 lucide 的 <svg>），显式 :deep() 命中，
   不依赖「组件根节点继承使用方 scope id」的跨层级推断。 */
.file-name-cell :deep(svg) {
  color: var(--accent);
}

.file-name-cell b {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}
</style>
