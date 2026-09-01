<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingAttribute } from '@/types/domain.types'

defineOptions({ name: 'AttributeManagementPage' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const query = ref('')
const selectedId = ref('')
const attributeName = ref('')
const attributeRequired = ref(false)
const attributeEnabled = ref(true)
const newFieldName = ref('')
const editingAttribute = ref(false)
const submitting = ref(false)
const editingFieldId = ref('')
const editingFieldName = ref('')

const attributes = computed(() => domainStore.attributes
  .slice()
  .sort((a, b) => a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'zh-CN')))

const filteredAttributes = computed(() => {
  const value = query.value.trim().toLowerCase()
  if (!value) return attributes.value
  return attributes.value.filter((attribute) => attribute.name.toLowerCase().includes(value) || attribute.fields.some((field) => field.name.toLowerCase().includes(value)))
})

const selectedAttribute = computed(() => attributes.value.find((attribute) => attribute.id === selectedId.value) ?? null)

function selectAttribute(attribute: DrawingAttribute) {
  selectedId.value = attribute.id
  attributeName.value = attribute.name
  attributeRequired.value = attribute.required
  attributeEnabled.value = attribute.enabled
  editingAttribute.value = true
  editingFieldId.value = ''
}

function startNewAttribute() {
  selectedId.value = ''
  attributeName.value = ''
  attributeRequired.value = false
  attributeEnabled.value = true
  editingAttribute.value = false
  editingFieldId.value = ''
}

async function saveAttribute() {
  const name = attributeName.value.trim()
  if (!name || submitting.value) return
  submitting.value = true
  try {
    if (editingAttribute.value && selectedId.value) {
      await domainStore.updateAttribute(selectedId.value, { name, required: attributeRequired.value, enabled: attributeEnabled.value })
      uiStore.toast('属性设置已保存', 'ok')
    } else {
      const created = await domainStore.addAttribute(name, attributeRequired.value)
      selectedId.value = created.id
      editingAttribute.value = true
      uiStore.toast(`属性「${name}」已创建`, 'ok')
    }
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '保存属性失败', 'warn')
  } finally {
    submitting.value = false
  }
}

async function addField() {
  if (!selectedAttribute.value || !newFieldName.value.trim() || submitting.value) return
  submitting.value = true
  try {
    await domainStore.addAttributeField(selectedAttribute.value.id, newFieldName.value)
    newFieldName.value = ''
    uiStore.toast('字段选项已添加', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '添加字段失败', 'warn')
  } finally {
    submitting.value = false
  }
}

function startEditField(id: string, name: string) {
  editingFieldId.value = id
  editingFieldName.value = name
}

async function saveField(fieldId: string) {
  if (!selectedAttribute.value || !editingFieldName.value.trim() || submitting.value) return
  submitting.value = true
  try {
    const field = selectedAttribute.value.fields.find((item) => item.id === fieldId)
    await domainStore.updateAttributeField(selectedAttribute.value.id, fieldId, editingFieldName.value, field?.enabled ?? true)
    editingFieldId.value = ''
    uiStore.toast('字段选项已保存', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '保存字段失败', 'warn')
  } finally {
    submitting.value = false
  }
}

async function toggleField(fieldId: string) {
  if (!selectedAttribute.value || submitting.value) return
  const field = selectedAttribute.value.fields.find((item) => item.id === fieldId)
  if (!field) return
  submitting.value = true
  try {
    await domainStore.updateAttributeField(selectedAttribute.value.id, field.id, field.name, !field.enabled)
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '更新字段状态失败', 'warn')
  } finally {
    submitting.value = false
  }
}

function deleteAttribute() {
  if (!selectedAttribute.value) return
  uiStore.confirm(`删除属性「${selectedAttribute.value.name}」？`, '该属性及其字段将从开发数据中移除，已有图纸中的对应值也会被清理。', {
    danger: true,
    confirmText: '确认删除',
    onConfirm: async () => {
      try {
        await domainStore.deleteAttribute(selectedAttribute.value!.id)
        startNewAttribute()
        uiStore.toast('属性已删除', 'ok')
      } catch (error) {
        uiStore.toast(error instanceof Error ? error.message : '删除属性失败', 'warn')
      }
    },
  })
}

onMounted(() => {
  domainStore.initialize().catch(() => undefined)
})
</script>

