<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { STATUS, useDemoStore } from '@/stores/demo.store'
import { useUiStore } from '@/stores/ui.store'
import type { DrawingStatus } from '@/types/demo.types'

defineOptions({
  name: 'DrawingLibraryPage',
})

const router = useRouter()
const demoStore = useDemoStore()
const uiStore = useUiStore()
const query = ref('')
const kind = ref('')
const status = ref<DrawingStatus | ''>('')
const menuFor = ref<string | null>(null)

const rows = computed(() => {
  const q = query.value.trim().toLowerCase()
  return demoStore.drawings.filter((drawing) => {
    const matchesQuery = !q || [drawing.no, drawing.name, drawing.vendor, drawing.material, drawing.project].join(' ').toLowerCase().includes(q)
    const matchesKind = !kind.value || drawing.kind === kind.value
    const matchesStatus = !status.value || drawing.status === status.value
    return matchesQuery && matchesKind && matchesStatus
  })
})

function openDetail(no: string) {
  demoStore.openDrawing(no)
  router.push({ name: 'drawing-preview', params: { drawingId: no } })
}

function createDrawing() {
  uiStore.openModal('create-drawing', '创建图纸')
}

function toggleMenu(no: string) {
  menuFor.value = menuFor.value === no ? null : no
}

function menuAction(action: string, no: string) {
  menuFor.value = null
  if (action === 'detail') openDetail(no)
  if (action === 'upload') uiStore.openModal('upload-version', '上传新版本')
  if (action === 'borrow') uiStore.openModal('borrow-drawing', '借用图纸')
  if (action === 'hide') uiStore.toast('图纸已隐藏：用户不可见，管理员可随时恢复，历史完整保留', 'warn')
}
</script>

<template>
  <div class="page library-page">
    <div class="lib-head">
      <span class="lib-title">图纸库</span>
      <span class="lib-count">{{ rows.length }} / {{ demoStore.drawings.length }} 项</span>

      <label class="search-box">
        <DemoIcon name="search" :size="15" />
        <input v-model="query" placeholder="搜索图号 / 名称 / 厂商 / 材料…" />
      </label>

      <select v-model="kind" class="inp filter-select">
        <option value="">全部类型</option>
        <option value="总图">总图</option>
        <option value="零件图">零件图</option>
      </select>

      <select v-model="status" class="inp filter-select">
        <option value="">全部状态</option>
        <option v-for="(item, key) in STATUS" :key="key" :value="key">{{ item.t }}</option>
      </select>

      <button class="btn primary" type="button" @click="createDrawing">
        <DemoIcon name="plus" :size="14" />创建图纸
      </button>
    </div>

    <div class="card library-card">
      <table class="tbl">
        <thead>
          <tr>
            <th>图号</th>
            <th>名称</th>
            <th>类型</th>
            <th>材料</th>
            <th>厂商</th>
            <th>状态</th>
            <th>版本</th>
            <th>更新</th>
            <th class="operation-column">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="drawing in rows" :key="drawing.no">
            <td class="link num" @click="openDetail(drawing.no)">{{ drawing.no }}</td>
            <td class="drawing-name">
              {{ drawing.name }}
              <span v-if="drawing.borrowFrom" class="tag plain borrow-tag">借用·{{ drawing.borrowFrom }}</span>
            </td>
            <td><span class="tag" :class="drawing.kind === '总图' ? 'plain' : 'mute'">{{ drawing.kind }}</span></td>
            <td class="num">{{ drawing.material }}</td>
            <td>{{ drawing.vendor }}</td>
            <td><span class="tag" :class="STATUS[drawing.status].c">{{ STATUS[drawing.status].t }}</span></td>
            <td class="num">{{ drawing.ver }}</td>
            <td class="num updated">{{ drawing.updated }}</td>
            <td class="row-actions">
              <button class="btn sm" type="button" @click="openDetail(drawing.no)"><DemoIcon name="eye" :size="14" />详情</button>
              <div class="row-menu-wrap">
                <button class="icon-btn row-menu-button" type="button" @click.stop="toggleMenu(drawing.no)"><DemoIcon name="ellipsis" :size="16" /></button>
                <div v-if="menuFor === drawing.no" class="dropdown row-dropdown">
                  <button class="dd-item" type="button" @click="menuAction('detail', drawing.no)"><DemoIcon name="eye" :size="14" />查看详情</button>
                  <button class="dd-item" type="button" @click="menuAction('upload', drawing.no)"><DemoIcon name="download" :size="14" />下载原始文件</button>
                  <button class="dd-item" type="button" @click="menuAction('borrow', drawing.no)"><DemoIcon name="share-2" :size="14" />借用此图</button>
                  <div class="dd-sep"></div>
                  <button class="dd-item" type="button" @click="menuAction('hide', drawing.no)"><DemoIcon name="eye-off" :size="14" />隐藏图纸（管理员）</button>
                </div>
              </div>
            </td>
          </tr>
          <tr v-if="!rows.length">
            <td colspan="9">
              <div class="empty"><DemoIcon name="search-x" :size="34" /><div class="t">{{ demoStore.drawings.length ? '没有匹配的图纸，试试更换关键词' : '暂无图纸，请先创建或导入图纸' }}</div></div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.library-page {
  min-width: 0;
}

.lib-head {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.lib-title {
  margin-right: 4px;
  font-family: var(--font-display);
  font-size: 19px;
  font-weight: 900;
}

.lib-count {
  color: var(--text-3);
  font-family: 'JetBrains Mono', monospace;
  font-size: 11.5px;
}

.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 320px;
  height: 36px;
  margin-left: auto;
  padding: 0 13px;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel);
  color: var(--text-3);
  transition: all 0.25s;
}

.search-box:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.search-box input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  background: none;
  font-size: 12.5px;
}

.filter-select {
  width: 110px;
  height: 36px;
}

.library-card {
  overflow: visible;
}

.drawing-name {
  font-weight: 500;
  white-space: nowrap;
}

.borrow-tag {
  margin-left: 4px;
  padding: 1px 7px;
  font-size: 9.5px;
}

.updated {
  color: var(--text-3);
}

.operation-column {
  width: 130px;
}

.row-actions {
  position: relative;
  white-space: nowrap;
}

.row-menu-wrap {
  position: relative;
  display: inline-block;
}

.row-menu-button {
  display: inline-grid;
  width: 28px;
  height: 28px;
  vertical-align: middle;
}

.row-dropdown {
  top: 34px;
  right: 0;
}

@media (max-width: 1180px) {
  .search-box {
    margin-left: 0;
  }
}

@media (max-width: 760px) {
  .lib-head {
    align-items: stretch;
  }

  .lib-title,
  .lib-count {
    width: auto;
  }

  .search-box {
    width: 100%;
  }

  .library-card {
    overflow-x: auto;
  }

  .tbl {
    min-width: 920px;
  }
}
</style>
