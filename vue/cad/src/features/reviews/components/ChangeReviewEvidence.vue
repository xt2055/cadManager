<script setup lang="ts">
import { ref, watch } from 'vue'
import { lifecycleApi, downloadEvidence, type LifecycleTree, type LifecycleSubmission } from '@/services/lifecycle.service'
import EvidenceDocuments from '@/features/drawings/components/EvidenceDocuments.vue'
const props = defineProps<{ drawingNo: string; submissionId: string }>()
const emit = defineEmits<{ ready: [ready:boolean] }>()
const submission = ref<LifecycleSubmission | null>(null), tree = ref<LifecycleTree | null>(null), changeId = ref(''), reason = ref(''), error = ref('')
let generation = 0
async function load() { const seq=++generation;emit('ready',false);error.value='';submission.value=null;try {
 const result=await lifecycleApi<LifecycleTree>(`/lifecycle-tree?drawingNo=${encodeURIComponent(props.drawingNo)}`)
 if(seq!==generation)return
 const change=result.changes.find(c=>c.submissions.some(s=>s.id===props.submissionId))
 const snapshot=change?.submissions.find(s=>s.id===props.submissionId)
 if(!change||!snapshot)throw new Error('未找到本轮冻结提交，暂不能签署，请刷新。')
 tree.value=result;submission.value=snapshot;changeId.value=change.id;reason.value=change.reason;emit('ready',true)
 }catch(e){if(seq===generation)error.value=(e as Error).message} }
watch(()=>[props.drawingNo,props.submissionId],load,{immediate:true})
async function download(id:string|undefined,name:string){if(!id)return;try{await downloadEvidence(`/lifecycle-versions/${id}`,name)}catch(e){error.value=(e as Error).message}}
</script>
<template><section class="change-evidence card"><h3>本次变更审核 · 冻结提交</h3><p>请查阅下列本轮文件后签署。图纸库中的正式在用文件在全部审核通过前保持原版本。</p><p v-if="error" role="alert">{{ error }} <button class="btn" @click="load">重新读取</button></p><template v-if="submission"><p>第 {{ submission.round }} 轮 · {{ new Date(submission.createdAt).toLocaleString() }}</p><p><b>原因：</b>{{ reason }}</p><p><b>实际修改：</b>{{ submission.actualChanges }}</p><p v-for="(value,key) in submission.proposedAttributes" :key="key">{{ ({name:'名称',material:'材料',vendor:'供应商'} as Record<string,string>)[key] || key }}：{{ value }}</p><div v-for="file in submission.files" :key="file.attachmentId" class="file"><strong>{{ file.name }}</strong><button class="btn" :disabled="!file.baseVersionId" @click="download(file.baseVersionId,file.name)">变更前原件</button><button class="btn primary" :disabled="!file.submittedVersionId" @click="download(file.submittedVersionId,file.name)">下载本轮审核文件</button></div><EvidenceDocuments v-if="tree" :drawing-id="tree.id" :change-request-id="changeId" :submission-id="submission.id" read-only /></template></section></template>
<style scoped>.change-evidence{padding:18px;margin:14px 0;border:1px solid #a4bdda}.file{display:flex;align-items:center;gap:10px;flex-wrap:wrap;padding:10px 0}p{white-space:pre-wrap}h3{margin-bottom:8px}</style>
