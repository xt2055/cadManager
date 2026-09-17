<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { CREATE_MODE_OPTIONS, type DrawingCreateMode } from '@/features/drawings/create/drawing-create-modes'

defineOptions({ name: 'CreateDrawingDialog' })

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (event: 'update:open', value: boolean): void }>()

const router = useRouter()
const createModeEntries = CREATE_MODE_OPTIONS

function close() {
  emit('update:open', false)
}

function select(createMode: DrawingCreateMode) {
  close()
  void router.push({ name: 'drawing-create', query: { mode: createMode } })
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

watch(() => props.open, (value) => {
  if (value) document.addEventListener('keydown', onKeydown)
  else document.removeEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div v-if="open" class="create-overlay" @click.self="close">
    <section class="modal create-modal" role="dialog" aria-modal="true" aria-label="选择创建图纸方式">
      <header class="modal-head">
        <h3>创建图纸</h3>
        <button class="icon-btn" type="button" aria-label="关闭" @click="close"><DemoIcon name="x" :size="17" /></button>
      </header>
      <div class="modal-body">
        <p class="create-modal-hint">选择一种创建方式，进入对应向导。</p>
        <div class="create-mode-cards">
          <button
            v-for="option in createModeEntries"
            :key="option.value"
            class="create-mode-card"
            type="button"
            @click="select(option.value)"
          >
            <span class="create-mode-card-icon"><DemoIcon :name="option.icon" :size="20" /></span>
            <b>{{ option.title }}</b>
            <small>{{ option.sub }}</small>
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.create-overlay {
  position: fixed;
  z-index: 100;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgb(4 8 15 / 55%);
  backdrop-filter: blur(5px);
  animation: fade-in 0.25s;
}

.create-modal {
  width: min(760px, 94vw);
}

.create-modal-hint {
  margin-bottom: 14px;
  color: var(--text-3);
  font-size: 12px;
}

.create-mode-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.create-mode-card {
  display: flex;
  align-items: center;
  flex-direction: column;
  gap: 8px;
  padding: 20px 16px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--panel-2);
  text-align: center;
  transition: all 0.25s;
}

.create-mode-card:hover {
  border-color: var(--accent);
  background: var(--active);
  transform: translateY(-2px);
}

.create-mode-card-icon {
  display: grid;
  width: 42px;
  height: 42px;
  place-items: center;
  border-radius: 11px;
  background: var(--accent-soft);
  color: var(--accent);
}

.create-mode-card b {
  color: var(--text-1);
  font-size: 13px;
}

.create-mode-card small {
  color: var(--text-3);
  font-size: 11px;
  line-height: 1.5;
}

@media (max-width: 640px) {
  .create-mode-cards { grid-template-columns: 1fr; }
}
</style>
