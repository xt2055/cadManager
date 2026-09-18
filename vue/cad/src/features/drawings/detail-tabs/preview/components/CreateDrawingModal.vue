<script setup lang="ts">
import { nextTick, useTemplateRef } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { NEW_DRAWING_NAME_PRESETS } from '../new-drawing-name-presets'

defineOptions({ name: 'CreateDrawingModal' })

/**
 * 新建图纸弹窗：表单态、创建进度与错误都只做展示。
 * 不引入任何 store / composable（名称预设是静态常量表，不是状态）；名称、图号前缀与图号后几位
 * 经 update 事件回抛给持有状态的 useDrawingCreation，提交只发 submit 意图。
 * 项目号（前缀）由页面按当前图纸带出，用户只需填后几位；完整图号 drawingNo 由外部拼好，这里只做展示。
 * Teleport 也收在组件内部——弹窗自己的外壳样式已经自包含，
 * 父页面不需要为了 scope id 而替它包一层 <Teleport>。
 */
defineProps<{
  /** 正在创建（含校验/上传/转换/打开四个阶段）。 */
  creating: boolean
  /** 创建进度：步骤号与文案。 */
  progress: { step: number; title: string; detail: string }
  /** 表单：名称。 */
  name: string
  /** 表单：图号前缀（当前图纸的项目号）。 */
  drawingNoPrefix: string
  /** 表单：图号后几位，用户真正要填的部分。 */
  drawingNoSuffix: string
  /** 已拼好的完整图号，只做展示。 */
  drawingNo: string
  /** 创建失败原因（为空表示无错误）。 */
  error: string
}>()

const emit = defineEmits<{
  close: []
  submit: []
  'update:name': [value: string]
  'update:drawingNoPrefix': [value: string]
  'update:drawingNoSuffix': [value: string]
}>()

/** 三个输入框都只是把原始值回抛给持有状态的一方，取值的写法统一在这里。 */
const inputValue = (event: Event) => (event.target as HTMLInputElement).value

/** 常用零件名称：点一下填入名称框，省掉打字并避免错别字（名称会拼进文件名）。 */
const namePresets = NEW_DRAWING_NAME_PRESETS
// 用 useTemplateRef 显式绑定：模板 ref 依赖「字符串 ref 回写到同名 setup 变量」这条隐式路径，
// 开发态与打包态的编译结果不同，出问题时表现为「点选后焦点交不出去」。显式绑定与编译模式无关。
const nameInput = useTemplateRef<HTMLInputElement>('nameInput')

/**
 * 点选预设：填入基础名后把光标交到名称框末尾，并把焦点也交给它，
 * 这样紧接着敲 1 / A 就直接落进名称框（变体后缀不单独做预设）。
 */
async function applyNamePreset(preset: string) {
  emit('update:name', preset)
  await nextTick()
  const input = nameInput.value
  if (!input) return
  input.focus()
  const end = input.value.length || preset.length
  input.setSelectionRange(end, end)
}
</script>

<template>
  <Teleport to="body">
    <div class="modal-backdrop creation-backdrop">
      <div
        v-if="creating"
        class="modal card drawing-creating-modal"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="drawing-creating-title"
        aria-describedby="drawing-creating-detail"
      >
        <div class="drawing-creating-spinner" aria-hidden="true"><span></span></div>
        <div class="drawing-creating-copy">
          <span class="drawing-creating-step">步骤 {{ progress.step }} / 4</span>
          <h3 id="drawing-creating-title">{{ progress.title }}</h3>
          <p id="drawing-creating-detail">{{ progress.detail }}</p>
          <strong>{{ drawingNo }} · {{ name }}</strong>
        </div>
        <div class="drawing-creating-track" aria-hidden="true"><i :style="{ width: `${progress.step * 25}%` }"></i></div>
        <p class="drawing-creating-warning"><DemoIcon name="info" :size="15" />创建期间请勿刷新、返回或重复操作</p>
      </div>
      <form
        v-else
        class="modal card new-drawing-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="new-drawing-title"
        @submit.prevent="emit('submit')"
      >
        <div class="modal-head"><h3 id="new-drawing-title">新建图纸</h3></div>
        <p>使用 CAXA 空白模板创建草稿零件，并在本机 CAXA 中绘制。保存后点击「结束编辑」回写版本。</p>
        <div v-if="error" class="drawing-creation-error" role="alert"><DemoIcon name="alert-triangle" :size="16" />{{ error }}</div>
        <label>
          名称<input
            class="inp"
            required
            ref="nameInput"
            maxlength="200"
            autofocus
            :value="name"
            @input="emit('update:name', inputValue($event))"
          />
        </label>
        <div class="new-drawing-presets">
          <span class="presets-label">常用名称 · 点选填入，可接着补 1 / A</span>
          <div class="preset-chips">
            <button
              v-for="preset in namePresets"
              :key="preset"
              class="preset-chip"
              :class="{ active: preset === name.trim() }"
              type="button"
              @click="applyNamePreset(preset)"
            >{{ preset }}</button>
          </div>
        </div>
        <div class="new-drawing-no">
          <label>
            项目号<input
              class="inp mono"
              maxlength="100"
              :value="drawingNoPrefix"
              @input="emit('update:drawingNoPrefix', inputValue($event))"
            />
          </label>
          <label>
            图号后几位<input
              class="inp mono"
              required
              maxlength="100"
              placeholder="例如 01"
              :value="drawingNoSuffix"
              @input="emit('update:drawingNoSuffix', inputValue($event))"
            />
          </label>
        </div>
        <div class="new-drawing-preview">
          <span class="preview-label">完整图号</span>
          <b class="mono">{{ drawingNo || '—' }}</b>
        </div>
        <p class="new-drawing-hint"><DemoIcon name="info" :size="14" />项目号已按当前图纸图号带出，只需填写图号后几位；借用件可直接粘贴完整图号。</p>
        <div class="modal-foot">
          <button class="btn" type="button" @click="emit('close')">取消</button>
          <button class="btn primary" type="submit">创建并本地编辑</button>
        </div>
      </form>
    </div>
  </Teleport>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 只放本弹窗专属样式；弹窗外壳来自上面的共享 modal-chrome。 */
