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
const selectedTab = ref<'all' | 'enabled' | 'required'>('all')
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
  let list = attributes.value

  if (selectedTab.value === 'enabled') {
    list = list.filter((item) => item.enabled)
  } else if (selectedTab.value === 'required') {
    list = list.filter((item) => item.required)
  }

  if (!value) return list
  return list.filter((attribute) =>
    attribute.name.toLowerCase().includes(value) ||
    attribute.fields.some((field) => field.name.toLowerCase().includes(value)),
  )
})

const selectedAttribute = computed(() => attributes.value.find((attribute) => attribute.id === selectedId.value) ?? null)

function selectAttribute(attribute: DrawingAttribute) {
  selectedId.value = attribute.id
  attributeName.value = attribute.name
  attributeRequired.value = attribute.required
  attributeEnabled.value = attribute.enabled
  editingAttribute.value = true
  editingFieldId.value = ''
  newFieldName.value = ''
}

function startNewAttribute() {
  selectedId.value = ''
  attributeName.value = ''
  attributeRequired.value = false
  attributeEnabled.value = true
  editingAttribute.value = false
  editingFieldId.value = ''
  newFieldName.value = ''
}

async function saveAttribute() {
  const name = attributeName.value.trim()
  if (!name || submitting.value) return
  submitting.value = true
  try {
    if (editingAttribute.value && selectedId.value) {
      await domainStore.updateAttribute(selectedId.value, {
        name,
        required: attributeRequired.value,
        enabled: attributeEnabled.value,
      })
      uiStore.toast('属性配置已保存', 'ok')
    } else {
      const created = await domainStore.addAttribute(name, attributeRequired.value)
      selectedId.value = created.id
      editingAttribute.value = true
      uiStore.toast(`属性「${name}」已创建，请继续添加字段选项`, 'ok')
    }
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '保存属性失败', 'warn')
  } finally {
    submitting.value = false
  }
}

