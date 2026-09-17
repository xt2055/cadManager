<script setup lang="ts">
import DemoIcon from '@/components/common/DemoIcon.vue'

defineOptions({ name: 'BorrowPartModal' })

/**
 * 跨工程项目借用零件弹窗：只做展示与意图收集。
 * 不引入 store / router / 业务 composable——检索关键词、模式、选中项与借用原因都经 update:* 与
 * select* 事件回抛给持有状态的 useDrawingBorrow；「项目零件数」「零件归属项目名」等派生串由父页面投影后传入。
 */
interface BorrowProjectCard {
  no: string
  name: string
  /** 该项目下可借用零件数（原 getProjectPartCount 的结果）。 */
  partCount: number
}

interface BorrowPartCard {
  no: string
  name: string
  parentNo: string
  partType: string
  material: string
  spec: string
  weight: number
  qty: number
  surfaceTreatment: string
  /** 关联图纸文件名（对应原 files 里用到的那部分）。 */
  fileNames: string[]
  /** 所属项目名（原 getPartProjectName 的结果）。 */
  projectName: string
}

defineProps<{
  /** 选型模式：按工程项目三栏 / 全库全局穿透搜索。 */
  mode: 'by-project' | 'global-part'
  projectQuery: string
  partQuery: string
  projects: BorrowProjectCard[]
  parts: BorrowPartCard[]
  selectedProjectNo: string
  selectedProject: { no: string; name: string } | null
  selectedPartNo: string
  selectedPart: BorrowPartCard | null
  /** 借用备注说明 / 选型原因。 */
  reason: string
  /** 正在提交借用。 */
  submitting: boolean
}>()

const emit = defineEmits<{
  close: []
  confirm: []
  'update:mode': [mode: 'by-project' | 'global-part']
  'update:projectQuery': [value: string]
  'update:partQuery': [value: string]
  'update:reason': [value: string]
  /** 选中来源项目（父页面会清空已选零件）。 */
  selectProject: [no: string]
  /** 选中零件；全库模式下带上所属项目，父页面据此同步来源项目。 */
  selectPart: [no: string, parentNo?: string]
}>()
</script>

