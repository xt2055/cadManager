<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import DrawingAttributesForm from '@/components/common/DrawingAttributesForm.vue'
import { STATUS, useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'
import type { Drawing, StructurePart, StructurePartEditable } from '@/types/domain.types'

defineOptions({ name: 'DrawingPropertiesTab' })

const domainStore = useDomainStore()
const router = useRouter()
const uiStore = useUiStore()

const currentItem = computed(() => domainStore.currentDrawing)
const currentDrawing = computed<Drawing | null>(() => {
  const item = currentItem.value
  return item && !('parentNo' in item) ? item : null
})
const currentPart = computed<StructurePart | null>(() => {
  if (!currentItem.value || !('parentNo' in currentItem.value)) return null
  return currentItem.value
})
const isPart = computed(() => Boolean(currentPart.value))
const parentDrawing = computed(() => {
  const parentNo = currentPart.value?.parentNo
  return parentNo ? domainStore.drawings.find((drawing) => drawing.no === parentNo) ?? domainStore.structure.find((part) => part.no === parentNo) ?? null : null
})

const editing = ref(false)
const saving = ref(false)
const attributeEditing = ref(false)
const attributeSaving = ref(false)
const attributeForm = ref<Record<string, string>>({})

function emptyEditForm(): StructurePartEditable {
  return {
    name: '',
    material: '',
    spec: '',
    weight: 0,
    surfaceTreatment: '',
    partType: '自制件',
    qty: 1,
    vendor: '',
    remark: '',
  }
}

const partNoEdit = ref('')

const editForm = ref<StructurePartEditable>(emptyEditForm())
const activityLogs = computed(() => {
  const drawingNo = currentItem.value?.no
  return drawingNo ? domainStore.logs.filter((item) => item.drawingNo === drawingNo) : []
})

function loadEditForm(part: StructurePart) {
  partNoEdit.value = part.no
  editForm.value = {
    name: part.name,
    material: part.material,
    spec: part.spec,
    weight: part.weight,
    surfaceTreatment: part.surfaceTreatment,
    partType: part.partType,
    qty: part.qty,
    vendor: part.vendor ?? '',
    remark: part.remark ?? '',
  }
}

watch(currentPart, (part) => {
  editing.value = false
  if (part) loadEditForm(part)
}, { immediate: true })

watch(currentItem, (item) => {
  attributeForm.value = { ...(item && !('parentNo' in item) ? item.attributeValues ?? {} : {}) }
  attributeEditing.value = false
}, { immediate: true })

function startEdit() {
  if (!currentPart.value) return
  loadEditForm(currentPart.value)
  editing.value = true
}

function cancelEdit() {
  if (currentPart.value) loadEditForm(currentPart.value)
  editing.value = false
}

async function saveEdit() {
  const part = currentPart.value
  if (!part) return
  saving.value = true
  try {
    const nextPartNo = partNoEdit.value.trim() || part.no
    await domainStore.updateStructurePart(part.no, { ...editForm.value, partNo: nextPartNo })
    editing.value = false
    if (nextPartNo !== part.no) {
      await router.replace({ name: 'drawing-properties', params: { drawingId: nextPartNo } })
    }
    uiStore.toast(`零件图「${nextPartNo}」属性已保存`, 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '零件属性保存失败，请稍后重试', 'warn')
  } finally {
    saving.value = false
  }
}

function startAttributeEdit() {
  if (!currentDrawing.value) return
  attributeForm.value = { ...(currentDrawing.value.attributeValues ?? {}) }
  attributeEditing.value = true
}

function cancelAttributeEdit() {
  if (currentDrawing.value) {
    attributeForm.value = { ...(currentDrawing.value.attributeValues ?? {}) }
  }
  attributeEditing.value = false
}

async function saveAttributes() {
  if (!currentDrawing.value || attributeSaving.value) return
  attributeSaving.value = true
  try {
    await domainStore.setDrawingAttributes(currentDrawing.value.no, attributeForm.value)
    attributeEditing.value = false
    uiStore.toast('图纸业务属性已更新', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '图纸属性保存失败，请稍后重试', 'warn')
  } finally {
    attributeSaving.value = false
  }
}

function openParentDrawing() {
  const parent = parentDrawing.value
  if (!parent) return
  domainStore.openDrawing(parent.no)
  router.push({ name: 'drawing-preview', params: { drawingId: parent.no } })
}

const feedIcons: Record<string, string> = {
  view: 'eye',
  edit: 'pencil',
  branch: 'git-branch',
  borrow: 'share-2',
  check: 'check-circle-2',
  back: 'undo-2',
}
</script>

<template>
  <div class="properties-page-view">
    <!-- 总图专属：业务技术规格（动态属性）卡片 -->
    <div v-if="!isPart && currentDrawing && domainStore.sortedAttributes.length" class="props-section card card-pad">
      <div class="card-title no-padding property-title">
        <div class="title-with-icon">
          <DemoIcon name="sliders-horizontal" :size="16" />
          <span>业务技术规格属性</span>
          <span class="spec-count-tag">{{ domainStore.sortedAttributes.length }} 项规格</span>
        </div>

        <div v-if="!attributeEditing" class="property-actions">
          <button class="btn sm primary" type="button" @click="startAttributeEdit">
            <DemoIcon name="pencil" :size="13" />修改规格属性
          </button>
        </div>
        <div v-else class="property-actions">
          <button class="btn sm" type="button" :disabled="attributeSaving" @click="cancelAttributeEdit">取消</button>
          <button class="btn sm primary" type="button" :disabled="attributeSaving" @click="saveAttributes">
            <DemoIcon :name="attributeSaving ? 'loader' : 'check'" :size="13" />{{ attributeSaving ? '保存中…' : '保存修改' }}
          </button>
        </div>
      </div>

      <div class="attributes-tab-body">
        <DrawingAttributesForm
          v-model="attributeForm"
          :attributes="domainStore.sortedAttributes"
          :readonly="!attributeEditing"
          title="业务技术参数"
          description="按企业标准化属性库定义；修改后立即生效"
        />
      </div>
    </div>

    <!-- 基础核心元数据卡片 -->
    <div class="props-section card card-pad">
      <div class="card-title no-padding property-title">
        <div class="title-with-icon">
          <DemoIcon name="info" :size="16" />
          <span>{{ isPart ? '零件图核心元数据属性' : '项目总图工程信息' }}</span>
        </div>

        <button v-if="isPart && !editing" class="btn sm primary property-edit-button" type="button" @click="startEdit">
          <DemoIcon name="pencil" :size="13" />编辑零件属性
        </button>
        <div v-else-if="isPart" class="property-actions">
          <button class="btn sm" type="button" :disabled="saving" @click="cancelEdit">取消</button>
          <button class="btn sm primary" type="button" :disabled="saving" @click="saveEdit">
            <DemoIcon :name="saving ? 'loader' : 'check'" :size="13" />{{ saving ? '保存中…' : '保存修改' }}
          </button>
        </div>
      </div>

      <!-- 零件编辑态 -->
      <template v-if="isPart && currentPart">
        <div v-if="editing" class="edit-form-grid">
          <div class="form-field">
            <label for="part-no">零件图号</label>
            <input id="part-no" v-model="partNoEdit" class="inp mono" type="text" />
          </div>
          <div class="form-field">
            <label for="part-name">零件名称 *</label>
            <input id="part-name" v-model="editForm.name" class="inp" type="text" />
          </div>
          <div class="form-field">
            <label for="part-material">材料牌号 *</label>
            <input id="part-material" v-model="editForm.material" class="inp" list="part-material-options" type="text" placeholder="如 45#钢、HT200" />
            <datalist id="part-material-options">
              <option value="45#钢" />
              <option value="40Cr" />
              <option value="HT200" />
              <option value="Q235B" />
              <option value="6061-T6" />
            </datalist>
          </div>
          <div class="form-field readonly-field">
            <label>所属总图</label>
            <button class="related-drawing-link" type="button" @click="openParentDrawing">
              <span class="mono">{{ currentPart.parentNo }}</span><DemoIcon name="arrow-up-right" :size="13" />
            </button>
          </div>
          <div class="form-field readonly-field">
            <label>所属项目</label>
            <div class="readonly-value">{{ currentPart.project || parentDrawing?.project || '—' }}</div>
          </div>
          <div class="form-field">
            <label for="part-type">制造类别</label>
            <select id="part-type" v-model="editForm.partType" class="inp">
              <option value="自制件">自制件</option>
              <option value="外协件">外协件</option>
              <option value="标准件">标准件</option>
              <option value="外购件">外购件</option>
            </select>
          </div>
          <div class="form-field">
            <label for="part-qty">单台装配数量 *</label>
            <input id="part-qty" v-model.number="editForm.qty" class="inp" type="number" min="0.01" step="1" />
          </div>
          <div class="form-field">
            <label for="part-vendor">承制厂商 / 外协单位</label>
            <input id="part-vendor" v-model="editForm.vendor" class="inp" type="text" placeholder="自制件可留空" />
          </div>
          <div class="form-field full-width-field">
            <label for="part-remark">工程说明与备注</label>
            <textarea id="part-remark" v-model="editForm.remark" class="inp" rows="3" placeholder="填写技术交底、替代料或装配注意事项" />
          </div>
        </div>

        <div v-else class="kv-grid">
          <div class="kv"><div class="k">零件图号</div><div class="v mono">{{ currentPart.no }}</div></div>
          <div class="kv"><div class="k">零件名称</div><div class="v">{{ currentPart.name }}</div></div>
          <div class="kv"><div class="k">对象分类</div><div class="v"><span class="tag plain">零件图</span></div></div>
          <div class="kv"><div class="k">材料牌号</div><div class="v">{{ currentPart.material || '—' }}</div></div>
          <div class="kv"><div class="k">所属总图</div><div class="v"><button class="related-drawing-link" type="button" @click="openParentDrawing">{{ parentDrawing?.name || currentPart.parentNo }}<DemoIcon name="arrow-up-right" :size="13" /></button></div></div>
          <div class="kv"><div class="k">所属项目</div><div class="v">{{ currentPart.project || parentDrawing?.project || '—' }}</div></div>
          <div class="kv"><div class="k">制造类别</div><div class="v"><span class="tag info">{{ currentPart.partType }}</span></div></div>
          <div class="kv"><div class="k">单台装配数量</div><div class="v mono">× {{ currentPart.qty }}</div></div>
          <div class="kv"><div class="k">承制厂商 / 外协单位</div><div class="v">{{ currentPart.vendor || '—' }}</div></div>
          <div class="kv"><div class="k">当前发布版本</div><div class="v mono">{{ currentPart.ver }}</div></div>
          <div class="kv"><div class="k">生命周期状态</div><div class="v"><span class="tag" :class="STATUS[currentPart.status].c">{{ STATUS[currentPart.status].t }}</span></div></div>
          <div class="kv full-width"><div class="k">工程说明与备注</div><div class="v remark-txt">{{ currentPart.remark || '无特殊备忘与交底要求' }}</div></div>
        </div>
      </template>

      <!-- 总图信息只读呈现 -->
      <div v-else class="kv-grid">
        <div class="kv"><div class="k">总图图号</div><div class="v mono accent-txt">{{ currentItem?.no }}</div></div>
        <div class="kv"><div class="k">图纸名称</div><div class="v">{{ currentItem?.name }}</div></div>
        <div class="kv"><div class="k">对象类型</div><div class="v"><span class="tag plain">项目总图</span></div></div>
        <div class="kv"><div class="k">责任单位</div><div class="v">{{ (currentItem as Drawing)?.vendor || '内部项目部' }}</div></div>
        <div class="kv"><div class="k">所属项目</div><div class="v">{{ (currentItem as Drawing)?.project || '—' }}</div></div>
        <div class="kv"><div class="k">发布版本</div><div class="v mono">{{ currentItem?.ver || 'v1.0' }}</div></div>
        <div class="kv"><div class="k">生命周期状态</div><div class="v"><span v-if="currentItem" class="tag" :class="STATUS[currentItem.status].c">{{ STATUS[currentItem.status].t }}</span></div></div>
        <div class="kv"><div class="k">更新时间</div><div class="v mono">{{ (currentItem && 'updated' in currentItem ? currentItem.updated : '') || '—' }}</div></div>
        <div class="kv full-width"><div class="k">工程说明与备注</div><div class="v remark-txt">{{ currentItem?.remark || '无特殊备忘与交底要求' }}</div></div>
      </div>
    </div>

    <!-- 操作与审计追踪记录 -->
    <div class="props-section card card-pad">
      <div class="card-title no-padding">
        <div class="title-with-icon">
          <DemoIcon name="activity" :size="16" />
          <span>全生命周期审计与操作追踪</span>
          <span class="hint">修改 · 分支 · 属性 · 文件 · 审核</span>
        </div>
      </div>

      <div class="feed">
        <div v-for="item in activityLogs" :key="item.id" class="feed-item">
          <div class="feed-ic" :class="item.act">
            <DemoIcon :name="feedIcons[item.act] ?? 'activity'" :size="14" />
          </div>
          <div class="feed-txt">
            <b>{{ item.user }}</b> <span v-html="item.txt"></span>
          </div>
          <div class="feed-time">{{ item.time }}</div>
        </div>
        <div v-if="!activityLogs.length" class="empty compact-empty">
          <DemoIcon name="activity" :size="30" />
          <div class="t">暂无审计操作记录</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.properties-page-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.no-padding {
  padding: 0 0 14px 0;
  border-bottom: 1px solid var(--line);
  margin-bottom: 14px;
}

.property-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.title-with-icon {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 700;
  color: var(--text-1);
}

.spec-count-tag {
  padding: 2px 7px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 11px;
  font-weight: 600;
}

.property-edit-button {
  margin-left: auto;
}

.property-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.attributes-tab-body {
  padding: 4px 0;
}

.edit-form-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 14px 18px;
}

