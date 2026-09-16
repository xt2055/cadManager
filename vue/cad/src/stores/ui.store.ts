import { defineStore } from 'pinia'
import { ref } from 'vue'

export type ToastType = 'ok' | 'info' | 'warn'

export type ModalType =
  | 'create-drawing'
  | 'upload-version'
  | 'borrow-drawing'
  | 'revert'
  | 'sync-original'
  | 'exit'
  | 'add-user'
  | 'edit-user-role'
  | 'reset-user'
  | 'edit-flow'
  | 'confirm'

export interface ToastMessage {
  id: number
  message: string
  type: ToastType
}

export interface ModalState {
  type: ModalType
  title: string
  payload?: Record<string, string>
  onConfirm?: () => void | Promise<void>
  onCancel?: () => void
}

let toastId = 0

export const useUiStore = defineStore('ui', () => {
  let leaveGuard: (() => Promise<boolean>) | null = null
  function setLeaveGuard(guard: () => Promise<boolean>) {
    leaveGuard = guard
    return () => { if (leaveGuard === guard) leaveGuard = null }
  }
  async function confirmNavigation(): Promise<boolean> {
    return leaveGuard ? leaveGuard() : true
  }
  const toasts = ref<ToastMessage[]>([])
  const modal = ref<ModalState | null>(null)

  function toast(message: string, type: ToastType = 'ok') {
    const id = ++toastId
    toasts.value.push({ id, message, type })
    window.setTimeout(() => {
      toasts.value = toasts.value.filter((item) => item.id !== id)
    }, 2800)
  }

  function openModal(type: ModalType, title: string, payload?: Record<string, string>, onConfirm?: () => void | Promise<void>) {
    closeModal()
    modal.value = { type, title, payload, onConfirm }
  }

  /** 通用确认弹窗：以项目自绘风格替代 window.confirm。 */
  function confirm(title: string, message: string, options?: { confirmText?: string; danger?: boolean; onConfirm: () => void | Promise<void> }) {
    openModal(
      'confirm',
      title,
      {
        message,
        confirmText: options?.confirmText || '确定',
        danger: options?.danger ? '1' : '',
      },
      options?.onConfirm,
    )
  }

  function askConfirm(title: string, message: string, confirmText: string, cancelText = '取消'): Promise<boolean> {
    closeModal()
    return new Promise((resolve) => {
      modal.value = {
        type: 'confirm', title,
        payload: { message, confirmText, cancelText, danger: '1' },
        onConfirm: () => resolve(true),
        onCancel: () => resolve(false),
      }
    })
  }

  function closeModal(confirmed = false) {
    const current = modal.value
    modal.value = null
    if (!confirmed) current?.onCancel?.()
  }

  return {
    toasts,
    modal,
    toast,
    openModal,
    confirm,
    askConfirm,
    setLeaveGuard,
    confirmNavigation,
    closeModal,
  }
})