<template>
  <div class="attribute-admin-page">
    <header class="attribute-page-head">
      <div>
        <div class="eyebrow"><DemoIcon name="sliders-horizontal" :size="13" />图纸元数据配置</div>
        <h1>图纸属性管理</h1>
        <p>属性是业务维度，字段是该属性下的可选值。创建图纸时，所有启用属性会平铺显示。</p>
      </div>
      <button class="btn primary" type="button" @click="startNewAttribute"><DemoIcon name="plus" :size="14" />新建属性</button>
    </header>

    <div class="attribute-workspace">
      <section class="attribute-list-panel card">
        <div class="panel-title-row">
          <div><h2>属性清单</h2><span>{{ attributes.length }} 个属性</span></div>
          <label class="compact-search"><DemoIcon name="search" :size="13" /><input v-model="query" placeholder="搜索属性或字段" /></label>
        </div>
        <div class="attribute-list">
          <button
            v-for="attribute in filteredAttributes"
            :key="attribute.id"
            class="attribute-list-item"
            :class="{ active: attribute.id === selectedId, disabled: !attribute.enabled }"
            type="button"
            @click="selectAttribute(attribute)"
          >
            <span class="attribute-list-icon"><DemoIcon name="sliders-horizontal" :size="15" /></span>
            <span class="attribute-list-main">
              <strong>{{ attribute.name }}</strong>
              <small>{{ attribute.fields.length }} 个字段 · {{ attribute.required ? '必填' : '可不填' }}</small>
            </span>
            <span class="attribute-state" :class="attribute.enabled ? 'on' : 'off'">{{ attribute.enabled ? '启用' : '停用' }}</span>
          </button>
          <div v-if="!filteredAttributes.length" class="empty-list">没有匹配的属性</div>
        </div>
      </section>

      <section class="attribute-editor-panel card">
        <div class="editor-head">
          <div><span class="editor-kicker">{{ editingAttribute ? '编辑属性' : '创建属性' }}</span><h2>{{ editingAttribute ? '属性设置' : '新建图纸属性' }}</h2></div>
          <button v-if="editingAttribute" class="btn sm danger" type="button" @click="deleteAttribute"><DemoIcon name="trash-2" :size="13" />删除</button>
        </div>

        <div class="editor-form">
          <div class="form-field wide">
            <label for="attribute-name">属性名称</label>
            <input id="attribute-name" v-model="attributeName" class="inp" placeholder="例如：安装方式、缸径、工作压力" @keyup.enter="saveAttribute" />
          </div>
          <label class="switch-card">
            <input v-model="attributeRequired" type="checkbox" />
            <span><strong>创建图纸时必填</strong><small>未选择时不能保存图纸</small></span>
          </label>
          <label v-if="editingAttribute" class="switch-card">
            <input v-model="attributeEnabled" type="checkbox" />
            <span><strong>启用此属性</strong><small>停用后不再出现在创建表单</small></span>
          </label>
          <div v-else class="switch-card hint-card"><DemoIcon name="info" :size="15" /><span><strong>创建后继续添加字段</strong><small>例如“安装方式”可添加“耳环、法兰”等选项</small></span></div>
          <button class="btn primary save-attribute" type="button" :disabled="submitting || !attributeName.trim()" @click="saveAttribute"><DemoIcon name="save" :size="14" />{{ editingAttribute ? '保存属性设置' : '创建属性并添加字段' }}</button>
        </div>

        <div v-if="editingAttribute && selectedAttribute" class="fields-editor">
          <div class="fields-head"><div><h3>字段选项</h3><span>创建图纸时从这里选择一个字段</span></div><span class="field-count">{{ selectedAttribute.fields.filter((field) => field.enabled).length }} 个启用</span></div>
          <div class="field-add-row">
            <input v-model="newFieldName" class="inp" placeholder="添加字段，例如：耳环" @keyup.enter="addField" />
            <button class="btn" type="button" :disabled="submitting || !newFieldName.trim()" @click="addField"><DemoIcon name="plus" :size="13" />添加字段</button>
          </div>
          <div class="field-list">
            <div v-for="field in selectedAttribute.fields.slice().sort((a, b) => a.sortOrder - b.sortOrder)" :key="field.id" class="field-row" :class="{ inactive: !field.enabled }">
              <span class="drag-handle"><DemoIcon name="grip-vertical" :size="14" /></span>
              <template v-if="editingFieldId === field.id">
                <input v-model="editingFieldName" class="inp field-edit-input" @keyup.enter="saveField(field.id)" />
                <button class="icon-btn" type="button" title="保存" @click="saveField(field.id)"><DemoIcon name="check" :size="14" /></button>
              </template>
              <template v-else>
                <span class="field-name">{{ field.name }}</span>
                <button class="icon-btn field-action" type="button" title="重命名" @click="startEditField(field.id, field.name)"><DemoIcon name="pencil" :size="13" /></button>
                <button class="field-action field-toggle" type="button" @click="toggleField(field.id)">{{ field.enabled ? '停用' : '启用' }}</button>
              </template>
            </div>
            <div v-if="!selectedAttribute.fields.length" class="empty-fields"><DemoIcon name="list-plus" :size="24" /><span>还没有字段选项</span><small>先添加“耳环、法兰”这样的可选值</small></div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.attribute-admin-page { display: flex; flex-direction: column; gap: 16px; min-height: 100%; padding: 22px 26px; color: var(--text-1); }
