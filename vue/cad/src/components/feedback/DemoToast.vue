<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DemoToast',
})

const uiStore = useUiStore()

const icons = {
  ok: 'check-circle',
  info: 'info',
  warn: 'alert-triangle',
} as const
</script>

<template>
  <Teleport to="body">
  <div id="toastWrap" role="status" aria-live="polite">
    <div v-for="toast in uiStore.toasts" :key="toast.id" class="toast" :class="toast.type">
      <DemoIcon :name="icons[toast.type]" :size="16" />
      <span>{{ toast.message }}</span>
    </div>
  </div>
  </Teleport>
</template>
