<script setup lang="ts">
import { computed } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import type { DrawingAttribute } from '@/types/domain.types'

const props = withDefaults(defineProps<{
  attributes: DrawingAttribute[]
  modelValue?: Record<string, string>
  readonly?: boolean
}>(), {
  modelValue: () => ({}),
  readonly: false,
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
</script>

<template>
  <div v-if="activeAttributes.length" class="attributes-form">
    <div class="attributes-form__intro">
      <DemoIcon name="sliders-horizontal" :size="15" />
      <span>图纸业务属性</span>
      <small>按企业属性配置填写；带 * 的属性为必填项</small>
    </div>

    <div class="attributes-grid">
      <div v-for="attribute in activeAttributes" :key="attribute.id" class="attribute-field">
        <label :for="`drawing-attribute-${attribute.id}`">
          <span>{{ attribute.name }}</span>
          <b v-if="attribute.required" class="required-mark">*</b>
          <em v-else>可不填</em>
        </label>

        <select
          :id="`drawing-attribute-${attribute.id}`"
          class="inp"
          :value="valueOf(attribute.id)"
          :disabled="readonly || !enabledFields(attribute).length"
          @change="setValue(attribute.id, ($event.target as HTMLSelectElement).value)"
        >
          <option value="">{{ enabledFields(attribute).length ? '请选择字段选项' : '暂无可用字段，请联系管理员' }}</option>
          <option v-for="field in enabledFields(attribute)" :key="field.id" :value="field.id">
            {{ field.name }}
          </option>
        </select>
      </div>
    </div>
  </div>
</template>

<style scoped>
.attributes-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 16px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: var(--radius);
}

.attributes-form__intro {
  display: flex;
  align-items: center;
  gap: 7px;
  color: var(--accent);
  font-size: 13px;
  font-weight: 700;
}

.attributes-form__intro small {
  margin-left: 5px;
  color: var(--text-3);
  font-size: 11px;
  font-weight: 400;
}

.attributes-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px 18px;
}

.attribute-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.attribute-field label {
  display: flex;
  align-items: baseline;
  gap: 4px;
  color: var(--text-2);
  font-size: 12px;
  font-weight: 600;
}

.attribute-field label em {
  color: var(--text-3);
  font-size: 10px;
  font-style: normal;
  font-weight: 400;
}

.required-mark {
  color: var(--danger);
}

@media (max-width: 900px) {
  .attributes-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 560px) {
  .attributes-grid { grid-template-columns: 1fr; }
  .attributes-form__intro { align-items: flex-start; flex-wrap: wrap; }
  .attributes-form__intro small { width: 100%; margin-left: 22px; }
}
</style>
