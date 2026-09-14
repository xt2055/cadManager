<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { lifecycleApi, downloadEvidence, type LifecycleTree } from '@/services/lifecycle.service'
import EvidenceDocuments from '../components/EvidenceDocuments.vue'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useDrawingStore } from '@/stores/drawing.store'
import { useUiStore } from '@/stores/ui.store'
import { formatReadableDateTime } from '@/utils/date-time'

const route = useRoute()
const drawings = useDrawingStore()
const uiStore = useUiStore()

const tree = ref<LifecycleTree | null>(null)
const error = ref('')
const loading = ref(false)

const labels: Record<string, string> = {
  pending_approval: '待审批',
  executing: '设计中',
  pending_verify: '审核中',
  completed: '已发布',
  rejected: '已驳回',
  cancelled: '已终止',
  pending: '待审核',
  pass: '通过',
  returned: '退回修改',
  accepted: '已发布',
  reviewing: '审核中',
  published: '审核通过',
  archived: '正式在用',
}

function getStatusTagClass(status: string): string {
  if (['archived', 'completed', 'pass', 'accepted', 'published'].includes(status)) return 'ok'
  if (['executing', 'pending_verify', 'reviewing'].includes(status)) return 'plain'
  if (['pending_approval', 'pending'].includes(status)) return 'warn'
  if (['rejected', 'cancelled', 'returned'].includes(status)) return 'danger'
  return 'mute'
}

let generation = 0

async function load() {
  const seq = ++generation
  loading.value = true
  error.value = ''
  try {
    const drawing = drawings.getDrawing(String(route.params.drawingId || ''))
    if (!drawing) throw new Error('请从所属总图或工程进入生命周期看板')
    const result = await lifecycleApi<LifecycleTree>(
      `/lifecycle-tree?drawingId=${encodeURIComponent(drawing.id)}`,
    )
    if (seq === generation) tree.value = result
  } catch (e) {
    if (seq === generation) error.value = (e as Error).message
  } finally {
    if (seq === generation) loading.value = false
  }
}

watch(
  () => route.params.drawingId,
  () => {
    tree.value = null
    void load()
  },
  { immediate: true },
)

async function download(id: string | undefined, name: string) {
  if (!id) return
  try {
    await downloadEvidence(`/lifecycle-versions/${id}`, name)
    uiStore.toast(`已开始下载历史受控版本：${name}`, 'ok')
  } catch (e) {
    error.value = (e as Error).message
    uiStore.toast(`下载失败：${error.value}`, 'warn')
  }
}

const totalReleasesCount = computed(() => tree.value?.releases?.length ?? 0)
const totalChangesCount = computed(() => tree.value?.changes?.length ?? 0)
const totalReviewsCount = computed(() => tree.value?.reviews?.length ?? 0)
</script>

