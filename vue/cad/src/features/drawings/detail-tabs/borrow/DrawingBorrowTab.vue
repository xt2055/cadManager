<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'
import type { BorrowRecord, StructurePart } from '@/types/domain.types'
import { isSameDrawingFamily } from '@/utils/drawing-number-parser'

defineOptions({ name: 'DrawingBorrowTab' })

const router = useRouter()
const domainStore = useDomainStore()
const currentNo = computed(() => domainStore.currentDrawing?.no || '')

interface BorrowRow {
  partNo: string
  partName: string
  drawingNo: string
  user: string
  date: string
  status: BorrowRecord['status']
}

function recordMatchesPart(record: BorrowRecord, part: StructurePart): boolean {
  return record.partNo === part.no || record.part.startsWith(`${part.no} `)
}

function belongsToCurrentProject(part: StructurePart, projectNo: string): boolean {
  if (part.parentNo === projectNo || part.no.startsWith(`${projectNo}-`)) return true
  if (part.files?.some((file) => file.drawingNo === projectNo) || part.otherFiles?.some((file) => file.drawingNo === projectNo)) return true

  const visited = new Set<string>()
  let parentNo = part.parentNo
  while (parentNo && !visited.has(parentNo)) {
    if (parentNo === projectNo) return true
    visited.add(parentNo)
    parentNo = domainStore.structure.find((candidate) => candidate.no === parentNo)?.parentNo || ''
  }
  return false
}

const borrowedRows = computed<BorrowRow[]>(() => {
  const current = currentNo.value
  if (!current) return []

  const rows: BorrowRow[] = []
  const borrowedParts = domainStore.structure.filter((part) => {
    if (!belongsToCurrentProject(part, current)) return false
    const sourceNo = part.borrowFrom?.trim() || ''
    // 只有创建时明确标记来源、且来源不属于当前项目族的零件才是借用件。
    // 不能从零件图号或文件存在与否推断借用关系。
    return Boolean(sourceNo && sourceNo !== current && !isSameDrawingFamily(part.no, current))
  })
  for (const part of borrowedParts) {
    const sourceNo = part.borrowFrom?.trim() || ''
    const record = domainStore.borrows.find((item) => item.dir === 'in' && recordMatchesPart(item, part) && item.targetDrawingNo === current)
    rows.push({
      partNo: part.no,
      partName: part.name,
      drawingNo: record?.sourceDrawingNo || sourceNo,
      user: record?.user || '历史记录',
      date: record?.date || '历史记录',
      status: record?.status || '使用中',
    })
  }

  const knownPartNos = new Set(rows.map((row) => row.partNo))

  for (const record of domainStore.borrows) {
    if (record.dir !== 'in' || record.targetDrawingNo !== current || !record.sourceDrawingNo) continue
    const partNo = record.partNo || record.part.split(/\s+/, 1)[0] || ''
    if (!partNo || knownPartNos.has(partNo)) continue
    if (isSameDrawingFamily(partNo, current)) continue
    rows.push({
      partNo,
      partName: record.partName || record.part.replace(partNo, '').trim() || '未命名零件',
      drawingNo: record.sourceDrawingNo,
      user: record.user,
      date: record.date,
      status: record.status,
    })
  }

  return rows.sort((left, right) => left.partNo.localeCompare(right.partNo, undefined, { numeric: true }))
})

function hasDrawing(no: string): boolean {
  return Boolean(no && domainStore.drawings.some((drawing) => drawing.no === no))
}

function openDrawing(no: string) {
  if (!hasDrawing(no)) return
  domainStore.openDrawing(no)
  void router.push({ name: 'drawing-preview', params: { drawingId: no } })
}
</script>

<template>
  <div class="borrow-page">
    <div class="card borrow-card">
      <div class="card-title">
        <DemoIcon name="share-2" :size="16" />
        借用零件清单
        <span class="hint">当前项目实际挂载的借用零件 · 来源图纸追溯</span>
      </div>
      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>零件图号</th>
              <th>借用零件</th>
              <th>借用图纸号</th>
              <th>借用人</th>
              <th>记录日期</th>
              <th>当前状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in borrowedRows" :key="`${row.partNo}-${row.drawingNo}`">
              <td class="num mono">{{ row.partNo }}</td>
              <td>{{ row.partName }}</td>
              <td>
                <button v-if="hasDrawing(row.drawingNo)" class="drawing-link mono" type="button" @click="openDrawing(row.drawingNo)">
                  {{ row.drawingNo }}<DemoIcon name="arrow-up-right" :size="12" />
                </button>
                <span v-else class="missing-project">没有上传该项目</span>
              </td>
              <td>{{ row.user }}</td>
              <td class="num">{{ row.date }}</td>
              <td><span class="tag" :class="row.status === '使用中' ? 'ok' : 'mute'">{{ row.status }}</span></td>
            </tr>
            <tr v-if="!borrowedRows.length">
              <td colspan="6">
                <div class="empty">
                  <DemoIcon name="share-2" :size="34" />
                  <div class="t">暂无借用零件</div>
                  <p>当前项目还没有挂载其他项目的零件图</p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.borrow-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.borrow-card {
  overflow: visible;
}
.table-pad {
  padding: 10px 14px;
  overflow-x: auto;
}
.tbl {
  min-width: 760px;
}
.drawing-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--accent);
}
.drawing-link:hover {
  text-decoration: underline;
}
.missing-project {
  color: var(--text-3);
  font-size: 12px;
}
</style>
