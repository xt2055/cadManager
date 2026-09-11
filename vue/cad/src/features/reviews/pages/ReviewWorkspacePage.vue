<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import ReviewWorkspacePanel from '@/features/reviews/components/ReviewWorkspacePanel.vue'

defineOptions({ name: 'ReviewWorkspacePage' })

const route = useRoute()
const drawingNo = computed(() => String(route.params.drawingNo ?? '').trim())
</script>

<template>
  <ReviewWorkspacePanel :drawing-no="drawingNo" />
</template>