.form-field {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 5px;
}

.form-field label {
  color: var(--text-3);
  font-size: 11.5px;
}

.readonly-value {
  display: flex;
  align-items: center;
  min-height: 36px;
  padding: 0 12px;
  border: 1px dashed var(--line-strong);
  border-radius: 9px;
  color: var(--text-2);
  background: var(--panel-2);
  font-size: 12.5px;
}

.related-drawing-link {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 5px;
  min-height: 36px;
  padding: 0;
  color: var(--accent);
  font-size: 12.5px;
  text-align: left;
}

.related-drawing-link:hover {
  text-decoration: underline;
}

.full-width-field {
  grid-column: span 3;
}

.form-field textarea.inp {
  height: auto;
  min-height: 76px;
  padding: 9px 12px;
  resize: vertical;
}

.kv-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 16px 20px;
}

.kv {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.kv.full-width {
  grid-column: span 3;
}

.kv .k {
  color: var(--text-3);
  font-size: 11.5px;
}

.kv .v {
  font-size: 13px;
  font-weight: 500;
}

.accent-txt {
  color: var(--accent);
  font-weight: 700;
}

.remark-txt {
  color: var(--text-2);
  line-height: 1.6;
}

.hint {
  margin-left: 8px;
  color: var(--text-3);
  font-size: 11px;
  font-weight: 400;
}

.feed {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.feed-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 10px;
  border-radius: 8px;
  background: var(--panel-2);
}

.feed-ic {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: var(--panel);
  color: var(--accent);
}

.feed-txt {
  flex: 1;
  font-size: 12px;
}

.feed-time {
  color: var(--text-3);
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
}

@media (max-width: 1024px) {
  .kv-grid {
    grid-template-columns: 1fr 1fr;
  }
  .kv.full-width {
    grid-column: span 2;
  }
  .edit-form-grid {
    grid-template-columns: 1fr 1fr;
  }
  .full-width-field {
    grid-column: span 2;
  }
}

@media (max-width: 640px) {
  .kv-grid {
    grid-template-columns: 1fr;
  }
  .kv.full-width {
    grid-column: span 1;
  }
  .edit-form-grid {
    grid-template-columns: 1fr;
  }
  .full-width-field {
    grid-column: span 1;
  }
  .property-title {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .property-edit-button,
  .property-actions {
    width: 100%;
    margin-left: 0;
  }
}
</style>
