<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { RouteName } from '@/router/route-names'
import { useAuthStore } from '@/stores/auth.store'
import { useTaskStore } from '@/stores/task.store'
import { dueDateLabel, isOverdue, progressTone } from '@/features/tasks/task.helpers'

defineOptions({ name: 'MyTaskPanel' })

/** 面板最多列出几条；更多任务用「查看全部」进入图纸库按图号继续核对。 */
const VISIBLE_LIMIT = 4

const router = useRouter()
const authStore = useAuthStore()
const taskStore = useTaskStore()

const tasks = computed(() => taskStore.myTasks)
const visibleTasks = computed(() => tasks.value.slice(0, VISIBLE_LIMIT))
const activeCount = computed(() => tasks.value.filter((task) => !task.progress.done).length)
const doneCount = computed(() => tasks.value.filter((task) => task.progress.done).length)
const overdueCount = computed(() => visibleTasks.value.filter((task) => isOverdue(task.assignment?.dueDate)).length)

function openDrawing(no: string) {
  void router.push({ name: RouteName.DrawingPreview, params: { drawingId: no } })
}

function openBoard() {
  void router.push({ name: RouteName.TaskBoard })
}
</script>

<template>
  <section class="wb-panel my-task-panel" aria-label="我的任务">
    <div class="wb-panel-heading">
      <h2><DemoIcon name="clipboard-list" :size="17" />我的任务</h2>
      <button v-if="authStore.canAssignTasks" class="wb-text-button" type="button" @click="openBoard">
        任务管理台<DemoIcon name="arrow-up-right" :size="14" />
      </button>
    </div>

    <p v-if="taskStore.errorMine" class="wb-inline-message" role="alert">
      {{ taskStore.errorMine }}
      <button class="wb-text-button" type="button" @click="taskStore.loadMine()">重新加载</button>
    </p>
    <p v-else-if="taskStore.loadingMine && !tasks.length" class="wb-inline-message">正在加载我的任务…</p>
    <p v-else-if="!tasks.length" class="wb-inline-message">
      当前没有指派给你的图纸。被指派为负责人后，任务和进度会显示在这里。
    </p>

    <template v-else>
      <div class="my-task-counts">
        <span><b>{{ activeCount }}</b> 进行中</span>
        <span><b>{{ doneCount }}</b> 已完成</span>
        <span v-if="overdueCount" class="overdue"><b>{{ overdueCount }}</b> 已逾期</span>
      </div>
      <ul class="my-task-list">
        <li v-for="task in visibleTasks" :key="task.assignment?.taskId || task.drawing.id">
          <button class="my-task-title" type="button" @click="openDrawing(task.drawing.no)">
            <span class="wb-file-icon"><DemoIcon name="file" :size="18" /></span>
            <span>
              <strong>{{ task.drawing.name || task.drawing.no }}</strong>
              <small>{{ task.drawing.no }}</small>
            </span>
          </button>
          <div class="my-task-progress" :class="progressTone(task)">
            <div class="my-task-bar" aria-hidden="true"><span :style="{ width: `${task.progress.percent}%` }" /></div>
            <div class="my-task-meta">
              <span class="wb-status" :class="task.progress.done ? 'published' : 'draft'"><i />{{ task.progress.stage }}</span>
              <b>{{ task.progress.percent }}%</b>
            </div>
          </div>
          <p class="my-task-detail">{{ task.progress.detail }}</p>
          <p v-if="task.assignment?.dueDate" class="my-task-due" :class="{ overdue: isOverdue(task.assignment.dueDate) }">
            <DemoIcon name="calendar" :size="12" />{{ task.assignment.dueDate }} · {{ dueDateLabel(task.assignment.dueDate) }}
          </p>
          <p v-else-if="task.assignment?.note" class="my-task-note">{{ task.assignment.note }}</p>
        </li>
      </ul>
      <footer class="my-task-footer">
        共 {{ tasks.length }} 项指派给你的图纸<template v-if="tasks.length > VISIBLE_LIMIT">，显示前 {{ VISIBLE_LIMIT }} 项</template>
      </footer>
    </template>
  </section>
</template>

<style scoped>
.my-task-panel {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.my-task-counts {
  display: flex;
  gap: 14px;
  font-size: 11.5px;
  color: var(--text-3);
}
.my-task-counts b {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 15px;
  color: var(--text-1);
  margin-right: 4px;
}
.my-task-counts .overdue b {
  color: var(--danger, #c0392b);
}
.my-task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.my-task-list li {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--line);
}
.my-task-list li:last-child {
  border-bottom: none;
  padding-bottom: 0;
}
.my-task-title {
  display: flex;
  align-items: center;
  gap: 8px;
  background: none;
  border: none;
  text-align: left;
  color: var(--text-1);
}
.my-task-title strong {
  font-size: 12.5px;
}
.my-task-title small {
  display: block;
  color: var(--text-3);
  font-size: 11px;
}
.my-task-progress {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.my-task-bar {
  height: 5px;
  border-radius: 999px;
  background: var(--line);
  overflow: hidden;
}
.my-task-bar span {
  display: block;
  height: 100%;
  background: var(--accent);
}
.my-task-progress.positive .my-task-bar span {
  background: var(--ok, #408b70);
}
.my-task-progress.attention .my-task-bar span {
  background: var(--warn, #c98a2e);
}
.my-task-progress.muted .my-task-bar span {
  background: var(--text-3);
}
.my-task-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 11.5px;
  color: var(--text-2);
}
.my-task-detail,
.my-task-note {
  font-size: 11.5px;
  color: var(--text-3);
  line-height: 1.6;
}
.my-task-due {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11.5px;
  color: var(--text-3);
}
.my-task-due.overdue {
  color: var(--danger, #c0392b);
  font-weight: 600;
}
.my-task-footer {
  color: var(--text-3);
  font-size: 11px;
}
</style>
