<script setup lang="ts">
import { computed } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { DrawingAttribute } from '@/types/domain.types'

const props = withDefaults(defineProps<{
  attributes: DrawingAttribute[]
  modelValue?: Record<string, string>
  readonly?: boolean
  compact?: boolean
  title?: string
  description?: string
}>(), {
  modelValue: () => ({}),
  readonly: false,
  compact: false,
  title: '图纸业务属性',
  description: '带 * 的属性为必填项；未选或停用的属性不影响历史已录入数据',
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: Record<string, string>): void
}>()

const activeAttributes = computed(() => props.attributes.filter((attribute) => attribute.enabled))

function valueOf(attributeId: string): string {
  return props.modelValue[attributeId] ?? ''
}

function setValue(attributeId: string, value: string) {
  emit('update:modelValue', { ...props.modelValue, [attributeId]: value })
}

function enabledFields(attribute: DrawingAttribute) {
  return attribute.fields
    .filter((field) => field.enabled)
    .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'zh-CN'))
}

function getFieldName(attribute: DrawingAttribute, fieldId: string): string {
  if (!fieldId) return ''
  const field = attribute.fields.find((item) => item.id === fieldId)
  return field ? field.name : '（已失效选项）'
}
</script>

<template>
  <div v-if="attributes.length" class="attributes-wrapper" :class="{ compact }">
    <!-- 编辑录入态 -->
    <div v-if="!readonly" class="attributes-form">
      <div class="attributes-form__header">
        <div class="header-main">
          <span class="header-icon"><DemoIcon name="sliders-horizontal" :size="14" /></span>
          <span class="header-title">{{ title }}</span>
          <span class="header-badge">{{ activeAttributes.length }} 项配置</span>
        </div>
        <div class="header-sub">{{ description }}</div>
      </div>

      <div class="attributes-grid">
        <div
          v-for="attribute in activeAttributes"
          :key="attribute.id"
          class="attribute-control-group"
          :class="{
            'is-required': attribute.required,
            'is-filled': Boolean(valueOf(attribute.id)),
            'no-options': !enabledFields(attribute).length,
          }"
        >
          <div class="attribute-label-row">
            <label :for="`drawing-attr-${attribute.id}`" class="attribute-label">
              <span class="label-name">{{ attribute.name }}</span>
            </label>
            <span v-if="attribute.required" class="badge-req" title="创建与保存必填">必填</span>
            <span v-else class="badge-opt">选填</span>
          </div>

          <div class="select-field-wrap">
            <select
              :id="`drawing-attr-${attribute.id}`"
              class="inp attribute-select"
              :value="valueOf(attribute.id)"
              :disabled="!enabledFields(attribute).length"
              @change="setValue(attribute.id, ($event.target as HTMLSelectElement).value)"
            >
              <option value="">{{ enabledFields(attribute).length ? `请选择${attribute.name}` : '暂无可选字段' }}</option>
              <option v-for="field in enabledFields(attribute)" :key="field.id" :value="field.id">
                {{ field.name }}
              </option>
            </select>
            <span class="select-indicator"><DemoIcon name="chevron-down" :size="13" /></span>
          </div>
        </div>
      </div>
    </div>

    <!-- 只读规格展示态（Spec Mode） -->
    <div v-else class="attributes-specs">
      <div class="specs-grid">
        <div
          v-for="attribute in attributes"
          :key="attribute.id"
          class="spec-card"
          :class="{
            'has-value': Boolean(valueOf(attribute.id)),
            'is-empty': !valueOf(attribute.id),
          }"
        >
          <div class="spec-label">
            <span class="spec-name">{{ attribute.name }}</span>
            <span v-if="attribute.required" class="spec-req-dot" title="必填属性" />
          </div>
          <div class="spec-value">
            <template v-if="valueOf(attribute.id)">
              <span class="spec-pill">
                <DemoIcon name="tag" :size="12" />
                {{ getFieldName(attribute, valueOf(attribute.id)) }}
              </span>
            </template>
            <template v-else>
              <span class="spec-placeholder">未指定</span>
            </template>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.attributes-wrapper {
  width: 100%;
}

/* ================= 编辑表单视觉 ================= */
.attributes-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px 18px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: var(--radius);
  transition: border-color 0.2s;
}

.attributes-form__header {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-bottom: 10px;
  border-bottom: 1px dashed var(--line);
}

.header-main {
  display: flex;
  align-items: center;
  gap: 8px;
}

.header-icon {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
}

.header-title {
  color: var(--text-1);
  font-size: 13px;
  font-weight: 700;
}

.header-badge {
  padding: 2px 7px;
  border-radius: 10px;
  background: var(--active);
  color: var(--accent);
  font-size: 10.5px;
  font-family: 'JetBrains Mono', monospace;
  font-weight: 600;
}

.header-sub {
  color: var(--text-3);
  font-size: 11px;
  line-height: 1.4;
}

.attributes-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 12px 16px;
}

.attribute-control-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.attribute-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.attribute-label {
  display: flex;
  align-items: center;
  min-width: 0;
  cursor: pointer;
}

.label-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 600;
}

.badge-req {
  padding: 1px 5px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--warn) 15%, transparent);
  color: var(--warn);
  font-size: 10px;
  font-weight: 600;
  white-space: nowrap;
}

.badge-opt {
  color: var(--text-3);
  font-size: 10px;
  white-space: nowrap;
}

.select-field-wrap {
  position: relative;
  display: flex;
  align-items: center;
}

.attribute-select {
  width: 100%;
  padding-right: 28px;
  font-size: 12.5px;
  background: var(--panel);
  border-color: var(--line);
  transition: all 0.18s;
  cursor: pointer;
}

.attribute-select:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-soft);
}

.attribute-control-group.is-filled .attribute-select {
  border-color: color-mix(in srgb, var(--accent) 60%, var(--line));
  font-weight: 500;
}

.select-indicator {
  position: absolute;
  right: 10px;
  color: var(--text-3);
  pointer-events: none;
  display: flex;
  align-items: center;
}

/* ================= 只读规格展示视觉 ================= */
.attributes-specs {
  width: 100%;
}

.specs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px 14px;
}

.spec-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 9px 12px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--panel-2);
  transition: background 0.18s, border-color 0.18s;
}

.spec-card.has-value {
  border-color: color-mix(in srgb, var(--accent) 30%, var(--line));
}

.spec-label {
  display: flex;
  align-items: center;
  gap: 4px;
}

.spec-name {
  color: var(--text-3);
  font-size: 11px;
  font-weight: 500;
}

.spec-req-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--warn);
}

.spec-value {
  display: flex;
  align-items: center;
  min-height: 24px;
}

.spec-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--accent-soft);
  border: 1px solid color-mix(in srgb, var(--accent) 25%, transparent);
  color: var(--accent);
  font-size: 12px;
  font-weight: 600;
}

.spec-placeholder {
  color: var(--text-3);
  font-size: 11.5px;
  font-style: italic;
  opacity: 0.7;
}

/* ================= 紧凑模式 ================= */
.attributes-wrapper.compact .attributes-form {
  padding: 10px 12px;
  gap: 10px;
}

.attributes-wrapper.compact .attributes-grid {
  grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
  gap: 8px 10px;
}

@media (max-width: 640px) {
  .attributes-grid,
  .specs-grid {
    grid-template-columns: 1fr;
  }
}
</style>