.new-drawing-modal {
  width: min(460px, calc(100vw - 32px));
  /* 名称预设让弹窗变高：内容超出全局 .modal 的 84vh 上限时由弹窗自己滚动。 */
  max-height: 84vh;
  padding: 24px;
  display: grid;
  gap: 18px;
  overflow-y: auto;
}
.new-drawing-modal p { color: var(--text-2); line-height: 1.6; margin: 0; }
.new-drawing-modal label { display: grid; gap: 8px; }

/* 名称预设：固定名称点选填入，省掉打字与错别字。 */
.new-drawing-modal .new-drawing-presets { display: grid; gap: 6px; }
.presets-label { color: var(--text-3); font-size: 11px; }
.preset-chips { display: flex; flex-wrap: wrap; gap: 6px; max-height: 116px; overflow-y: auto; padding-right: 2px; }
.preset-chip { flex: none; padding: 3px 9px; border: 1px solid var(--line); border-radius: 999px; background: var(--panel-2); color: var(--text-2); font-size: 11px; }
.preset-chip:hover { border-color: var(--accent); color: var(--accent); }
.preset-chip.active { border-color: var(--accent); background: var(--accent-soft); color: var(--accent); font-weight: 600; }

/* 项目号 + 后几位两列并排：前缀已带出，视线只需落在右边一格。 */
.new-drawing-no { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 10px; }
.new-drawing-no label { min-width: 0; }
.new-drawing-no .inp { font-size: 12.5px; }
.new-drawing-modal .new-drawing-preview { display: flex; align-items: baseline; gap: 8px; margin: 0; font-size: 11.5px; color: var(--text-3); }
.new-drawing-preview .preview-label { flex: none; }
.new-drawing-preview b { min-width: 0; overflow-wrap: anywhere; color: var(--accent); font-size: 12.5px; }
.new-drawing-modal p.new-drawing-hint { display: flex; align-items: center; gap: 6px; margin: 0; color: var(--text-3); font-size: 11.5px; line-height: 1.5; }
.new-drawing-hint :deep(svg) { flex: none; color: var(--accent); }
.new-drawing-modal .modal-foot { display: flex; justify-content: flex-end; gap: 10px; }
/* 旧结构里 z-index: 2900 被父页面同优先级的 .modal-backdrop（z-index: 1000，源码在后）整条覆盖，
   从未生效；为保持重构前后渲染一致，这里只保留真正生效的 cursor。 */
.creation-backdrop { cursor: wait; }
.new-drawing-modal { cursor: default; }
.drawing-creating-modal { width: min(480px, calc(100vw - 32px)); padding: 30px; display: grid; justify-items: center; gap: 16px; text-align: center; cursor: wait; }
.drawing-creating-spinner { width: 54px; height: 54px; padding: 5px; border-radius: 50%; background: conic-gradient(var(--accent), transparent 65%); animation: drawing-creating-spin .85s linear infinite; }
.drawing-creating-spinner span { display: block; width: 100%; height: 100%; border-radius: 50%; background: var(--panel); }
.drawing-creating-copy { display: grid; justify-items: center; gap: 7px; }
.drawing-creating-copy h3, .drawing-creating-copy p { margin: 0; }
.drawing-creating-copy p { color: var(--text-2); line-height: 1.6; }
.drawing-creating-copy strong { color: var(--text-1); font-family: 'JetBrains Mono', monospace; overflow-wrap: anywhere; }
.drawing-creating-step { color: var(--accent); font-size: 12px; font-weight: 700; }
.drawing-creating-track { width: 100%; height: 5px; overflow: hidden; border-radius: 999px; background: var(--panel-2); }
.drawing-creating-track i { display: block; height: 100%; border-radius: inherit; background: var(--accent); transition: width .35s ease; }
.drawing-creating-warning, .drawing-creation-error { display: flex; align-items: center; gap: 8px; }
.drawing-creating-warning { color: var(--text-2); font-size: 12px; margin: 0; }
.drawing-creation-error { padding: 10px 12px; border: 1px solid color-mix(in srgb, var(--danger) 45%, transparent); border-radius: 6px; color: var(--danger); background: color-mix(in srgb, var(--danger) 8%, transparent); }
@keyframes drawing-creating-spin { to { transform: rotate(360deg); } }
</style>
