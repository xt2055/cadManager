<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import { useUiStore } from '@/stores/ui.store'

defineOptions({ name: 'CategoryManagementPage' })

const domainStore = useDomainStore()
const uiStore = useUiStore()

const newCategoryName = ref('')
const newCategoryParent = ref('')
const creating = ref(false)
const renamingId = ref('')
const renamingValue = ref('')
const saving = ref(false)
const moveSelection = ref<Record<string, string>>({})

const tree = computed(() => domainStore.categoryTree)

const orphanCount = computed(() => domainStore.drawings.filter((drawing) => !drawing.categoryId).length)

function drawingsOf(categoryId: string) {
  return domainStore.drawings.filter((drawing) => drawing.categoryId === categoryId)
}

function drawingCount(categoryId: string): number {
  return drawingsOf(categoryId).length
}

async function handleCreate() {
  const name = newCategoryName.value.trim()
  if (!name) {
    uiStore.toast('请输入分类名称', 'warn')
    return
  }
  if (creating.value) return
  creating.value = true
  try {
    const parent = newCategoryParent.value || undefined
    await domainStore.addCategory(name, parent)
    newCategoryName.value = ''
    uiStore.toast(`分类「${name}」已创建`, 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '创建分类失败', 'warn')
  } finally {
    creating.value = false
  }
}

function startRename(id: string, currentName: string) {
  renamingId.value = id
  renamingValue.value = currentName
}

async function confirmRename() {
  const id = renamingId.value
  const name = renamingValue.value.trim()
  if (!id || !name) {
    renamingId.value = ''
    return
  }
  if (saving.value) return
  saving.value = true
  try {
    await domainStore.renameCategory(id, name)
    renamingId.value = ''
    uiStore.toast('分类已重命名', 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '重命名失败', 'warn')
  } finally {
    saving.value = false
  }
}

function handleDelete(id: string, name: string) {
  uiStore.confirm(`删除分类「${name}」`, '仅允许删除空分类：含子分类或含图纸的分类无法删除。', {
    danger: true,
    confirmText: '删除',
    onConfirm: async () => {
      try {
        await domainStore.deleteCategory(id)
        uiStore.toast(`分类「${name}」已删除`, 'ok')
      } catch (error) {
        uiStore.toast(error instanceof Error ? error.message : '删除失败', 'warn')
      }
    },
  })
}

async function handleMove(drawingNo: string) {
  const target = moveSelection.value[drawingNo]
  if (target === undefined) return
  try {
    await domainStore.setDrawingCategory(drawingNo, target)
    uiStore.toast(`图纸 ${drawingNo} 分类已更新`, 'ok')
  } catch (error) {
    uiStore.toast(error instanceof Error ? error.message : '移动图纸失败', 'warn')
  }
}

onMounted(() => domainStore.initialize().catch(() => undefined))
</script>