// 支持逗号/顿号/换行批量添加字段
async function addFieldsFromInput() {
  if (!selectedAttribute.value || !newFieldName.value.trim() || submitting.value) return
  const raw = newFieldName.value
  const tokens = raw.split(/[\n,，、;；\s]+/).map((t) => t.trim()).filter(Boolean)
  if (!tokens.length) return

  submitting.value = true
  let successCount = 0
  try {
    for (const token of tokens) {
      try {
        await domainStore.addAttributeField(selectedAttribute.value.id, token)
        successCount++
      } catch {
        // 跳过重复项
      }
    }
    newFieldName.value = ''
    if (successCount > 0) {
      uiStore.toast(`已成功添加 ${successCount} 个字段选项`, 'ok')
    } else {
      uiStore.toast('输入的字段已全部存在或格式无效', 'warn')
    }
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
    uiStore.toast('字段选项已更新', 'ok')
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

async function moveField(index: number, direction: 'up' | 'down') {
  if (!selectedAttribute.value || submitting.value) return
  const fields = selectedAttribute.value.fields.slice().sort((a, b) => a.sortOrder - b.sortOrder)
  const targetIndex = direction === 'up' ? index - 1 : index + 1
  if (targetIndex < 0 || targetIndex >= fields.length) return

  const itemA = fields[index]
  const itemB = fields[targetIndex]
  if (!itemA || !itemB) return

  fields[index] = itemB
  fields[targetIndex] = itemA

  submitting.value = true
  try {
    await domainStore.reorderAttributeFields(selectedAttribute.value.id, fields.map((f) => f.id))
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '调序失败', 'warn')
  } finally {
    submitting.value = false
  }
}

function deleteAttribute() {
  if (!selectedAttribute.value) return
  uiStore.confirm(`删除属性「${selectedAttribute.value.name}」？`, '该属性及其字段选项将被永久移除，历史图纸中对应的属性值也会被清理。', {
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
      <div class="head-left">
        <div class="eyebrow"><DemoIcon name="sliders-horizontal" :size="13" /> 企业标准化配置</div>
        <h1>图纸业务属性与字段管理</h1>
        <p>属性定义业务维度（如安装方式、压力等级），字段定义可选值（如耳环、法兰）。平铺展示，创建图纸动态校验。</p>
      </div>
      <div class="head-actions">
        <button class="btn primary" type="button" @click="startNewAttribute">
          <DemoIcon name="plus" :size="14" />新建业务属性
        </button>
      </div>
    </header>

    <div class="attribute-workspace">
      <!-- 左栏：属性列表 -->
      <section class="attribute-list-panel card">
        <div class="list-panel-head">
          <div class="head-title-row">
            <h2>业务属性清单</h2>
            <span class="count-badge">{{ attributes.length }}</span>
          </div>
          <div class="filter-bar">
            <div class="tab-filters">
              <button
                class="tab-btn"
                :class="{ active: selectedTab === 'all' }"
                type="button"
                @click="selectedTab = 'all'"
              >
                全部
              </button>
              <button
                class="tab-btn"
                :class="{ active: selectedTab === 'enabled' }"
                type="button"
                @click="selectedTab = 'enabled'"
              >
                启用中
              </button>
              <button
                class="tab-btn"
                :class="{ active: selectedTab === 'required' }"
                type="button"
                @click="selectedTab = 'required'"
              >
                必填项
              </button>
            </div>
            <label class="compact-search">
              <DemoIcon name="search" :size="13" />
              <input v-model="query" placeholder="搜索名称或字段" />
            </label>
          </div>
        </div>

        <div class="attribute-list-scroll">
          <button
            v-for="attribute in filteredAttributes"
            :key="attribute.id"
            class="attribute-card-item"
            :class="{
              active: attribute.id === selectedId,
              disabled: !attribute.enabled,
            }"
            type="button"
            @click="selectAttribute(attribute)"
          >
            <div class="card-item-top">
              <div class="card-title-group">
                <span class="attr-icon"><DemoIcon name="sliders-horizontal" :size="13" /></span>
                <strong class="attr-title">{{ attribute.name }}</strong>
              </div>
              <div class="card-badges">
                <span v-if="attribute.required" class="badge-req">必填</span>
                <span v-else class="badge-opt">选填</span>
                <span class="state-pill" :class="attribute.enabled ? 'on' : 'off'">
                  {{ attribute.enabled ? '已启用' : '已停用' }}
                </span>
              </div>
            </div>

            <div class="card-item-bottom">
              <span class="field-meta">
                <DemoIcon name="tag" :size="11" />
                {{ attribute.fields.filter((f) => f.enabled).length }} 个可用字段
              </span>
              <span v-if="attribute.fields.length" class="field-preview-tags">
                {{ attribute.fields.slice(0, 3).map((f) => f.name).join('、') }}{{ attribute.fields.length > 3 ? '…' : '' }}
              </span>
            </div>
          </button>

          <div v-if="!filteredAttributes.length" class="empty-list">
            <DemoIcon name="search-x" :size="28" />
            <span>未找到匹配的属性</span>
          </div>
        </div>
      </section>

      <!-- 右栏：属性配置与字段管理 -->
      <div class="attribute-detail-column">
        <!-- 属性基本设置卡片 -->
        <section class="attribute-editor-panel card">
          <div class="editor-head">
            <div class="editor-head-title">
              <span class="editor-kicker">
                <DemoIcon name="pencil" :size="12" />
                {{ editingAttribute ? '属性参数配置' : '创建新属性' }}
              </span>
              <h2>{{ editingAttribute ? `配置属性「${selectedAttribute?.name || ''}」` : '新建图纸业务属性' }}</h2>
            </div>
            <button
              v-if="editingAttribute"
              class="btn sm danger delete-btn"
              type="button"
              @click="deleteAttribute"
            >
              <DemoIcon name="trash-2" :size="13" />删除此属性
            </button>
          </div>

          <div class="editor-form-grid">
            <div class="form-field wide">
              <label for="attribute-name-inp">
                <span>属性显示名称 *</span>
                <small>如：安装方式、气缸缸径、额定工压、油口规格</small>
              </label>
              <input
                id="attribute-name-inp"
                v-model="attributeName"
                class="inp"
                placeholder="请输入属性名称"
                @keyup.enter="saveAttribute"
              />
            </div>

            <label class="switch-toggle-card" :class="{ checked: attributeRequired }">
              <input v-model="attributeRequired" type="checkbox" />
              <div class="toggle-content">
                <div class="toggle-label">创建图纸时必填项</div>
                <div class="toggle-desc">勾选后，工程师未选该属性将无法创建或保存图纸</div>
              </div>
            </label>

            <label v-if="editingAttribute" class="switch-toggle-card" :class="{ checked: attributeEnabled }">
              <input v-model="attributeEnabled" type="checkbox" />
              <div class="toggle-content">
                <div class="toggle-label">启用此属性</div>
                <div class="toggle-desc">关闭后，新建与分叉图纸表单将不再展示此属性</div>
              </div>
            </label>

            <div v-else class="switch-toggle-card info-card">
              <div class="info-icon"><DemoIcon name="info" :size="16" /></div>
              <div class="toggle-content">
                <div class="toggle-label">创建后即可配置字段选项</div>
                <div class="toggle-desc">点击保存后，在下方添加“耳环、法兰”等可选值</div>
              </div>
            </div>

            <div class="form-action-row wide">
              <button
                class="btn primary save-btn"
                type="button"
                :disabled="submitting || !attributeName.trim()"
                @click="saveAttribute"
              >
                <DemoIcon name="save" :size="14" />
                {{ editingAttribute ? '保存属性设置' : '创建并开始添加字段' }}
              </button>
            </div>
          </div>

          <!-- 字段选项维护（仅在已选属性时显示） -->
          <div v-if="editingAttribute && selectedAttribute" class="fields-section">
            <div class="fields-head">
              <div class="fields-head-left">
                <h3>字段选项清单（供工程师下拉单选）</h3>
                <p>支持逗号、顿号或回车批量粘贴录入；可通过上下箭头调整呈现顺序</p>
              </div>
              <span class="active-field-count">
                {{ selectedAttribute.fields.filter((f) => f.enabled).length }} 个已启用 / 共 {{ selectedAttribute.fields.length }} 项
              </span>
            </div>

            <!-- 快捷批量录入栏 -->
            <div class="field-input-box">
              <input
                v-model="newFieldName"
                class="inp batch-input"
                placeholder="输入选项，支持批量粘贴（如：单耳环, 双耳环, 尾部法兰, 中部铰轴）"
                @keyup.enter="addFieldsFromInput"
              />
              <button
                class="btn primary add-field-btn"
                type="button"
                :disabled="submitting || !newFieldName.trim()"
                @click="addFieldsFromInput"
              >
                <DemoIcon name="plus" :size="13" />添加选项
              </button>
            </div>

            <!-- 字段列表 -->
            <div class="fields-container">
              <div
                v-for="(field, index) in selectedAttribute.fields.slice().sort((a, b) => a.sortOrder - b.sortOrder)"
                :key="field.id"
                class="field-row-card"
                :class="{ disabled: !field.enabled }"
              >
                <div class="field-drag-num mono">#{{ index + 1 }}</div>

                <template v-if="editingFieldId === field.id">
                  <input
                    v-model="editingFieldName"
                    class="inp inline-edit-inp"
                    autofocus
                    @keyup.enter="saveField(field.id)"
                    @keyup.esc="editingFieldId = ''"
                  />
                  <button class="icon-btn ok-btn" type="button" title="保存" @click="saveField(field.id)">
                    <DemoIcon name="check" :size="14" />
                  </button>
                  <button class="icon-btn" type="button" title="取消" @click="editingFieldId = ''">
                    <DemoIcon name="x" :size="14" />
                  </button>
                </template>

                <template v-else>
                  <span class="field-title" @dblclick="startEditField(field.id, field.name)">
                    {{ field.name }}
                  </span>
                  <div class="field-actions">
                    <!-- 调序操作 -->
                    <button
                      class="icon-btn sm"
                      type="button"
                      title="上移"
                      :disabled="index === 0"
                      @click="moveField(index, 'up')"
                    >
                      <DemoIcon name="arrow-up" :size="12" />
                    </button>
                    <button
                      class="icon-btn sm"
                      type="button"
                      title="下移"
                      :disabled="index === selectedAttribute.fields.length - 1"
                      @click="moveField(index, 'down')"
                    >
                      <DemoIcon name="arrow-down" :size="12" />
                    </button>
                    <!-- 编辑操作 -->
                    <button
                      class="icon-btn sm"
                      type="button"
                      title="修改名称"
                      @click="startEditField(field.id, field.name)"
                    >
                      <DemoIcon name="pencil" :size="12" />
                    </button>
                    <!-- 启停用切换 -->
                    <button
                      class="btn-text-status"
                      :class="field.enabled ? 'btn-warn' : 'btn-ok'"
                      type="button"
                      @click="toggleField(field.id)"
                    >
                      {{ field.enabled ? '停用' : '启用' }}
                    </button>
                  </div>
                </template>
              </div>

              <div v-if="!selectedAttribute.fields.length" class="empty-fields-tip">
                <DemoIcon name="list-plus" :size="24" />
                <strong>暂无字段选项</strong>
                <span>请在上方输入框添加如「耳环」「法兰」「底脚」等预设值</span>
              </div>
            </div>
          </div>
        </section>

      </div>
    </div>
  </div>
</template>

<style scoped>
.attribute-admin-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 100%;
  padding: 22px 26px;
  color: var(--text-1);
}

.attribute-page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
}

.head-left h1 {
  margin: 5px 0 4px;
  font-family: var(--font-display);
  font-size: 22px;
  font-weight: 900;
}

.head-left p {
  margin: 0;
  color: var(--text-3);
  font-size: 12px;
}

.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--accent);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  letter-spacing: 0.5px;
}

