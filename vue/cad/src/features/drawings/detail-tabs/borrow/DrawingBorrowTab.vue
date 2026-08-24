<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDomainStore } from '@/stores/domain.store'

defineOptions({ name: 'DrawingBorrowTab' })

const domainStore = useDomainStore()
</script>

<template>
  <div class="borrow-page">
    <div class="card borrow-card">
      <div class="card-title">
        <DemoIcon name="share-2" :size="16" />
        借用记录清单
        <span class="hint">借出 / 借入项目 · 零件版本 · 借用人追溯</span>
      </div>
      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>方向</th>
              <th>关联项目</th>
              <th>零件图号 / 版本</th>
              <th>借用人</th>
              <th>记录日期</th>
              <th>当前状态</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in domainStore.borrows" :key="`${record.project}-${record.part}`">
              <td>
                <span class="tag" :class="record.dir === 'out' ? 'info' : 'plain'">
                  {{ record.dir === 'out' ? '借出' : '借入' }}
                </span>
              </td>
              <td>{{ record.project }}</td>
              <td class="num">{{ record.part }}</td>
              <td>{{ record.user }}</td>
              <td class="num">{{ record.date }}</td>
              <td>
                <span class="tag" :class="record.status === '使用中' ? 'ok' : 'mute'">{{ record.status }}</span>
              </td>
            </tr>
            <tr v-if="!domainStore.borrows.length">
              <td colspan="6">
                <div class="empty">
                  <DemoIcon name="share-2" :size="34" />
                  <div class="t">暂无图纸借用记录</div>
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
  min-width: 700px;
}
</style>