<template>
  <div class="category-manage">
    <header class="page-head">
      <div>
        <h1>图纸分类管理</h1>
        <p>两级分类（父分类 + 子分类）。图纸创建时选择分类，可在此移动归属。</p>
      </div>
      <span class="head-stat">未分类图纸 {{ orphanCount }} 张</span>
    </header>

    <section class="card create-card">
      <h2 class="section-title">新建分类</h2>
      <div class="create-row">
        <input v-model="newCategoryName" class="inp name-input" placeholder="分类名称" @keyup.enter="handleCreate" />
        <select v-model="newCategoryParent" class="inp parent-select">
          <option value="">作为父分类</option>
          <optgroup v-for="group in tree" :key="group.category.id" :label="group.category.name">
            <option :value="group.category.id">作为「{{ group.category.name }}」的子分类</option>
          </optgroup>
        </select>
        <button class="btn primary" type="button" :disabled="creating" @click="handleCreate">
          <DemoIcon name="plus" :size="14" />创建
        </button>
      </div>
    </section>

    <section class="card">
      <h2 class="section-title">分类树</h2>
      <div v-if="!tree.length" class="empty-tip">还没有分类，先在上方创建一个。</div>
      <div v-for="group in tree" :key="group.category.id" class="cat-group">
        <div class="cat-row parent">
          <DemoIcon name="folder" :size="15" />
          <template v-if="renamingId === group.category.id">
            <input v-model="renamingValue" class="inp rename-input" @keyup.enter="confirmRename" />
            <button class="btn sm primary" type="button" :disabled="saving" @click="confirmRename">保存</button>
          </template>
          <template v-else>
            <strong class="cat-name">{{ group.category.name }}</strong>
            <span class="cat-count">{{ drawingCount(group.category.id) }} 图纸</span>
            <button class="icon-btn" type="button" title="重命名" @click="startRename(group.category.id, group.category.name)"><DemoIcon name="pencil" :size="13" /></button>
            <button class="icon-btn" type="button" title="删除" @click="handleDelete(group.category.id, group.category.name)"><DemoIcon name="trash-2" :size="13" /></button>
          </template>
        </div>
        <div v-for="child in group.children" :key="child.id" class="cat-row child">
          <DemoIcon name="folder-tree" :size="14" />
          <template v-if="renamingId === child.id">
            <input v-model="renamingValue" class="inp rename-input" @keyup.enter="confirmRename" />
            <button class="btn sm primary" type="button" :disabled="saving" @click="confirmRename">保存</button>
          </template>
          <template v-else>
            <span class="cat-name">{{ child.name }}</span>
            <span class="cat-count">{{ drawingCount(child.id) }} 图纸</span>
            <button class="icon-btn" type="button" title="重命名" @click="startRename(child.id, child.name)"><DemoIcon name="pencil" :size="13" /></button>
            <button class="icon-btn" type="button" title="删除" @click="handleDelete(child.id, child.name)"><DemoIcon name="trash-2" :size="13" /></button>
          </template>
        </div>
        <div v-if="!group.children.length" class="cat-child-empty">（无子分类）</div>
      </div>
    </section>

    <section class="card">
      <h2 class="section-title">图纸归类（移动）</h2>
      <table v-if="domainStore.drawings.length" class="tbl move-table">
        <thead>
          <tr><th>项目图号</th><th>名称</th><th>当前分类</th><th>移动到</th></tr>
        </thead>
        <tbody>
          <tr v-for="drawing in domainStore.drawings" :key="drawing.no">
            <td class="mono">{{ drawing.no }}</td>
            <td>{{ drawing.name }}</td>
            <td><span class="tag plain">{{ domainStore.categoryName(drawing.categoryId) || '未分类' }}</span></td>
            <td>
              <div class="move-row">
                <select v-model="moveSelection[drawing.no]" class="inp move-select">
                  <option value="">未分类</option>
                  <optgroup v-for="group in tree" :key="group.category.id" :label="group.category.name">
                    <option :value="group.category.id">{{ group.category.name }}</option>
                    <option v-for="child in group.children" :key="child.id" :value="child.id">　{{ child.name }}</option>
                  </optgroup>
                </select>
                <button class="btn sm" type="button" :disabled="moveSelection[drawing.no] === undefined" @click="handleMove(drawing.no)">移动</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="empty-tip">暂无图纸</div>
    </section>
  </div>
</template>

<style scoped>
.category-manage { display: flex; flex-direction: column; gap: 16px; padding: 22px 26px; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; flex-wrap: wrap; }
.page-head h1 { font-size: 18px; }
.page-head p { margin-top: 4px; color: var(--text-3); font-size: 12px; }
.head-stat { color: var(--text-3); font-size: 12px; }
.section-title { margin-bottom: 14px; font-size: 14px; }
.create-card .create-row { display: flex; gap: 10px; flex-wrap: wrap; }
.name-input { flex: 1; min-width: 200px; }
.parent-select { width: 260px; }
.cat-group { margin-bottom: 14px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 10px; }
.cat-row { display: flex; align-items: center; gap: 8px; padding: 5px 4px; color: var(--accent); }
.cat-row.parent { color: var(--text-1); }
.cat-row.child { padding-left: 26px; color: var(--text-2); }
.cat-name { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.cat-count { color: var(--text-3); font-family: 'JetBrains Mono', monospace; font-size: 11px; }
.cat-row .icon-btn { margin-left: auto; }
.cat-row .icon-btn + .icon-btn { margin-left: 0; }
.rename-input { width: 220px; }
.cat-child-empty { padding: 2px 4px 6px 26px; color: var(--text-3); font-size: 11.5px; }
.empty-tip { color: var(--text-3); font-size: 12.5px; }
.move-table { width: 100%; border-collapse: collapse; font-size: 12.5px; }
.move-table th { padding: 8px 10px; color: var(--text-3); text-align: left; font-weight: 500; font-size: 11.5px; border-bottom: 1px solid var(--line); }
.move-table td { padding: 8px 10px; border-bottom: 1px solid var(--line); color: var(--text-2); }
.move-row { display: flex; gap: 8px; }
.move-select { min-width: 180px; }
.mono { font-family: 'JetBrains Mono', monospace; }
</style>