/* ================= 布局栅格 ================= */
.attribute-workspace {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 16px;
  flex: 1;
  align-items: start;
}

/* ================= 左栏列表 ================= */
.attribute-list-panel {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.list-panel-head {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.head-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.head-title-row h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 800;
}

.count-badge {
  padding: 2px 7px;
  border-radius: 10px;
  background: var(--panel-2);
  color: var(--text-2);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
}

.filter-bar {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tab-filters {
  display: flex;
  background: var(--panel-2);
  padding: 2px;
  border-radius: 7px;
  border: 1px solid var(--line);
}

.tab-btn {
  flex: 1;
  padding: 4px 6px;
  border: 0;
  background: transparent;
  color: var(--text-3);
  font-size: 11px;
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s;
}

.tab-btn.active {
  background: var(--panel);
  color: var(--text-1);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.08);
}

.compact-search {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--panel-2);
  color: var(--text-3);
}

.compact-search input {
  width: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-1);
  font-size: 11.5px;
}

.attribute-list-scroll {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 680px;
  overflow-y: auto;
}

.attribute-card-item {
  display: flex;
  flex-direction: column;
  gap: 7px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-2);
  color: var(--text-1);
  text-align: left;
  cursor: pointer;
  transition: all 0.18s;
}

.attribute-card-item:hover {
  background: var(--hover);
  border-color: color-mix(in srgb, var(--accent) 30%, var(--line));
}