<template>
  <main class="lifecycle-container" :aria-busy="loading">
    <!-- 头部卡片 -->
    <header class="lifecycle-header card">
      <div class="header-left">
        <div class="header-icon">
          <DemoIcon name="workflow" :size="24" />
        </div>
        <div>
          <div class="header-title-row">
            <h2>图纸工程生命周期数字脉络</h2>
            <span v-if="tree" class="tag" :class="getStatusTagClass(tree.status)">
              {{ labels[tree.status] || tree.status }}
            </span>
          </div>
          <p>全景串联总图资产首次设计、审核签署、受控发布、变更工单及证据链的全生命周期追溯树。</p>
        </div>
      </div>
      <div class="header-right">
        <button class="btn sm" type="button" :disabled="loading" @click="load">
          <DemoIcon name="rotate-cw" :size="13" class="action-icon" :class="{ 'spin-icon': loading }" />
          刷新记录
        </button>
      </div>
    </header>

    <!-- 错误警告提示 -->
    <div v-if="error" class="card card-pad error-alert" role="alert">
      <DemoIcon name="alert-triangle" :size="18" />
      <div class="alert-content">
        <b>读取生命周期数据失败</b>
        <span>{{ error }}</span>
      </div>
      <button class="btn sm" type="button" @click="load">重试</button>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="card card-pad loading-card">
      <DemoIcon name="rotate-cw" :size="24" class="spin-icon" />
      <span>正在串联图纸全生命周期脉络与历史原件…</span>
    </div>

    <template v-else-if="tree">
      <!-- 根图号资产概览卡片 -->
      <section class="card card-pad asset-root-card">
        <div class="asset-main">
          <div class="asset-no-row">
            <span class="mono asset-no">{{ tree.drawingNo }}</span>
            <span class="version-pill mono">{{ tree.version }}</span>
          </div>
          <h3 class="asset-name">{{ tree.name }}</h3>
        </div>
        <div class="asset-stats-grid">
          <div class="stat-box">
            <span class="stat-lbl">正式发布档案</span>
            <b class="stat-val mono">{{ totalReleasesCount }} <small>版</small></b>
          </div>
          <div class="stat-box">
            <span class="stat-lbl">变更历程工单</span>
            <b class="stat-val mono">{{ totalChangesCount }} <small>项</small></b>
          </div>
          <div class="stat-box">
            <span class="stat-lbl">审核流程记录</span>
            <b class="stat-val mono">{{ totalReviewsCount }} <small>次</small></b>
          </div>
        </div>
      </section>

      <!-- 时间线时间轴大容器 -->
      <div class="timeline-threads">
        <!-- 阶段 1：正式发布档案快照（Releases） -->
        <section class="thread-section card card-pad">
          <div class="thread-head">
            <div class="thread-badge-icon release-icon">
              <DemoIcon name="archive" :size="18" />
            </div>
            <div class="thread-title-wrap">
              <h4>正式发布与受控快照档案</h4>
              <p>每次图纸经完整审核发布后固化的基线档案，包含当时的零件拓扑、备料表及全套签署记录</p>
            </div>
            <span class="badge muted-badge">{{ totalReleasesCount }} 个版本</span>
          </div>

          <div v-if="!tree.releases?.length" class="empty-branch-msg">
            <DemoIcon name="info" :size="14" />
            <span>首次审核通过并存档后，将自动建立首个正式在用发布档案。</span>
          </div>

          <div v-else class="releases-flow">
            <article
              v-for="release in tree.releases"
              :key="release.id"
              class="card card-pad release-snapshot-card"
            >
              <div class="snapshot-header">
                <div class="snapshot-title-row">
                  <span class="version-tag mono">{{ release.version }}</span>
                  <span class="tag" :class="release.version === tree.version ? 'ok' : 'mute'">
                    {{ release.version === tree.version ? '当前在用基准版' : '历史受控存档' }}
                  </span>
                  <span class="source-tag plain">{{ release.source || '正式发布' }}</span>
                </div>
                <time class="mono text-time">{{ formatReadableDateTime(release.createdAt) }}</time>
              </div>

              <!-- 当版参数规格 -->
              <div class="snapshot-spec-grid">
                <div class="spec-col">
                  <span class="lbl">图纸名称</span>
                  <span class="val">{{ release.snapshot.drawing.name }}</span>
                </div>
                <div class="spec-col">
                  <span class="lbl">材料规格</span>
                  <span class="val">{{ release.snapshot.drawing.material || '—' }}</span>
                </div>
                <div class="spec-col">
                  <span class="lbl">结构零件数</span>
                  <span class="val">{{ release.snapshot.structure?.length ?? 0 }} 项关联零件</span>
                </div>
              </div>

              <!-- 签署人员名单 -->
              <div class="signers-bar">
                <span class="signers-lbl">签署凭证:</span>
                <div v-if="release.snapshot.signers?.length" class="signers-chips">
                  <span
                    v-for="(s, idx) in release.snapshot.signers"
                    :key="idx"
                    class="signer-chip"
                  >
                    <DemoIcon name="stamp" :size="11" />
                    <strong>{{ s.role }}:</strong>
                    <span>{{ s.signer_name }}</span>
                  </span>
                </div>
                <span v-else class="empty-muted">原记录未登记具体签署专家</span>
              </div>

              <!-- 文件原件清单及版本下载 -->
              <div class="files-archive-block">
                <span class="files-lbl">本版本固化的图纸文件清单（{{ release.snapshot.files?.length ?? 0 }}）:</span>
                <div class="files-cards-grid">
                  <div
                    v-for="file in release.snapshot.files"
                    :key="file.attachmentId"
                    class="file-version-card"
                  >
                    <DemoIcon name="file" :size="14" />
                    <span class="file-name" :title="file.name">{{ file.name }}</span>
                    <button
                      class="btn sm download-file-btn"
                      type="button"
                      :disabled="!file.versionId"
                      title="下载此发布版本受控原件"
                      @click="download(file.versionId, file.name)"
                    >
                      <DemoIcon name="download" :size="12" />
                      下载原件
                    </button>
                  </div>
                </div>
              </div>

              <!-- 当版备料表快照折叠 -->
              <details v-if="release.snapshot.bom?.length" class="bom-details">
                <summary>
                  <DemoIcon name="file-spreadsheet" :size="13" />
                  <span>当时的备料表清单（共 {{ release.snapshot.bom.length }} 项物料）</span>
                </summary>
                <div class="table-scroll-wrap">
                  <table class="tbl bom-table">
                    <thead>
                      <tr>
                        <th style="width: 60px;">序号</th>
                        <th>物料名称</th>
                        <th>规格型号</th>
                        <th style="width: 80px;">数量</th>
                        <th>备注</th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr v-for="b in release.snapshot.bom" :key="b.item_no">
                        <td class="mono">{{ b.item_no }}</td>
                        <td><strong>{{ b.name }}</strong></td>
                        <td class="mono">{{ b.spec }}</td>
                        <td class="mono">{{ b.quantity }}</td>
                        <td>{{ b.remark || '—' }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </details>
            </article>
          </div>
        </section>

        <!-- 阶段 2：首次设计出图与审核流程（Initial Reviews） -->
        <section class="thread-section card card-pad">
          <div class="thread-head">
            <div class="thread-badge-icon review-icon">
              <DemoIcon name="stamp" :size="18" />
            </div>
            <div class="thread-title-wrap">
              <h4>首次设计与出图审核流程</h4>
              <p>图纸立项后的首次全流程校审、设计复核、工艺校对及批准各节点签署日志</p>
            </div>
            <span class="badge muted-badge">{{ totalReviewsCount }} 轮记录</span>
          </div>

          <div v-if="!tree.reviews?.length" class="empty-branch-msg">
            <DemoIcon name="info" :size="14" />
            <span>暂无首次线上审核记录；导入的历史老图纸将直接保留其既有历史归属。</span>
          </div>

          <div v-else class="reviews-list">
            <article
              v-for="review in tree.reviews"
              :key="review.id"
              class="card card-pad review-item-card"
            >
              <div class="review-top-bar">
                <div class="review-status-row">
                  <span class="tag" :class="getStatusTagClass(review.status)">
                    {{ labels[review.status] || review.status }}
                  </span>
                  <span class="mono text-time">{{ formatReadableDateTime(review.startedAt) }} 发起</span>
                </div>
              </div>

              <!-- 审核节点卡片链 -->
              <div class="nodes-stepper-track">
                <div
                  v-for="(node, idx) in review.nodes"
                  :key="idx"
                  class="node-step-card"
                  :class="{ 'node-passed': node.status === 'pass' }"
                >
                  <div class="node-icon-indicator">
                    <DemoIcon :name="node.status === 'pass' ? 'check' : 'clock'" :size="13" />
                  </div>
                  <div class="node-main">
                    <div class="node-title-row">
                      <strong>{{ node.name }}</strong>
                      <span class="node-user">{{ node.assignedName }}</span>
                      <span class="tag" :class="getStatusTagClass(node.status)">
                        {{ labels[node.status] || node.status }}
                      </span>
                    </div>
                    <p v-if="node.opinion" class="node-opinion">{{ node.opinion }}</p>
                    <time v-if="node.reviewedAt" class="mono node-time">{{ formatReadableDateTime(node.reviewedAt) }}</time>
                  </div>
                </div>
              </div>
            </article>
          </div>
        </section>

        <!-- 阶段 3：受控工程变更演进工单（Changes & Submissions） -->
        <section class="thread-section card card-pad">
          <div class="thread-head">
            <div class="thread-badge-icon change-icon">
              <DemoIcon name="git-branch" :size="18" />
            </div>
            <div class="thread-title-wrap">
              <h4>工程变更工单与演进历程</h4>
              <p>串联历次工单申请、管理员审批动作、每轮提交的文件新旧版本对比原件及补充证据材料</p>
            </div>
            <span class="badge muted-badge">{{ totalChangesCount }} 项工单</span>
          </div>

          <div v-if="!tree.changes?.length" class="empty-branch-msg">
            <DemoIcon name="info" :size="14" />
            <span>尚无工程变更记录。后续发起的修改工单、技术依据、多轮审核成果均会在此串联展示。</span>
          </div>

          <div v-else class="changes-timeline">
            <article
              v-for="change in tree.changes"
              :key="change.id"
              class="card card-pad change-order-card"
            >
              <div class="order-card-head">
                <div class="order-lead-title">
                  <span class="order-no-pill mono">{{ change.requestNo }}</span>
                  <span class="tag" :class="getStatusTagClass(change.status)">
                    {{ labels[change.status] || change.status }}
                  </span>
                </div>
                <time class="mono text-time">{{ formatReadableDateTime(change.createdAt) }}</time>
              </div>

              <!-- 变更原因与范围 -->
              <div class="order-info-grid">
                <div class="info-block">
                  <span class="lbl">变更缘由</span>
                  <p class="val">{{ change.reason }}</p>
                </div>
                <div class="info-block">
                  <span class="lbl">影响与修改范围</span>
                  <p class="val">{{ change.scope }}</p>
                </div>
              </div>

              <!-- 申请与审批动作折叠 -->
              <details class="modern-details-card">
                <summary>
                  <DemoIcon name="history" :size="13" />
                  <span>审批动作与办理流转（共 {{ change.actions?.length ?? 0 }} 条）</span>
                </summary>
                <div class="details-body">
                  <div
                    v-for="(act, aIdx) in change.actions"
                    :key="aIdx"
                    class="action-timeline-item"
                  >
                    <div class="dot"></div>
                    <time class="mono text-time">{{ formatReadableDateTime(act.createdAt) }}</time>
                    <strong class="actor">{{ act.actor }}</strong>
                    <span class="opinion">{{ act.opinion }}</span>
                  </div>
                </div>
              </details>

              <!-- 关联证明材料 (嵌入 EvidenceDocuments) -->
              <details class="modern-details-card">
                <summary>
                  <DemoIcon name="archive" :size="13" />
                  <span>查看本次变更关联证明材料与依据原件</span>
                </summary>
                <div class="details-body embed-body">
                  <EvidenceDocuments
                    :drawing-id="tree.id"
                    :change-request-id="change.id"
                    read-only
                  />
                </div>
              </details>

              <!-- 终止前保存的工作文件 -->
              <div v-if="change.terminalFiles?.length" class="terminal-files-block">
                <span class="sub-lbl">终止前已保存的工作原件:</span>
                <div class="files-cards-grid">
                  <div
                    v-for="tf in change.terminalFiles"
                    :key="tf.versionId"
                    class="file-version-card"
                  >
                    <DemoIcon name="file" :size="14" />
                    <span class="file-name">{{ tf.name }}</span>
                    <button class="btn sm" type="button" @click="download(tf.versionId, tf.name)">
                      <DemoIcon name="download" :size="12" />
                      下载工作版
                    </button>
                  </div>
                </div>
              </div>

              <!-- 各轮提交演进卡片 (Submissions) -->
              <div v-if="change.submissions?.length" class="submissions-flow">
                <h5 class="submissions-title">
                  <DemoIcon name="rotate-cw" :size="13" />
                  各轮次修改提交与版本溯源
                </h5>

                <div
                  v-for="sub in change.submissions"
                  :key="sub.id"
                  class="card card-pad sub-round-card"
                >
                  <div class="sub-head">
                    <span class="round-badge mono">第 {{ sub.round }} 轮成果提交</span>
                    <span class="tag" :class="getStatusTagClass(sub.status)">
                      {{ labels[sub.status] || sub.status }}
                    </span>
                    <time class="mono text-time">{{ formatReadableDateTime(sub.createdAt) }}</time>
                  </div>

                  <p class="actual-changes-text">{{ sub.actualChanges }}</p>

                  <div v-if="sub.proposedAttributes && Object.keys(sub.proposedAttributes).length" class="prop-diffs">
                    <span
                      v-for="(val, key) in sub.proposedAttributes"
                      :key="key"
                      class="diff-tag"
                    >
                      {{ key }}: <strong>{{ val }}</strong>
                    </span>
                  </div>

                  <!-- 本轮变更前后受控原件对比下载 -->
                  <div class="file-versions-pair-list">
                    <div
                      v-for="sf in sub.files"
                      :key="sf.attachmentId"
                      class="file-pair-row"
                    >
                      <div class="pair-name">
                        <DemoIcon name="file" :size="14" />
                        <span>{{ sf.name }}</span>
                      </div>
                      <div class="pair-actions">
                        <button
                          class="btn sm"
                          type="button"
                          :disabled="!sf.baseVersionId"
                          title="变更前原版本基线"
                          @click="download(sf.baseVersionId, `[变更前基线]_${sf.name}`)"
                        >
                          <DemoIcon name="corner-up-left" :size="12" />
                          变更前基线
                        </button>
                        <button
                          class="btn sm primary"
                          type="button"
                          :disabled="!sf.submittedVersionId"
                          title="本轮提交的原件成果"
                          @click="download(sf.submittedVersionId, `[第${sub.round}轮修改]_${sf.name}`)"
                        >
                          <DemoIcon name="download" :size="12" />
                          本轮修改原件
                        </button>
                      </div>
                    </div>
                  </div>

                  <!-- 本轮提交时的证明材料 -->
                  <details class="modern-details-card sub-evidence-details">
                    <summary>
                      <DemoIcon name="archive" :size="12" />
                      <span>查看本轮提交时的专项证明材料</span>
                    </summary>
                    <div class="details-body embed-body">
                      <EvidenceDocuments
                        :drawing-id="tree.id"
                        :change-request-id="change.id"
                        :submission-id="sub.id"
                        read-only
                      />
                    </div>
                  </details>

                  <!-- 本轮对应创建的审核流程 -->
                  <div v-if="sub.review?.nodes?.length" class="sub-review-track">
                    <span class="sub-lbl">本轮审核签署流转:</span>
                    <div class="nodes-stepper-track">
                      <div
                        v-for="(rNode, rIdx) in sub.review.nodes"
                        :key="rIdx"
                        class="node-step-card"
                        :class="{ 'node-passed': rNode.status === 'pass' }"
                      >
                        <div class="node-icon-indicator">
                          <DemoIcon :name="rNode.status === 'pass' ? 'check' : 'clock'" :size="12" />
                        </div>
                        <div class="node-main">
                          <div class="node-title-row">
                            <strong>{{ rNode.name }}</strong>
                            <span class="node-user">{{ rNode.assignedName }}</span>
                            <span class="tag" :class="getStatusTagClass(rNode.status)">
                              {{ labels[rNode.status] || rNode.status }}
                            </span>
                          </div>
                          <p v-if="rNode.opinion" class="node-opinion">{{ rNode.opinion }}</p>
                          <time v-if="rNode.reviewedAt" class="mono node-time">{{ formatReadableDateTime(rNode.reviewedAt) }}</time>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </article>
          </div>
        </section>
      </div>
    </template>
  </main>
</template>

<style scoped>
.lifecycle-container {
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: min(100%, 1160px);
  margin: 0 auto;
  padding: 16px 20px 48px;
  box-sizing: border-box;
}

.lifecycle-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 24px;
  flex-wrap: wrap;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  flex-shrink: 0;
  border-radius: 12px;
  background: var(--accent-soft);
  color: var(--accent);
}