<template>
  <div class="modal-backdrop">
    <div class="modal card borrow-modal">
      <div class="modal-head">
        <div class="modal-title">
          <DemoIcon name="share-2" :size="18" />
          <span>跨工程项目借用零件与图纸</span>
        </div>
        <div class="modal-mode-tabs">
          <button
            class="mode-tab-btn"
            :class="{ active: mode === 'by-project' }"
            type="button"
            @click="emit('update:mode', 'by-project')"
          >
            <DemoIcon name="folder" :size="13" />按工程项目选型
          </button>
          <button
            class="mode-tab-btn"
            :class="{ active: mode === 'global-part' }"
            type="button"
            @click="emit('update:mode', 'global-part')"
          >
            <DemoIcon name="search" :size="13" />全库全局穿透搜索
          </button>
        </div>
        <button class="btn sm close-btn" type="button" @click="emit('close')">✕</button>
      </div>

      <div class="modal-body borrow-modal-body">
        <!-- 模式 1：按工程项目三栏分级导航与选型（专为几千个项目设计） -->
        <div v-if="mode === 'by-project'" class="borrow-three-grid">
          <!-- 栏 1：项目库快速检索与选择 -->
          <div class="borrow-panel-col">
            <div class="col-header">
              <span class="col-title"><DemoIcon name="folder" :size="13" />工程项目库 ({{ projects.length }})</span>
            </div>
            <div class="search-input-wrap compact">
              <DemoIcon name="search" :size="13" />
              <input
                :value="projectQuery" @input="emit('update:projectQuery', ($event.target as HTMLInputElement).value)"
                type="text"
                class="inp filter-inp"
                placeholder="搜索项目名称/图号/厂商..."
              />
            </div>
            <div class="scroll-select-list">
              <div
                v-for="proj in projects"
                :key="proj.no"
                class="project-item-card"
                :class="{ active: (selectedProjectNo || projects[0]?.no) === proj.no }"
                @click="emit('selectProject', proj.no)"
              >
                <div class="proj-card-title">{{ proj.name }}</div>
                <div class="proj-card-meta">
                  <span class="mono">{{ proj.no }}</span>
                  <span class="tag tag-no-dot plain tag-xs">{{ proj.partCount }} 个零件</span>
                </div>
              </div>
              <div v-if="!projects.length" class="empty-list-prompt">
                <DemoIcon name="search-x" :size="20" />
                <span>未找到匹配的项目</span>
              </div>
            </div>
          </div>

          <!-- 栏 2：当前项目下的零件列表 -->
          <div class="borrow-panel-col">
            <div class="col-header">
              <span class="col-title"><DemoIcon name="file" :size="13" />可选零件清单 ({{ parts.length }})</span>
              <span class="col-badge mono">{{ selectedProject?.no }}</span>
            </div>
            <div class="search-input-wrap compact">
              <DemoIcon name="filter" :size="13" />
              <input
                :value="partQuery" @input="emit('update:partQuery', ($event.target as HTMLInputElement).value)"
                type="text"
                class="inp filter-inp"
                placeholder="过滤零件图号/名称/材质..."
              />
            </div>
            <div class="scroll-select-list">
              <div
                v-for="part in parts"
                :key="part.no"
                class="part-candidate-card"
                :class="{ active: selectedPartNo === part.no }"
                @click="emit('selectPart', part.no)"
              >
                <div class="part-card-head">
                  <span class="part-name">{{ part.name }}</span>
                  <span class="tag tag-no-dot mono tag-xs">{{ part.no }}</span>
                </div>
                <div class="part-card-sub">
                  <span>材质: {{ part.material || '—' }}</span>
                  <span>数量: {{ part.qty || 1 }}</span>
                  <span class="has-file-badge" :class="{ ok: part.fileNames.length }">
                    {{ part.fileNames.length ? `${part.fileNames.length} 份图纸` : '无图纸' }}
                  </span>
                </div>
              </div>
              <div v-if="!parts.length" class="empty-list-prompt">
                <DemoIcon name="search-x" :size="20" />
                <span>该项目暂无匹配零件</span>
              </div>
            </div>
          </div>

          <!-- 栏 3：选中零件档案核对与借用理由 -->
          <div class="borrow-panel-col borrow-detail-col">
            <div class="col-header">
              <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
            </div>

            <div v-if="selectedPart" class="borrow-card-detail-content">
              <div class="preview-hero-card">
                <DemoIcon name="file-check-2" :size="22" />
                <div>
                  <h4>{{ selectedPart.name }}</h4>
                  <span class="mono text-accent">{{ selectedPart.no }}</span>
                </div>
              </div>

              <div class="preview-spec-grid">
                <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ selectedProject?.name }}</span></div>
                <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPart.partType }}</span></div>
                <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPart.material }} {{ selectedPart.spec }}</span></div>
                <div class="spec-row"><span class="k">单重/数量：</span><span class="v">{{ selectedPart.weight ? `${selectedPart.weight} kg` : '—' }} / {{ selectedPart.qty }} 件</span></div>
                <div class="spec-row"><span class="k">表面处理：</span><span class="v">{{ selectedPart.surfaceTreatment || '—' }}</span></div>
                <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPart?.fileNames[0] || '（无独立图纸文件）' }}</span></div>
              </div>

              <div class="field" style="margin-top: auto;">
                <label class="bold">借用备注说明 / 选型原因</label>
                <input
                  :value="reason" @input="emit('update:reason', ($event.target as HTMLInputElement).value)"
                  type="text"
                  class="inp"
                  placeholder="例如：复用成熟导向套设计，缩短加工周期"
                />
              </div>
            </div>

            <div v-else class="empty-preview-prompt">
              <DemoIcon name="mouse-pointer-click" :size="32" />
              <p>请点击中间列表选择需要借用的零件</p>
            </div>
          </div>
        </div>

        <!-- 模式 2：全库全局穿透搜索（直击数万图纸） -->
        <div v-else class="borrow-two-grid">
          <div class="borrow-panel-col">
            <div class="search-input-wrap">
              <DemoIcon name="search" :size="15" />
              <input
                :value="partQuery" @input="emit('update:partQuery', ($event.target as HTMLInputElement).value)"
                type="text"
                class="inp global-search-inp"
                placeholder="在企业全库中穿透搜索：输入图号如 04、活塞、HT200、或项目名称..."
                autofocus
              />
            </div>

            <div class="scroll-select-list global-list" style="margin-top: 10px;">
              <div
                v-for="part in parts"
                :key="part.no"
                class="global-part-card"
                :class="{ active: selectedPartNo === part.no }"
                @click="emit('selectPart', part.no, part.parentNo)"
              >
                <div class="gp-top">
                  <span class="gp-name">{{ part.name }}</span>
                  <span class="mono gp-no">{{ part.no }}</span>
                  <span class="tag info tag-xs">{{ part.projectName }}</span>
                </div>
                <div class="gp-btm">
                  <span>材质: {{ part.material || '—' }}</span>
                  <span>数量: {{ part.qty || 1 }}</span>
                  <span>图纸: {{ part.fileNames[0] || '无文件' }}</span>
                </div>
              </div>
              <div v-if="!parts.length" class="empty-list-prompt">
                <DemoIcon name="search-x" :size="24" />
                <span>全库中未找到匹配的零件，请尝试更简短的关键词</span>
              </div>
            </div>
          </div>

          <!-- 右侧详情 -->
          <div class="borrow-panel-col borrow-detail-col">
            <div class="col-header">
              <span class="col-title"><DemoIcon name="check-square" :size="13" />借用档案核对</span>
            </div>

            <div v-if="selectedPart" class="borrow-card-detail-content">
              <div class="preview-hero-card">
                <DemoIcon name="file-check-2" :size="22" />
                <div>
                  <h4>{{ selectedPart.name }}</h4>
                  <span class="mono text-accent">{{ selectedPart.no }}</span>
                </div>
              </div>

              <div class="preview-spec-grid">
                <div class="spec-row"><span class="k">来源工程：</span><span class="v">{{ selectedPart?.projectName }}</span></div>
                <div class="spec-row"><span class="k">制造分类：</span><span class="v tag plain">{{ selectedPart.partType }}</span></div>
                <div class="spec-row"><span class="k">材质规格：</span><span class="v mono">{{ selectedPart.material }} {{ selectedPart.spec }}</span></div>
                <div class="spec-row"><span class="k">关联图纸：</span><span class="v mono text-accent">{{ selectedPart?.fileNames[0] || '（无文件）' }}</span></div>
              </div>

              <div class="field" style="margin-top: auto;">
                <label class="bold">借用备注说明</label>
                <input
                  :value="reason" @input="emit('update:reason', ($event.target as HTMLInputElement).value)"
                  type="text"
                  class="inp"
                  placeholder="输入借用说明..."
                />
              </div>
            </div>

            <div v-else class="empty-preview-prompt">
              <DemoIcon name="mouse-pointer-click" :size="32" />
              <p>请在搜索结果中点击选定要借用的零件</p>
            </div>
          </div>
        </div>

        <div class="note info-note" style="margin-top: 12px;">
          <DemoIcon name="shield-check" :size="14" />
          <div>系统将自动克隆图纸零件并挂载至当前工程，在借用记录台账中建立双向可追溯凭据，不污染源工程。</div>
        </div>
      </div>

      <div class="modal-foot">
        <button class="btn" type="button" :disabled="submitting" @click="emit('close')">取消</button>
        <button
          class="btn primary"
          type="button"
          :disabled="!selectedPartNo || submitting"
          @click="emit('confirm')"
        >
          <DemoIcon name="check" :size="14" />{{ submitting ? '借入中...' : '确认借入此零件' }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="../styles/modal-chrome.css"></style>

<style scoped>
/* 借用零件弹窗专属样式；外壳来自共享 modal-chrome。 */

/* 借用零件高阶选型体系弹窗 */
.borrow-modal {
  width: 960px;
  max-width: 96vw;
  height: 640px;
  max-height: 92vh;
  background: var(--panel);
  border: 1px solid var(--line-strong);
  border-radius: var(--radius-md, 8px);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.modal-mode-tabs {
  display: flex;
  gap: 6px;
  background: var(--panel-2);
  padding: 3px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.mode-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 4px;
  border: none;
  background: transparent;
  color: var(--text-3);
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.15s ease;
}

.mode-tab-btn:hover {
  color: var(--text-1);
}

.mode-tab-btn.active {
  background: var(--panel);
  color: var(--accent);
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
}

.borrow-modal-body {
  flex: 1;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

/* 模式 1：三栏式自适应布局 */
.borrow-three-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 260px 310px 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

/* 模式 2：双栏穿透搜索布局 */
.borrow-two-grid {
  flex: 1;
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

.borrow-panel-col {
  display: flex;
  flex-direction: column;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
  padding: 12px;
  min-height: 0;
  overflow: hidden;
}

.col-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.col-title {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  display: flex;
  align-items: center;
  gap: 6px;
}

.col-title svg {
  color: var(--accent);
}

.col-badge {
  font-size: 11px;
  color: var(--text-3);
  background: var(--panel);
  padding: 1px 5px;
  border-radius: 3px;
  border: 1px solid var(--line);
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.compact {
  margin-top: 0 !important;
  margin-bottom: 8px;
}

.filter-inp {
  font-size: 12px !important;
  height: 30px !important;
  padding-left: 28px !important;
}

.global-search-inp {
  padding-left: 36px !important;
  height: 38px !important;
  font-size: 13px !important;
}

.scroll-select-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 2px;
}

.project-item-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.project-item-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.project-item-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.proj-card-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--text-1);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.proj-card-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.part-candidate-card {
  padding: 8px 10px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 5px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.part-candidate-card:hover {
  background: var(--hover);
  border-color: var(--accent-light, var(--line-strong));
}

.part-candidate-card.active {
  background: var(--panel);
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.part-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.part-name {
  font-size: 12.5px;
  font-weight: 700;
  color: var(--text-1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.part-card-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-3);
}

.has-file-badge {
  color: var(--text-3);
}

.has-file-badge.ok {
  color: var(--accent);
}

.global-part-card {
  padding: 10px 12px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.global-part-card:hover {
  background: var(--hover);
  border-color: var(--accent);
}

.global-part-card.active {
  border-color: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
}

.gp-top {
  display: flex;
  align-items: center;
  gap: 8px;
}

.gp-name {
  font-size: 13.5px;
  font-weight: 700;
  color: var(--text-1);
}

.gp-no {
  font-size: 12px;
  color: var(--text-2);
}

.gp-btm {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 11.5px;
  color: var(--text-3);
}

.borrow-detail-col {
  background: var(--panel);
  padding: 14px;
}

.borrow-card-detail-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  overflow-y: auto;
}

.preview-hero-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  background: var(--panel-2);
  border: 1px solid var(--line);
  border-radius: 6px;
}

.preview-hero-card svg {
  color: var(--accent);
}

.preview-hero-card h4 {
  margin: 0;
  font-size: 14.5px;
  font-weight: 700;
}

.preview-spec-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--panel-2);
  padding: 12px;
  border-radius: 6px;
  border: 1px solid var(--line);
}

.spec-row {
  display: flex;
  align-items: center;
  font-size: 12.5px;
  gap: 6px;
}

.spec-row .k {
  color: var(--text-3);
  width: 75px;
  flex-shrink: 0;
}

.spec-row .v {
  color: var(--text-1);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
}

.empty-list-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 32px 10px;
  color: var(--text-3);
  gap: 6px;
  font-size: 12px;
}

.empty-preview-prompt {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  flex: 1;
  color: var(--text-3);
  gap: 8px;
  font-size: 13px;
}

@media (max-width: 900px) {
  .borrow-three-grid {
    grid-template-columns: 1fr;
  }
  .borrow-two-grid {
    grid-template-columns: 1fr;
  }
}
</style>
