<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import EvidenceDocuments from '../components/EvidenceDocuments.vue'
import { useDrawingStore } from '@/stores/drawing.store'
const route = useRoute()
const drawings = useDrawingStore()
const drawingId = computed(() => drawings.getDrawing(String(route.params.drawingId || ''))?.id || '')
</script>
<template><EvidenceDocuments v-if="drawingId" :key="drawingId" :drawing-id="drawingId" /><p v-else>请从所属总图打开资料档案。</p></template>