.header-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.header-title-row h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 800;
}

.header-left p {
  margin: 4px 0 0;
  color: var(--text-3);
  font-size: 12px;
}

.error-alert {
  display: flex;
  align-items: center;
  gap: 12px;
  border-color: var(--danger);
  background: rgb(248 113 113 / 8%);
  color: var(--danger);
}

.alert-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.loading-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 48px 24px;
  text-align: center;
  color: var(--text-3);
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* 资产概览卡片 */
.asset-root-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 20px 24px;
  flex-wrap: wrap;
  background: linear-gradient(135deg, var(--panel), var(--panel-2));
}

.asset-main {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.asset-no-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.asset-no {
  font-size: 18px;
  font-weight: 800;
  color: var(--accent);
}

.version-pill {
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--accent);
  color: var(--accent-ink);
  font-size: 12px;
  font-weight: 700;
}

.asset-name {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--text-1);
}

.asset-stats-grid {
  display: flex;
  gap: 20px;
}

.stat-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 8px 16px;
  border-radius: 10px;
  background: var(--panel);
  border: 1px solid var(--line);
}

.stat-lbl {
  font-size: 11px;
  color: var(--text-3);
}

.stat-val {
  font-size: 18px;
  font-weight: 800;
  color: var(--text-1);
}

.stat-val small {
  font-size: 11px;
  font-weight: 400;
  color: var(--text-3);
}

