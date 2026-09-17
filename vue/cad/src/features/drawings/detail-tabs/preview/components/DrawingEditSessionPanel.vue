<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'DrawingEditSessionPanel' })

/**
 * 本地 CAD 协同状态面板：正在编辑的会话（心跳 / 复制路径 / 呼出 CAXA / 结束编辑）与结束成功反馈。
 * 不引入 store / composable——每行的「心跳是否正常」「是否正在结束」都由父页面投影成布尔串，
 * 动作只 emit 会话 id 或路径，真实会话对象留在父页面手里。
 */
defineProps<{
  activeSessions: {
    sessionId: string
    fileName: string
    startedAt: string
    uncPath: string
    /** 心跳是否新鲜（原 isHeartbeatFresh(session)）。 */
    heartbeatFresh: boolean
    /** 是否正在结束（原 closingSessionIds.has(sessionId)）。 */
    closing: boolean
  }[]
  recentlyClosed: { sessionId: string; fileName: string; savedAt: string }[]
}>()

const emit = defineEmits<{
  /** 复制共享路径（父页面负责提示文案）。 */
  copyPath: [path: string]
  /** 重新呼出本地 CAD。 */
  relaunch: [sessionId: string]
  /** 结束编辑（父页面按 id 找回落盘会话）。 */
  stop: [sessionId: string]
}>()
</script>

<template>
  <div v-if="activeSessions.length > 0" class="collab-multi-container">
    <div v-for="session in activeSessions" :key="session.sessionId" class="card collab-dock-card">
      <div class="dock-left">
        <div class="dock-status-tag">
          <span class="pulse-dot"></span>
          <strong>本地协同编辑中</strong>
        </div>
        <div class="dock-file-info">
          <span class="file-name" :title="session.fileName">{{ session.fileName }}</span>
          <span class="dock-time">
            开始于 {{ session.startedAt }} ·
            <b :class="session.heartbeatFresh ? 'hb-ok' : 'hb-lost'">{{ session.heartbeatFresh ? '心跳正常' : '心跳检测中' }}</b>
            · {{ session.heartbeatFresh ? '自动落盘与版本保护生效中' : '等待心跳确认...' }}
          </span>
        </div>
      </div>
      <div class="dock-actions">
        <button class="btn sm" type="button" title="在外部 CAD 或资源管理器中打开此共享路径" @click="emit('copyPath', session.uncPath)">
          <DemoIcon name="copy" :size="13" />复制路径
        </button>
        <button class="btn sm" type="button" title="重新唤醒本地 CAXA CAD 程序" @click="emit('relaunch', session.sessionId)">
          <DemoIcon name="external-link" :size="13" />呼出 CAXA
        </button>
        <button
          class="btn sm primary danger-tone"
          type="button"
          :disabled="session.closing"
          title="结束当前编辑：等待图纸落盘后生成新版本并释放文件锁"
          @click="emit('stop', session.sessionId)"
        >
          <span v-if="session.closing" class="local-edit-spinner" aria-hidden="true"></span>
          <DemoIcon v-else name="square" :size="12" />
          {{ session.closing ? '正在结束，等待图纸落盘...' : '结束编辑' }}
        </button>
      </div>
    </div>
  </div>

  <!-- 结束成功反馈：后端完成版本捕获后短暂展示 -->
  <div v-if="recentlyClosed.length > 0" class="collab-multi-container">
    <div v-for="closed in recentlyClosed" :key="closed.sessionId" class="card collab-dock-card closed-ok">
      <div class="dock-left">
        <div class="dock-status-tag success">
          <DemoIcon name="check-circle-2" :size="15" />
          <strong>结束成功</strong>
        </div>
        <div class="dock-file-info">
          <span class="file-name" :title="closed.fileName">{{ closed.fileName }}</span>
          <span class="dock-time">{{ closed.savedAt }} · 图纸已落盘并生成新版本</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 协同状态面板专属样式 + 本组件用到的微件（.pulse-dot / .local-edit-spinner）。 */

.dock-time .hb-ok {
  color: var(--ok, #16a34a);
}

.dock-time .hb-lost {
  color: var(--warn, #f59e0b);
}

.collab-multi-container {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 100%;
}

.collab-dock-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  border: 1px solid var(--accent);
  background: linear-gradient(135deg, var(--panel) 0%, var(--panel-2) 100%);
  border-radius: 12px;
  box-shadow: 0 4px 16px -4px rgba(0, 0, 0, 0.1);
}

.dock-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.dock-status-tag {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 4px 10px;
  background: var(--ok-soft, rgba(34, 197, 94, 0.12));
  color: var(--ok, #16a34a);
  border-radius: 99px;
  font-size: 11.5px;
  font-weight: 700;
  white-space: nowrap;
  flex: none;
}

.dock-file-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.dock-file-info .file-name {
  color: var(--text-1);
  font-size: 13px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dock-file-info .dock-time {
  color: var(--text-3);
  font-size: 11px;
}

.dock-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
}

.danger-tone {
  background: var(--danger, #ef4444) !important;
  color: #fff !important;
  border-color: transparent !important;
}

.dock-status-tag.success {
  color: var(--ok);
}

.dock-status-tag.success :deep(svg),
.dock-status-tag.success strong {
  color: var(--ok);
}

.closed-ok {
  border-color: rgb(52 211 153 / 40%);
  background: rgb(52 211 153 / 6%);
}

/* .pulse-dot / .local-edit-spinner 是本特性多个组件共用的微件；父页面与文件表格也各有一份 scoped 副本
   （scoped 样式无法跨组件复用），keyframes 名在各自 scoped 下会自动带 scope，不会冲突。 */
.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--ok, #22c55e);
  box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  animation: pulse-ring 1.8s infinite cubic-bezier(0.66, 0, 0, 1);
  flex: none;
}

@keyframes pulse-ring {
  0% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.7);
  }
  70% {
    box-shadow: 0 0 0 10px rgba(34, 197, 94, 0);
  }
  100% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0);
  }
}

.local-edit-spinner {
  width: 13px;
  height: 13px;
  flex: none;
  border: 2px solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: local-edit-spin 0.75s linear infinite;
}

@keyframes local-edit-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .local-edit-spinner {
    animation-duration: 1.5s;
  }
}
</style>
