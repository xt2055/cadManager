import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { attributeQueryService, attributeService } from '@/app/container'
import type { DrawingAttribute } from '@/types/domain.types'

/** 属性筛选 Read Model；规则计算委托 AttributeService。 */
export const useAttributeStore = defineStore('attribute', () => {
  const attributes = ref<DrawingAttribute[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sortedAttributes = computed(() => attributeService.sort(attributes.value))

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      attributes.value = await attributeQueryService.list()
    } catch (loadError: unknown) {
      error.value = loadError instanceof Error ? loadError.message : String(loadError)
      throw loadError
    } finally {
      loading.value = false
    }
  }

  function fieldName(attributeId: string, fieldId?: string): string {
    return attributeService.fieldName(attributes.value, attributeId, fieldId)
  }

  function validate(values: Record<string, string>): string[] {
    return attributeService.validate(attributes.value, values)
  }

  function invalidate() {
    attributes.value = []
    error.value = null
  }

  return { attributes, sortedAttributes, loading, error, load, fieldName, validate, invalidate }
})