/* 时间线大板块 */
.timeline-threads {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.thread-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.thread-head {
  display: flex;
  align-items: center;
  gap: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--line);
}

.thread-badge-icon {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  flex-shrink: 0;
}

.release-icon {
  background: rgb(52 211 153 / 12%);
  color: var(--ok);
}

.review-icon {
  background: rgb(96 165 250 / 12%);
  color: var(--info);
}

.change-icon {
  background: var(--accent-soft);
  color: var(--accent);
}

.thread-title-wrap {
  flex: 1;
}

.thread-title-wrap h4 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  color: var(--text-1);
}

.thread-title-wrap p {
  margin: 2px 0 0;
  font-size: 11.5px;
  color: var(--text-3);
}

.empty-branch-msg {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 24px;
  color: var(--text-3);
  font-size: 12px;
}

/* 发布卡片流 */
.releases-flow,
.reviews-list,
.changes-timeline {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.release-snapshot-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: var(--panel-2);
}

.snapshot-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.snapshot-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.version-tag {
  font-size: 13px;
  font-weight: 800;
  color: var(--accent);
}

.source-tag {
  font-size: 11px;
}

.snapshot-spec-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--panel);
}

.spec-col {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.spec-col .lbl {
  font-size: 10.5px;
  color: var(--text-3);
}

.spec-col .val {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-1);
}