.attribute-card-item.active {
  border-color: var(--accent);
  background: var(--active);
  box-shadow: 0 0 0 1px var(--accent);
}

.attribute-card-item.disabled {
  opacity: 0.55;
}

.card-item-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.card-title-group {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
}

.attr-icon {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 5px;
  background: var(--accent-soft);
  color: var(--accent);
}

.attr-title {
  font-size: 13px;
  font-weight: 700;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.card-badges {
  display: flex;
  align-items: center;
  gap: 4px;
}

.badge-req {
  padding: 1px 5px;
  border-radius: 4px;
  background: color-mix(in srgb, var(--warn) 15%, transparent);
  color: var(--warn);
  font-size: 10px;
  font-weight: 600;
}

.badge-opt {
  color: var(--text-3);
  font-size: 10px;
}

.state-pill {
  padding: 1px 5px;
  border-radius: 4px;
  font-size: 10px;
}

.state-pill.on {
  background: color-mix(in srgb, var(--ok) 15%, transparent);
  color: var(--ok);
}

.state-pill.off {
  background: var(--line);
  color: var(--text-3);
}

.card-item-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
  padding-top: 4px;
  border-top: 1px dashed color-mix(in srgb, var(--line) 60%, transparent);
}

.field-meta {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.field-preview-tags {
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 10.5px;
}

.empty-list {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 40px 10px;
  color: var(--text-3);
  font-size: 12px;
}

/* ================= 中右栏配置 ================= */
.attribute-detail-column {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}

.attribute-editor-panel {
  padding: 18px 20px;
}

.editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--line);
}

