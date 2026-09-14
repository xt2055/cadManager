<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { lifecycleApi, patentAlerts, type PatentRecord, type EvidenceDocument } from '@/services/lifecycle.service'
import EvidenceDocuments from '@/features/drawings/components/EvidenceDocuments.vue'
import { useAuthStore } from '@/stores/auth.store'
import { useDrawingStore } from '@/stores/drawing.store'
import { changeRequestService, type ChangeUserOption } from '@/services/change-request.service'
const auth = useAuthStore()
const drawingStore = useDrawingStore(), users = ref<ChangeUserOption[]>([])
const patents = ref<PatentRecord[]>([]), selected = ref<PatentRecord | null>(null), search = ref(''), onlyAlerts = ref(false)
const busy = ref(false), loading = ref(false), error = ref(''), message = ref(''), editing = ref(false)
const empty = () => ({ number:'',title:'',patentType:'发明',jurisdiction:'中国',ownerName:'',responsibleId:auth.currentUser?.id || '',drawingId:'',feeDue:'',expiresOn:'',deadlineSource:'',reminderDays:90,notes:'',revision:0 })
const form = reactive(empty())
const payment = reactive({ receiptId:'',paidOn:'',amount:'',feeDue:'',deadlineSource:'' })
const receipts = ref<EvidenceDocument[]>([])
const events = ref<{ id:string; action:string; actor:string; createdAt:string; detail:Record<string,unknown> }[]>([])
const eventLabels: Record<string,string> = { create:'新增登记', update:'修改登记', payment:'缴费登记', reminder:'期限提醒' }
function eventLines(event: (typeof events.value)[number]) {
  if (event.action==='reminder') return [`${event.detail.kind==='fee'?'缴费截止':'权利到期'}：${event.detail.deadline}`, `触发时距登记期限 ${event.detail.days} 天`, `日期依据：${event.detail.source || '登记资料'}`]
  const next = event.detail.submitted as Record<string,unknown> | undefined
  const before = event.detail.before as Record<string,unknown> | null | undefined
  if (!next) return ['原记录未包含详细字段。']
  if (event.action === 'payment') return [
    `本期缴费截止：${before?.fee_due || '原记录未登记'}`,
    `实际缴费：${next.paidOn || ''}，金额 ${next.amount || ''}`,
    `缴费凭证：${receipts.value.find(d => d.id === next.receiptId)?.title || '已归档原件'}`,
    `下一缴费截止：${next.feeDue || ''}`,
    `期限依据：${next.deadlineSource || ''}`,
  ]
  const fields = [ ['number','number','专利编号'], ['title','title','名称'], ['patentType','patent_type','类型'], ['jurisdiction','jurisdiction','国家 / 地区'], ['ownerName','owner_name','权利人'], ['feeDue','fee_due','缴费期限'], ['expiresOn','expires_on','权利到期'], ['reminderDays','reminder_days','提前提醒天数'], ['deadlineSource','deadline_source','日期依据'], ['notes','notes','备注'] ] as const
  return fields.filter(([key,oldKey]) => event.action==='create' || String(next[key]??'')!==String(before?.[oldKey]??'')).map(([key,oldKey,label]) => event.action==='create' ? `${label}：${next[key] || '未登记'}` : `${label}：${before?.[oldKey] || '未登记'} → ${next[key] || '未登记'}`)
}
const canEdit = computed(() => !selected.value || selected.value.responsibleId === auth.currentUser?.id || auth.hasRole('admin'))
const visible = computed(() => patents.value.filter(p => `${p.number} ${p.title} ${p.ownerName}`.toLowerCase().includes(search.value.toLowerCase()) && (!onlyAlerts.value || patentAlerts(p).length)))
const alertCount = computed(() => patents.value.filter(p => patentAlerts(p).length).length)
async function load() { loading.value = true; error.value = ''; try { patents.value = await lifecycleApi<PatentRecord[]>('/patents'); if (selected.value) selected.value = patents.value.find(p => p.id === selected.value?.id) || null } catch(e) { error.value=(e as Error).message } finally { loading.value=false } }
let selectionGeneration = 0
async function select(p: PatentRecord) { selected.value=p;editing.value=false;events.value=[];receipts.value=[];error.value='';message.value='';Object.assign(payment,{receiptId:'',paidOn:'',amount:'',feeDue:'',deadlineSource:''});const seq=++selectionGeneration;try { const [docs,history]=await Promise.all([lifecycleApi<EvidenceDocument[]>(`/lifecycle-documents?patentId=${p.id}`),lifecycleApi<typeof events.value>(`/patents/${p.id}/events`)]);if(seq===selectionGeneration){receipts.value=docs;events.value=history} } catch(e){if(seq===selectionGeneration)error.value=(e as Error).message} }
function create() { ++selectionGeneration;selected.value=null;Object.assign(form,empty());editing.value=true;message.value='';error.value='' }
function edit() { const p=selected.value;if(!p)return;Object.assign(form,{number:p.number,title:p.title,patentType:p.patentType,jurisdiction:p.jurisdiction,ownerName:p.ownerName,responsibleId:p.responsibleId,drawingId:p.drawingId||'',feeDue:p.feeDue||'',expiresOn:p.expiresOn||'',deadlineSource:p.deadlineSource,reminderDays:p.reminderDays,notes:p.notes,revision:p.revision});editing.value=true }
async function save() { if(busy.value)return;busy.value=true;error.value='';try { const result=await lifecycleApi<{id:string}>(`/patents${selected.value?`/${selected.value.id}`:''}`,{method:selected.value?'PUT':'POST',body:JSON.stringify(form)});await load();const p=patents.value.find(p=>p.id===result.id);if(p)await select(p);message.value='专利记录已保存，提醒日期已更新。' }catch(e){error.value=(e as Error).message}finally{busy.value=false} }
async function refreshReceipts() { if(!selected.value)return;try{receipts.value=await lifecycleApi<EvidenceDocument[]>(`/lifecycle-documents?patentId=${selected.value.id}`)}catch(e){error.value=(e as Error).message} }
async function recordPayment() { if(!selected.value||busy.value)return;busy.value=true;error.value='';try{await lifecycleApi(`/patents/${selected.value.id}/payment`,{method:'POST',body:JSON.stringify({...payment,revision:selected.value.revision})});await load();if(selected.value)await select(selected.value);message.value='缴费凭证及本期记录已保存，下一期缴费提醒已启用。'}catch(e){error.value=(e as Error).message}finally{busy.value=false} }
onMounted(() => { void load(); void drawingStore.load().catch(() => { error.value='关联图号列表暂不可用，请刷新' }); if(auth.hasRole('admin')) void changeRequestService.listUsers().then(result=>{users.value=result}).catch(()=>{error.value='负责人列表暂不可用，请刷新'}) })
</script>
<template>
 <div class="page patent-page"><header class="patent-head"><div><h2>专利管理</h2><p>缴费、期限、证明材料及办理记录统一归档。</p></div><button class="btn" :disabled="busy || loading" @click="load">刷新</button><button class="btn primary" :disabled="busy" @click="create">新增专利</button></header>
 <p v-if="error" role="alert" class="patent-error">{{ error }}</p><p v-if="message" role="status">{{ message }}</p>
 <div class="patent-head"><input v-model="search" placeholder="搜索专利编号、名称、权利人" aria-label="搜索专利" /><label><input v-model="onlyAlerts" type="checkbox" /> 仅需关注（{{ alertCount }}）</label><span>{{ patents.length }} 项专利</span></div>
 <div class="patent-layout"><aside><p v-if="loading">正在加载…</p><p v-else-if="!visible.length">暂无符合条件的专利。</p><button v-for="p in visible" :key="p.id" class="patent-row" :class="{active:selected?.id===p.id}" :disabled="busy" @click="select(p)"><strong>{{ p.title }}</strong><small>{{ p.number }} · {{ p.responsibleName }}</small><span v-for="alert in patentAlerts(p)" :key="alert" class="patent-alert">{{ alert }}</span><small v-if="!patentAlerts(p).length">期限正常 · 下次缴费 {{ p.feeDue }}</small></button></aside>
 <main><form v-if="editing" class="patent-form" @submit.prevent="save"><h3>{{ selected ? '修改专利登记' : '新增专利' }}</h3>
 <label>专利 / 申请编号<input v-model="form.number" required maxlength="100" /></label><label>专利名称<input v-model="form.title" required maxlength="300" /></label><label>类型<select v-model="form.patentType"><option>发明</option><option>实用新型</option><option>外观设计</option><option>其他</option></select></label><label>国家 / 地区<input v-model="form.jurisdiction" required /></label><label>权利人<input v-model="form.ownerName" /></label><label>提前提醒天数<input v-model.number="form.reminderDays" type="number" min="1" max="365" required /></label><label>本期缴费截止日期<input v-model="form.feeDue" type="date" /></label><label>权利到期日期<input v-model="form.expiresOn" type="date" /></label><label class="wide">日期依据<input v-model="form.deadlineSource" required placeholder="官方通知书、登记簿或代理机构确认文件及日期" /></label><label class="wide">备注<textarea v-model="form.notes" rows="3" /></label>
 <label>关联图号<select v-model="form.drawingId"><option value="">独立专利，不关联图号</option><option v-for="d in drawingStore.drawings" :key="d.id || d.no" :value="d.id">{{ d.no }} · {{ d.name }}</option></select></label><label v-if="auth.hasRole('admin')">负责人<select v-model="form.responsibleId" required><option :value="auth.currentUser?.id">我本人</option><option v-for="u in users.filter(u=>u.id!==auth.currentUser?.id)" :key="u.id" :value="u.id">{{ u.displayName }}</option></select></label>
 <p class="wide muted">缴费日期和权利到期日分别登记，以所持官方资料为准。缺少日期会显示资料待完善提醒；缴费不会延长已登记的权利期限。</p><div class="wide patent-head"><button class="btn primary" :disabled="busy">{{ busy?'正在保存…':'保存专利' }}</button><button class="btn" type="button" :disabled="busy" @click="editing=false">取消编辑</button></div></form>
 <template v-else-if="selected"><header class="patent-head"><h3>{{ selected.title }}</h3><button v-if="canEdit" class="btn" :disabled="busy" @click="edit">编辑登记</button></header><p>{{ selected.number }} · {{ selected.patentType }} · {{ selected.jurisdiction }}</p><p>权利人：{{ selected.ownerName || '未登记' }}　负责人：{{ selected.responsibleName }}</p><p>缴费截止：<b>{{ selected.feeDue || '待完善' }}</b>　权利到期：<b>{{ selected.expiresOn || '待完善' }}</b></p><p>日期依据：{{ selected.deadlineSource }}</p><p v-for="alert in patentAlerts(selected)" :key="alert" class="patent-alert">{{ alert }}</p><p>{{ selected.notes }}</p>
 <EvidenceDocuments :key="selected.id" :patent-id="selected.id" :read-only="!canEdit" />
 <details v-if="canEdit" class="payment"><summary>登记已缴费并安排下一期提醒</summary><form class="patent-form" @submit.prevent="recordPayment"><p class="wide">先在上方上传缴费凭证，再刷新凭证列表。这里登记办理结果，不会执行付款。</p><label class="wide">缴费凭证<select v-model="payment.receiptId" required><option value="">请选择归档原件</option><option v-for="d in receipts" :key="d.id" :value="d.id">{{ d.title }}</option></select></label><button type="button" class="btn" @click="refreshReceipts">刷新凭证列表</button><label>实际缴费日期<input v-model="payment.paidOn" type="date" required /></label><label>金额及币种<input v-model="payment.amount" required placeholder="例如：900.00 CNY" /></label><label>下一期缴费截止<input v-model="payment.feeDue" type="date" required /></label><label class="wide">下一期期限依据<input v-model="payment.deadlineSource" required /></label><button class="btn primary" :disabled="busy">保存缴费记录</button></form></details>
 <h3>办理历史</h3><p v-if="!events.length">暂无办理记录。</p><details v-for="event in events" :key="event.id" class="event"><summary>{{ new Date(event.createdAt).toLocaleString() }} · {{ event.actor }} · {{ eventLabels[event.action] || event.action }}</summary><p v-for="(line,index) in eventLines(event)" :key="index">{{ line }}</p></details>
 </template><p v-else class="muted">选择一项专利查看资料和办理历史，或新增专利登记。</p></main></div></div>
