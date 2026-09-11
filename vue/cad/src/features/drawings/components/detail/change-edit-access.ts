interface EditableChange {
  status: string
  executorId: string
  targets?: readonly { attachmentId: string }[]
}

// 存档授权只属于指定执行人和目标附件；缺少明细时不扩大授权。
export function editableChangeTargets(requests: readonly EditableChange[], userId: string): Set<string> {
  return new Set(requests.filter((request) => request.status === 'executing' && request.executorId === userId)
    .flatMap((request) => request.targets?.map((target) => target.attachmentId) ?? []))
}
