<script setup lang="ts">
import { computed, ref } from 'vue'

import DemoIcon from '@/components/common/DemoIcon.vue'
import { useUiStore } from '@/stores/ui.store'

defineOptions({
  name: 'DemoModal',
})

const uiStore = useUiStore()
const formNo = ref('')
const formName = ref('')
const formReason = ref('')
const selectedVersion = ref('')
const selectedSync = ref('copy')

const modal = computed(() => uiStore.modal)

function close() {
  uiStore.closeModal()
}

function submit() {
  const current = modal.value
  if (!current) return

  if (current.type === 'create-drawing') {
    uiStore.toast(`图纸「${formNo.value || '未填写图号'}」已创建（草稿）· 可先搭结构、后补传文件`)
  } else if (current.type === 'upload-version') {
    uiStore.toast('新版本已生成 · 历史版本原样保留 · 已进入转换队列')
  } else if (current.type === 'borrow-drawing') {
    uiStore.toast('借用关系已建立 · 默认不同步原图 · 已记录借用人/时间/版本')
  } else if (current.type === 'revert') {
    uiStore.toast(`已回退：以 ${selectedVersion.value || '目标版本'} 内容生成新版本 · 历史版本原样保留 · 全程留痕`)
  } else if (current.type === 'sync-original') {
    uiStore.toast('已同步：原图生成新版本 v2.3 · 3 个引用项目已通知 · 已进入审核')
  } else if (current.type === 'exit') {
    uiStore.toast('窗口关闭请求已提交', 'info')
  } else if (current.type === 'add-user') {
    uiStore.toast('账号已分配 · 初始密码已通过内部渠道发送')
  } else if (current.type === 'edit-flow') {
    uiStore.toast('审核流程已更新 · 变更已记录')
  }

  close()
}
</script>

