import type { DrawingAttribute } from '@/types/domain.types'

/** 属性模块只提供属性规则与展示映射，不持有 Store 或页面状态。 */
export class AttributeService {
  sort(attributes: readonly DrawingAttribute[]): DrawingAttribute[] {
    return [...attributes]
      .filter((attribute) => attribute.enabled)
      .sort((left, right) => left.sortOrder - right.sortOrder || left.name.localeCompare(right.name, 'zh-CN'))
  }

  name(attributes: readonly DrawingAttribute[], id: string): string {
    return attributes.find((attribute) => attribute.id === id)?.name ?? ''
  }

  fieldName(attributes: readonly DrawingAttribute[], attributeId: string, fieldId?: string): string {
    if (!fieldId) return ''
    return attributes.find((attribute) => attribute.id === attributeId)?.fields.find((field) => field.id === fieldId)?.name ?? ''
  }

  validate(attributes: readonly DrawingAttribute[], values: Record<string, string> = {}): string[] {
    return this.sort(attributes).flatMap((attribute) => {
      const value = values[attribute.id] ?? ''
      if (attribute.required && !value) return [`请填写属性「${attribute.name}」`]
      if (value && !attribute.fields.some((field) => field.enabled && field.id === value)) {
        return [`属性「${attribute.name}」的字段选项无效`]
      }
      return []
    })
  }
}