.attribute-page-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; }
.attribute-page-head h1 { margin: 5px 0 4px; font-family: var(--font-display); font-size: 22px; font-weight: 900; }
.attribute-page-head p { margin: 0; color: var(--text-3); font-size: 12px; }
.eyebrow, .editor-kicker { display: inline-flex; align-items: center; gap: 6px; color: var(--accent); font-size: 11px; font-family: 'JetBrains Mono', monospace; letter-spacing: 1px; }
.attribute-workspace { display: grid; grid-template-columns: minmax(260px, 34%) minmax(0, 1fr); gap: 16px; flex: 1; min-height: 520px; }
.attribute-list-panel, .attribute-editor-panel { padding: 16px; min-width: 0; }
.panel-title-row, .editor-head, .fields-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.panel-title-row { margin-bottom: 14px; }
.panel-title-row h2, .editor-head h2, .fields-head h3 { margin: 0; font-size: 15px; font-weight: 800; }
.panel-title-row span, .fields-head span { color: var(--text-3); font-size: 11px; }
.compact-search { display: flex; align-items: center; gap: 6px; width: 145px; padding: 5px 8px; border: 1px solid var(--line); border-radius: 7px; color: var(--text-3); }
.compact-search input { width: 100%; border: 0; outline: 0; background: transparent; color: var(--text-1); font-size: 11px; }
.attribute-list { display: flex; flex-direction: column; gap: 5px; }
.attribute-list-item { display: flex; align-items: center; gap: 9px; width: 100%; padding: 10px; border: 1px solid transparent; border-radius: 9px; background: transparent; color: var(--text-1); text-align: left; cursor: pointer; transition: background .18s, border-color .18s; }
.attribute-list-item:hover { background: var(--hover); }
.attribute-list-item.active { border-color: var(--accent); background: var(--active); }
.attribute-list-item.disabled { opacity: .55; }
.attribute-list-icon { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 7px; background: var(--accent-soft); color: var(--accent); }
.attribute-list-main { display: flex; flex-direction: column; gap: 3px; flex: 1; min-width: 0; }
.attribute-list-main strong { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 12.5px; }
.attribute-list-main small { color: var(--text-3); font-size: 10.5px; }
.attribute-state { padding: 2px 6px; border-radius: 10px; font-size: 10px; }
.attribute-state.on { color: var(--ok); background: var(--accent-soft); }
.attribute-state.off { color: var(--text-3); background: var(--panel-2); }
.empty-list { padding: 35px 10px; color: var(--text-3); text-align: center; font-size: 12px; }
.editor-head { padding-bottom: 14px; border-bottom: 1px solid var(--line); }
.editor-head h2 { margin-top: 4px; font-size: 17px; }
.editor-form { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding: 18px 0; }
.form-field { display: flex; flex-direction: column; gap: 6px; }
.form-field.wide { grid-column: 1 / -1; }
.form-field label { color: var(--text-2); font-size: 12px; font-weight: 600; }
.switch-card { display: flex; align-items: center; gap: 9px; min-height: 45px; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; cursor: pointer; }
.switch-card input { accent-color: var(--accent); }
.switch-card span { display: flex; flex-direction: column; gap: 3px; }
.switch-card strong { font-size: 12px; }
.switch-card small { color: var(--text-3); font-size: 10.5px; }
.hint-card { color: var(--accent); }
.hint-card small { color: var(--text-3); }
.save-attribute { grid-column: 1 / -1; justify-self: end; }
.fields-editor { padding-top: 16px; border-top: 1px solid var(--line); }
.fields-head { margin-bottom: 12px; }
.fields-head h3 { font-size: 14px; }
.fields-head span { display: block; margin-top: 3px; }
.field-count { padding: 3px 8px; border-radius: 10px; background: var(--accent-soft); color: var(--accent) !important; }
.field-add-row { display: flex; gap: 8px; margin-bottom: 10px; }
.field-add-row .inp { flex: 1; }
.field-list { display: flex; flex-direction: column; gap: 5px; }
.field-row { display: flex; align-items: center; gap: 8px; min-height: 38px; padding: 5px 8px; border: 1px solid var(--line); border-radius: 7px; background: var(--panel-2); }
.field-row.inactive { opacity: .55; }
.drag-handle { color: var(--text-3); }
.field-name { flex: 1; font-size: 12.5px; }
.field-edit-input { flex: 1; }
.field-action { color: var(--text-3); cursor: pointer; }
.field-action:hover { color: var(--accent); }
.field-toggle { padding: 3px 6px; border: 0; border-radius: 5px; background: transparent; font-size: 10.5px; }
.empty-fields { display: flex; flex-direction: column; align-items: center; gap: 5px; padding: 35px; color: var(--text-3); }
.empty-fields span { font-size: 12px; }
.empty-fields small { font-size: 10.5px; }
@media (max-width: 900px) { .attribute-workspace { grid-template-columns: 1fr; } .attribute-list-panel { min-height: 240px; } }
@media (max-width: 560px) { .attribute-admin-page { padding: 16px; } .attribute-page-head { flex-direction: column; } .editor-form { grid-template-columns: 1fr; } .form-field.wide, .save-attribute { grid-column: auto; } .compact-search { width: 120px; } }
</style>
