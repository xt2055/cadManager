<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import type { SelectOption } from '@/features/operation-logs/operation-log.helpers'

defineOptions({ name: 'CandidateInput' })

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
    loading?: boolean
    options: SelectOption[]
  }>(),
  {
    placeholder: '请输入或选择',
    loading: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'search', keyword: string): void
  (e: 'select', value: string): void
  (e: 'enter'): void
}>()

const isOpen = ref(false)
const isFocused = ref(false)
const activeIndex = ref(-1)
const rootRef = ref<HTMLElement | null>(null)

const displayOptions = computed(() => props.options)

function handleInput(e: Event) {
  const target = e.target as HTMLInputElement
  emit('update:modelValue', target.value)
  emit('search', target.value)
  isOpen.value = true
  activeIndex.value = -1
}

function handleFocus() {
  isFocused.value = true
  isOpen.value = true
  emit('search', props.modelValue)
}

function handleBlur(e: FocusEvent) {
  isFocused.value = false
  // 延迟关闭下拉面板，以便触发点击选项事件
  setTimeout(() => {
    if (!rootRef.value?.contains(document.activeElement)) {
      isOpen.value = false
    }
  }, 200)
}

function selectOption(option: SelectOption) {
  emit('update:modelValue', option.value)
  emit('select', option.value)
  isOpen.value = false
}

function clear() {
  emit('update:modelValue', '')
  emit('search', '')
  emit('select', '')
}

function handleKeydown(e: KeyboardEvent) {
  if (!isOpen.value && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
    isOpen.value = true
    return
  }

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (displayOptions.value.length === 0) return
    activeIndex.value = (activeIndex.value + 1) % displayOptions.value.length
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (displayOptions.value.length === 0) return
    activeIndex.value = (activeIndex.value - 1 + displayOptions.value.length) % displayOptions.value.length
  } else if (e.key === 'Enter') {
    const item = displayOptions.value[activeIndex.value]
    if (activeIndex.value >= 0 && item) {
      e.preventDefault()
      selectOption(item)
    } else {
      isOpen.value = false
      emit('enter')
    }
  } else if (e.key === 'Escape') {
    isOpen.value = false
  }
}
</script>

<template>
  <div ref="rootRef" class="candidate-input-wrap" :class="{ 'is-focused': isFocused, 'is-open': isOpen }">
    <div class="input-inner">
      <input
        type="text"
        class="inp candidate-inp"
        :value="modelValue"
        :placeholder="placeholder"
        @input="handleInput"
        @focus="handleFocus"
        @blur="handleBlur"
        @keydown="handleKeydown"
      />
      <button v-if="modelValue" type="button" class="btn-clear" title="清空" @mousedown.prevent @click="clear">
        <DemoIcon name="x" :size="12" />
      </button>
      <span v-if="loading" class="loading-spinner">
        <DemoIcon name="loader-circle" :size="13" />
      </span>
      <span v-else class="dropdown-arrow" @mousedown.prevent="isOpen = !isOpen">
        <DemoIcon name="chevron-down" :size="13" />
      </span>
    </div>

    <!-- 浮动候选面板 -->
    <transition name="candidate-fade">
      <div v-if="isOpen" class="candidate-dropdown" @mousedown.prevent>
        <div v-if="loading && !displayOptions.length" class="candidate-status">
          <DemoIcon name="loader-circle" :size="14" />
          <span>加载候选中...</span>
        </div>
        <div v-else-if="!displayOptions.length" class="candidate-status empty">
          <span>无匹配候选，回车直接搜索</span>
        </div>
        <ul v-else class="candidate-list">
          <li
            v-for="(option, idx) in displayOptions"
            :key="option.value"
            class="candidate-item"
            :class="{ 'is-active': idx === activeIndex, 'is-selected': option.value === modelValue }"
            @click="selectOption(option)"
          >
            <span class="option-label">{{ option.label }}</span>
            <DemoIcon v-if="option.value === modelValue" name="check" :size="13" class="check-icon" />
          </li>
        </ul>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.candidate-input-wrap {
  position: relative;
  width: 100%;
}

.input-inner {
  position: relative;
  display: flex;
  align-items: center;
  width: 100%;
}

.candidate-inp {
  height: 36px;
  line-height: 36px;
  padding: 0 54px 0 12px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 9px;
  color: var(--text-1);
  font-size: 13px;
  outline: none;
  transition: all 0.25s ease;
}

.candidate-inp::placeholder {
  color: var(--text-3);
  font-size: 12.5px;
}

.candidate-input-wrap.is-focused .candidate-inp,
.candidate-input-wrap.is-open .candidate-inp {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.btn-clear {
  position: absolute;
  right: 26px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--text-3);
  cursor: pointer;
  border-radius: 50%;
  transition: color 0.2s;
}

.btn-clear:hover {
  color: var(--text-1);
}

.loading-spinner {
  position: absolute;
  right: 8px;
  display: flex;
  align-items: center;
  color: var(--accent);
  animation: spin 1s linear infinite;
  pointer-events: none;
}

.dropdown-arrow {
  position: absolute;
  right: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  color: var(--text-3);
  cursor: pointer;
  transition: transform 0.25s ease, color 0.2s;
}

.dropdown-arrow:hover {
  color: var(--accent);
}

.candidate-input-wrap.is-open .dropdown-arrow {
  transform: rotate(180deg);
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 下拉菜单面板 */
.candidate-dropdown {
  position: absolute;
  top: calc(100% + 5px);
  left: 0;
  width: 100%;
  min-width: 200px;
  max-height: 240px;
  overflow-y: auto;
  padding: 4px;
  background: var(--panel-top, var(--panel));
  border: 1px solid var(--line);
  border-radius: 9px;
  box-shadow: var(--shadow);
  backdrop-filter: blur(14px);
  z-index: 999;
}

.candidate-status {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 14px 10px;
  color: var(--text-3);
  font-size: 12px;
}

.candidate-status.empty {
  color: var(--text-3);
  font-style: italic;
}

.candidate-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.candidate-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px 11px;
  margin: 1px 0;
  border-radius: 6px;
  color: var(--text-1);
  font-size: 12.5px;
  cursor: pointer;
  transition: background-color 0.15s, color 0.15s;
}

.candidate-item:hover,
.candidate-item.is-active {
  background: var(--hover);
  color: var(--accent);
}

.candidate-item.is-selected {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}

.option-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.check-icon {
  flex-shrink: 0;
  color: var(--accent);
  margin-left: 8px;
}

/* 动效 */
.candidate-fade-enter-active,
.candidate-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.candidate-fade-enter-from,
.candidate-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
