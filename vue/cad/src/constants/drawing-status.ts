export const STATUS = {
  published: { t: '生产中', c: 'ok' },
  reviewing: { t: '审核中', c: 'info' },
  draft: { t: '草稿', c: 'mute' },
  disabled: { t: '已禁用', c: 'danger' },
  archived: { t: '已存档', c: 'warn' },
} as const