.signers-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}

.signers-lbl {
  color: var(--text-3);
  font-size: 11.5px;
  white-space: nowrap;
}

.signers-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.signer-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--panel);
  border: 1px solid var(--line);
  font-size: 11px;
}

.signer-chip strong {
  color: var(--text-2);
}

.files-archive-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.files-lbl,
.sub-lbl {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--text-2);
}

.files-cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 8px;
}

.file-version-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--panel);
  border: 1px solid var(--line);
}

.file-name {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.download-file-btn {
  flex-shrink: 0;
}

.bom-details,
.modern-details-card {
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--panel);
  overflow: hidden;
}

.bom-details summary,
.modern-details-card summary {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  cursor: pointer;
  background: var(--panel-2);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
  user-select: none;
}

.table-scroll-wrap {
  overflow-x: auto;
}

.bom-table {
  width: 100%;
}

/* 审核流程卡片 */
.review-item-card {
  background: var(--panel-2);
}

.review-top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.review-status-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.nodes-stepper-track {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.node-step-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--panel);
  border: 1px solid var(--line);
}

.node-icon-indicator {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--panel-2);
  color: var(--text-3);
  margin-top: 2px;
  flex-shrink: 0;
}

.node-step-card.node-passed .node-icon-indicator {
  background: rgb(52 211 153 / 15%);
  color: var(--ok);
}

