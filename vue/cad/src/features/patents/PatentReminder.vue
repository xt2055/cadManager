<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { lifecycleApi, patentAlerts, type PatentRecord } from '@/services/lifecycle.service'
import { useAuthStore } from '@/stores/auth.store'
const auth = useAuthStore()
const records = ref<PatentRecord[]>([]), failed = ref(false)
const count = computed(() => records.value.filter(p => (auth.hasRole('admin') || p.responsibleId===auth.currentUser?.id) && patentAlerts(p).length).length)
let timer: ReturnType<typeof setInterval> | undefined
let active = true, fetching = false
async function refresh() { if(fetching)return;fetching=true;try {const result=await lifecycleApi<PatentRecord[]>('/patents');if(active){records.value=result;failed.value=false}} catch {if(active)failed.value=true} finally{fetching=false} }
function onVisible(){if(document.visibilityState==='visible')void refresh()}
onMounted(()=>{void refresh();timer=setInterval(refresh,5*60_000);document.addEventListener('visibilitychange',onVisible)})
onUnmounted(()=>{active=false;if(timer)clearInterval(timer);document.removeEventListener('visibilitychange',onVisible)})
</script>
<template><RouterLink v-if="count || failed" to="/patents" class="patent-reminder" role="status">{{ failed?'专利提醒暂未更新':`${count} 项专利需要关注` }}</RouterLink></template>
<style scoped>.patent-reminder{display:block;margin:10px;padding:10px;border:1px solid #dfbd86;background:#fff7e7;border-radius:8px;color:#854807;font-size:12px;text-decoration:none}</style>
