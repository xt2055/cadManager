<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'
import { onMounted } from 'vue'
import { useReviewStore } from '@/stores/review.store'

defineOptions({ name: 'ReviewCompletedPage' })

const reviewStore = useReviewStore()

onMounted(() => { void reviewStore.load().catch(() => undefined) })
</script>

<template>
  <div class="page review-completed-page">
    <div class="section-head">
      <h3>已办审核归档</h3>
      <span class="lib-count">所有历史审核意见与流转结果永久追溯</span>
    </div>

    <div v-if="reviewStore.completed.length" class="card history-card">
      <div class="table-pad">
        <table class="tbl">
          <thead>
            <tr>
              <th>图号</th>
              <th>图纸名称</th>
              <th>审核专业节点</th>
              <th>审核人</th>
              <th>审核结论</th>
              <th>签署审核意见</th>
              <th>审核时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in reviewStore.completed" :key="item.id">
              <td class="num mono link">{{ item.no }}</td>
              <td><b>{{ item.name }}</b></td>
              <td><span class="tag plain">{{ item.node }}</span></td>
              <td>{{ item.reviewer }}</td>
              <td>
                <span class="tag" :class="item.result === 'pass' ? 'ok' : 'danger'">
                  {{ item.result === 'pass' ? '通过' : '驳回' }}
                </span>
              </td>
              <td class="opinion-cell">{{ item.opinion }}</td>
              <td class="num updated">{{ item.time }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div v-else class="card empty">
      <DemoIcon name="clipboard-check" :size="36" />
      <div class="t">暂无已办审核记录</div>
      <p>处理完待办专业审核后，在此汇总留存历史签署凭证</p>
    </div>
  </div>
</template>

<style scoped>
.review-completed-page {
  min-width: 0;
}
.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 4px 0 16px;
}
.section-head h3 {
  font-family: var(--font-display);
  font-size: 16px;
  font-weight: 800;
}
.lib-count {
  color: var(--text-3);
  font-size: 11.5px;
}
.history-card {
  overflow: visible;
}
.table-pad {
  padding: 10px 14px;
  overflow-x: auto;
}
.opinion-cell {
  color: var(--text-2);
  font-size: 12px;
  max-width: 320px;
}
</style>
