import type { DrawingAttribute, DrawingAttributeField } from '@/types/domain.types'

export interface AttributeCommandInput {
  name: string
  required: boolean
  enabled: boolean
  sortOrder: number
}

export interface AttributeFieldCommandInput {
  name: string
  enabled: boolean
  sortOrder: number
}

export interface AttributeCommandGateway {
  create(input: AttributeCommandInput): Promise<DrawingAttribute>
  update(id: string, input: AttributeCommandInput): Promise<DrawingAttribute>
  remove(id: string): Promise<void>
  addField(attributeId: string, input: AttributeFieldCommandInput): Promise<DrawingAttributeField>
  updateField(attributeId: string, fieldId: string, input: AttributeFieldCommandInput): Promise<DrawingAttributeField>
  removeField(attributeId: string, fieldId: string): Promise<void>
}

/** 属性配置的原子命令入口，页面不再提交整张属性表。 */
export class AttributeCommandService {
  public constructor(private readonly gateway: AttributeCommandGateway) {}

  create(input: AttributeCommandInput): Promise<DrawingAttribute> { return this.gateway.create(input) }
  update(id: string, input: AttributeCommandInput): Promise<DrawingAttribute> { return this.gateway.update(id, input) }
  remove(id: string): Promise<void> { return this.gateway.remove(id) }
  addField(attributeId: string, input: AttributeFieldCommandInput): Promise<DrawingAttributeField> {
    return this.gateway.addField(attributeId, input)
  }
  updateField(attributeId: string, fieldId: string, input: AttributeFieldCommandInput): Promise<DrawingAttributeField> {
    return this.gateway.updateField(attributeId, fieldId, input)
  }
  removeField(attributeId: string, fieldId: string): Promise<void> {
    return this.gateway.removeField(attributeId, fieldId)
  }

  async reorderFields(attributeId: string, fields: readonly DrawingAttributeField[]): Promise<void> {
    for (const [index, field] of fields.entries()) {
      await this.updateField(attributeId, field.id, {
        name: field.name,
        enabled: field.enabled,
        sortOrder: index + 1,
      })
    }
  }
}