</template>
<style scoped>
.patent-page{padding:22px;overflow:auto}.patent-head{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin-bottom:16px}.patent-head>div:first-child{flex:1}.patent-layout{display:grid;grid-template-columns:minmax(240px,320px) minmax(0,1fr);gap:22px}.patent-row{display:flex;flex-direction:column;gap:8px;text-align:left;width:100%;padding:16px;margin-bottom:8px;background:var(--bg-card,#fff);border:1px solid var(--border,#ddd);border-radius:8px;cursor:pointer;color:inherit}.patent-row.active{border-color:#2975c8;background:var(--bg-soft,#eff6ff)}.patent-form{display:grid;grid-template-columns:1fr 1fr;gap:14px}.patent-form label{display:flex;flex-direction:column;gap:6px}.patent-form h3,.wide{grid-column:1/-1}input,select,textarea{padding:8px;border:1px solid var(--border,#ccc);border-radius:6px;font:inherit;min-width:0}.patent-alert{color:#a34e08;font-size:13px}.patent-error{color:#b42318}.muted,small{color:var(--text-muted,#64748b)}.payment,.event{padding:14px 0}summary{cursor:pointer}pre{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px}main{min-width:0}@media(max-width:850px){.patent-layout{grid-template-columns:1fr}.patent-form{grid-template-columns:1fr}}
</style>