.editor-kicker {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  color: var(--accent);
  font-size: 11px;
  font-weight: 600;
}

.editor-head-title h2 {
  margin: 3px 0 0;
  font-size: 17px;
  font-weight: 800;
}

.editor-form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  padding-top: 16px;
}

.form-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-field.wide {
  grid-column: 1 / -1;
}

.form-field label {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}

.form-field label small {
  font-weight: 400;
  color: var(--text-3);
  font-size: 11px;
}

.switch-toggle-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel-2);
  cursor: pointer;
  transition: all 0.18s;
}

.switch-toggle-card:hover {
  background: var(--hover);
}

.switch-toggle-card.checked {
  border-color: var(--accent);
  background: var(--active);
}

.switch-toggle-card input {
  margin-top: 2px;
  accent-color: var(--accent);
}

.toggle-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.toggle-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-1);
}

.toggle-desc {
  font-size: 11px;
  color: var(--text-3);
  line-height: 1.35;
}

.info-card {
  cursor: default;
  background: var(--panel-2);
}

.info-icon {
  color: var(--accent);
  margin-top: 2px;
}

.form-action-row {
  display: flex;
  justify-content: flex-end;
  padding-top: 4px;
}

/* ================= 字段管理清单 ================= */
.fields-section {
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid var(--line);
}

.fields-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.fields-head-left h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 700;
}

.fields-head-left p {
  margin: 3px 0 0;
  font-size: 11.5px;
  color: var(--text-3);
}

.active-field-count {
  padding: 3px 8px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.field-input-box {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.batch-input {
  flex: 1;
}

.fields-container {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 280px;
  overflow-y: auto;
}

.field-row-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7px 12px;
  border: 1px solid var(--line);
  border-radius: 7px;
  background: var(--panel-2);
  transition: all 0.15s;
}

.field-row-card:hover {
  background: var(--hover);
  border-color: color-mix(in srgb, var(--accent) 30%, var(--line));
}

.field-row-card.disabled {
  opacity: 0.55;
}

.field-drag-num {
  color: var(--text-3);
  font-size: 11px;
  width: 26px;
}

.field-title {
  flex: 1;
  font-size: 13px;
  font-weight: 500;
  user-select: none;
}

.inline-edit-inp {
  flex: 1;
  height: 28px;
  padding: 0 8px;
  font-size: 12.5px;
}

.field-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-text-status {
  padding: 3px 7px;
  border: 0;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  background: transparent;
  transition: background 0.15s;
}

.btn-warn {
  color: var(--warn);
}

.btn-warn:hover {
  background: color-mix(in srgb, var(--warn) 12%, transparent);
}

.btn-ok {
  color: var(--ok);
}

.btn-ok:hover {
  background: color-mix(in srgb, var(--ok) 12%, transparent);
}

.empty-fields-tip {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
  padding: 32px;
  border: 1px dashed var(--line);
  border-radius: 8px;
  color: var(--text-3);
}

.empty-fields-tip strong {
  font-size: 13px;
  color: var(--text-2);
}

.empty-fields-tip span {
  font-size: 11.5px;
}

@media (max-width: 960px) {
  .attribute-workspace {
    grid-template-columns: 1fr;
  }
  .attribute-list-panel {
    max-height: 300px;
  }
}

@media (max-width: 600px) {
  .attribute-admin-page {
    padding: 14px;
  }
  .attribute-page-head {
    flex-direction: column;
  }
  .editor-form-grid {
    grid-template-columns: 1fr;
  }
  .form-action-row {
    justify-content: stretch;
  }
  .save-btn {
    width: 100%;
  }
}
</style>
