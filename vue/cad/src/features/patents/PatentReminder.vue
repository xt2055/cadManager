<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { lifecycleApi, patentAlerts, type PatentRecord } from '@/services/lifecycle.service'
import DemoIcon from '@/components/common/DemoIcon.vue'
import { useAuthStore } from '@/stores/auth.store'
const auth = useAuthStore()
const records = ref<PatentRecord[]>([]), failed = ref(false)
const isAdmin = computed(() => auth.hasRole('admin'))
const count = computed(() => records.value.filter(p => patentAlerts(p).length).length)
let timer: ReturnType<typeof setInterval> | undefined
let active = true, fetching = false
async function refresh() { if (!isAdmin.value || fetching) return; fetching = true; try { const result = await lifecycleApi<PatentRecord[]>('/patents'); if (active) { records.value = result; failed.value = false } } catch { if (active) failed.value = true } finally { fetching = false } }
function onVisible() { if (document.visibilityState === 'visible') void refresh() }
onMounted(() => { if (!isAdmin.value) return; void refresh(); timer = setInterval(refresh, 5 * 60_000); document.addEventListener('visibilitychange', onVisible) })
onUnmounted(() => { active = false; if (timer) clearInterval(timer); document.removeEventListener('visibilitychange', onVisible) })
</script>
<template>
  <RouterLink v-if="isAdmin && (count || failed)" to="/patents" class="patent-reminder" role="status">
    <span class="pr-icon"><DemoIcon :name="failed ? 'alert-circle' : 'bell'" :size="14" /></span>
    <span class="pr-text">{{ failed ? '专利提醒暂未更新' : `${count} 项专利需要关注` }}</span>
    <DemoIcon class="pr-arrow" name="chevron-right" :size="14" />
  </RouterLink>
</template>
<style scoped>
.patent-reminder {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 10px;
  padding: 11px 13px;
  border: 1px solid rgb(251 191 36 / 40%);
  border-radius: 12px;
  background: rgb(251 191 36 / 10%);
  color: var(--warn);
  font-size: 12.5px;
  text-decoration: none;
  transition: border-color 0.25s, transform 0.25s, background 0.25s;
}

.patent-reminder:hover {
  border-color: var(--warn);
  background: rgb(251 191 36 / 16%);
  transform: translateX(2px);
}

.pr-icon {
  display: grid;
  width: 24px;
  height: 24px;
  flex: none;
  place-items: center;
  border-radius: 8px;
  background: rgb(251 191 36 / 18%);
}

.pr-text {
  flex: 1;
  font-weight: 600;
}

.pr-arrow {
  flex: none;
  opacity: 0.7;
}

@media (prefers-reduced-motion: reduce) {
  .patent-reminder {
    transition: none;
  }
}
</style>