<template>
  <div v-if="modal" id="modalWrap" class="show">
    <button class="mask" type="button" aria-label="关闭弹窗" @click="close"></button>
    <section class="modal">
      <header class="modal-head">
        <h3>{{ modal.title }}</h3>
        <button class="icon-btn" type="button" @click="close"><DemoIcon name="x" /></button>
      </header>

      <div class="modal-body">
        <template v-if="modal.type === 'create-drawing'">
          <div class="note"><DemoIcon name="info" :size="14" /><div>可先创建图纸记录并搭建结构，<b>不要求立即上传图纸文件</b>，稍后在详情页任意子页补传。</div></div>
          <div class="modal-form-grid">
            <div class="field"><label for="form-no">图纸图号 *</label><input id="form-no" v-model="formNo" class="inp" placeholder="请输入图纸图号" /></div>
            <div class="field"><label for="form-name">图纸名称 *</label><input id="form-name" v-model="formName" class="inp" placeholder="如 二级圆柱齿轮减速器" /></div>
            <div class="field"><label>类型</label><select class="inp"><option>总图</option><option>零件图</option></select></div>
            <div class="field"><label>所属总图</label><select class="inp"><option>无（作为总图）</option></select></div>
            <div class="field"><label>厂商</label><input class="inp" placeholder="如 华辰重工" /></div>
            <div class="field"><label>零件材料</label><input class="inp" placeholder="如 HT200 / 45钢" /></div>
          </div>
          <div class="field field-last"><label>签署人员</label><div class="signature-grid"><select v-for="role in ['设计', '标准', '校对', '工艺', '批准']" :key="role" class="inp"><option>{{ role }}·待定</option><option>选择人员</option></select></div></div>
        </template>

        <template v-else-if="modal.type === 'upload-version'">
          <button class="dropzone" type="button" @click="uiStore.toast('文件选择功能待接入', 'info')"><DemoIcon name="file-up" :size="26" /><b>点击选择或拖入 CAD 文件</b><span>支持 .exb / .dwg / .dxf / .pdf · 单文件 ≤ 100 MB · 上传后自动转换矢量预览</span></button>
          <div class="field modal-top-field"><label for="version-reason">版本说明 *</label><textarea id="version-reason" v-model="formReason" class="inp" placeholder="如：按审核意见调整壁厚 8 → 10，同步更新明细栏"></textarea></div>
          <div class="field field-last"><label>借用同步策略（该图为借用件时生效）</label><label class="radio-row" :class="{ on: selectedSync === 'copy' }"><input v-model="selectedSync" type="radio" value="copy" /><span class="rt"><b>仅更新当前项目副本（默认）</b><span>不影响原借用零件图与其他引用项目，保留借用来源可追溯。</span></span></label><label class="radio-row" :class="{ on: selectedSync === 'origin' }"><input v-model="selectedSync" type="radio" value="origin" /><span class="rt"><b>修改并同步至原借用零件图</b><span>原图生成新版本，所有引用项目受影响，需重新走审核流程。</span></span></label></div>
        </template>

        <template v-else-if="modal.type === 'borrow-drawing'">
          <div class="field"><label>借用到项目</label><select class="inp"><option>请选择项目</option></select></div>
          <div class="field field-last"><label>同步策略（关键单选）</label><label class="radio-row" :class="{ on: selectedSync === 'copy' }"><input v-model="selectedSync" type="radio" value="copy" /><span class="rt"><b>仅项目副本，不同步原图（推荐）</b><span>当前项目独立演进；保留借用来源与版本快照，随时可追溯。</span></span></label><label class="radio-row" :class="{ on: selectedSync === 'origin' }"><input v-model="selectedSync" type="radio" value="origin" /><span class="rt"><b>修改后同步至原借用零件图</b><span>原图生成新版本并通知全部引用项目，需重新审核。</span></span></label></div>
        </template>

        <template v-else-if="modal.type === 'revert'">
          <div class="field"><label>回退至版本</label><select v-model="selectedVersion" class="inp"><option value="">请选择版本</option></select></div>
          <div class="field"><label for="revert-reason">回退原因</label><textarea id="revert-reason" v-model="formReason" class="inp" placeholder="请输入回退原因"></textarea></div>
          <div class="note"><DemoIcon name="shield-check" :size="14" /><div>回退将以目标版本内容生成<b>新版本</b>；历史版本<b>原样保留</b>，操作留痕，可再次回退撤销。</div></div>
        </template>

        <template v-else-if="modal.type === 'sync-original'">
          <div class="note"><DemoIcon name="alert-triangle" :size="14" /><div>同步将以<b>新版本</b>写入原图（不覆盖任何历史版本），以下引用项目将收到变更通知：</div></div>
          <div class="sync-list"><div class="empty"><DemoIcon name="folder" :size="28" /><div class="t">暂无引用项目</div></div></div>
          <div class="note"><DemoIcon name="stamp" :size="14" /><div>同步后原图需<b>重新走审核流程</b>方可发布。</div></div>
        </template>

        <template v-else-if="modal.type === 'exit'">
          <div class="exit-confirm"><DemoIcon name="power" :size="36" /><p>确定要关闭窗口吗？<br />所有变更已实时同步至服务器。</p></div>
        </template>

        <template v-else-if="modal.type === 'add-user'">
          <div class="note"><DemoIcon name="user-plus" :size="14" /><div>系统不开放自行注册，账号仅能由管理员在此分配。</div></div>
          <div class="modal-form-grid"><div class="field"><label>登录账号 *</label><input class="inp" placeholder="如 zhang" /></div><div class="field"><label>姓名 *</label><input class="inp" placeholder="如 张工" /></div><div class="field"><label>角色</label><select class="inp"><option>普通用户</option><option>设计人员</option><option>审核人员</option><option>管理员</option></select></div><div class="field"><label>所属组</label><select class="inp"><option>设计组</option><option>工艺组</option><option>标准化组</option></select></div></div>
          <div class="field field-last"><label>初始密码</label><input class="inp" value="Td@2025!" readonly /></div>
        </template>

        <template v-else-if="modal.type === 'edit-flow'">
          <div class="field"><label>流程名称</label><input class="inp" placeholder="请输入流程名称" /></div>
          <div class="field field-last"><label>审核节点（无序 · 全部必需）</label><div class="flow-edit-row"><input class="inp" placeholder="节点名称" /><select class="inp"><option>选择审核人员</option></select><input type="checkbox" checked /><button class="icon-btn" type="button" @click="uiStore.toast('节点已移除 · 历史审核记录不受影响', 'warn')"><DemoIcon name="trash-2" :size="14" /></button></div><button class="btn sm" type="button" @click="uiStore.toast('请配置新审核节点与审核人员', 'info')"><DemoIcon name="plus" :size="14" />添加节点</button></div><div class="note"><DemoIcon name="info" :size="14" /><div>无序审核：节点可并行处理、不分先后；删除节点不影响历史审核记录。</div></div>
        </template>
      </div>

      <footer class="modal-foot">
        <button class="btn" type="button" @click="close">取消</button>
        <button class="btn primary" type="button" @click="submit"><DemoIcon name="check" :size="14" />{{ modal.type === 'exit' ? '退出' : modal.type === 'create-drawing' ? '创建图纸' : modal.type === 'upload-version' ? '生成新版本' : modal.type === 'borrow-drawing' ? '建立借用' : modal.type === 'revert' ? '执行回退' : modal.type === 'sync-original' ? '确认同步' : modal.type === 'add-user' ? '创建账号' : '保存流程' }}</button>
      </footer>
    </section>
  </div>
</template>