.node-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.node-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12.5px;
}

.node-user {
  color: var(--text-3);
  font-size: 11.5px;
}

.node-opinion {
  margin: 0;
  font-size: 12px;
  color: var(--text-2);
  line-height: 1.4;
}

.node-time {
  font-size: 11px;
  color: var(--text-3);
}

/* 变更工单大卡片 */
.change-order-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  background: var(--panel-2);
}

.order-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.order-lead-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.order-no-pill {
  padding: 2px 8px;
  border-radius: 6px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 12px;
  font-weight: 700;
}

.order-info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--panel);
}

.info-block .lbl {
  display: block;
  font-size: 10.5px;
  color: var(--text-3);
  margin-bottom: 3px;
}

.info-block .val {
  margin: 0;
  font-size: 12.5px;
  color: var(--text-1);
  line-height: 1.4;
}

.details-body {
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.embed-body {
  padding: 8px;
  background: var(--panel);
}

.action-timeline-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}

.action-timeline-item .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent);
}

.action-timeline-item .actor {
  color: var(--text-1);
}

.action-timeline-item .opinion {
  color: var(--text-2);
}

.submissions-flow {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 4px;
}

.submissions-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--text-1);
}

.sub-round-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: var(--panel);
  border-left: 3px solid var(--accent);
}

.sub-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.round-badge {
  font-size: 12px;
  font-weight: 700;
  color: var(--accent);
}

.actual-changes-text {
  margin: 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--text-1);
}

.prop-diffs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.diff-tag {
  padding: 2px 8px;
  border-radius: 4px;
  background: var(--panel-2);
  font-size: 11px;
  color: var(--text-2);
}

.file-versions-pair-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 4px 0;
}

.file-pair-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 10px;
  border-radius: 6px;
  background: var(--panel-2);
  flex-wrap: wrap;
}

.pair-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--text-1);
}

.pair-actions {
  display: flex;
  gap: 8px;
}

.sub-review-track {
  margin-top: 6px;
}

@media (max-width: 768px) {
  .snapshot-spec-grid,
  .order-info-grid {
    grid-template-columns: 1fr;
  }
  .asset-root-card {
    flex-direction: column;
    align-items: flex-start;
  }
  .asset-stats-grid {
    width: 100%;
    justify-content: space-between;
  }
}
</style>
