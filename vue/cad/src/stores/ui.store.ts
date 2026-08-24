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
  | 'reset-user'
  | 'edit-flow'

export interface ToastMessage {
  id: number
  message: string
  type: ToastType
}

export interface ModalState {
  type: ModalType
  title: string
  payload?: Record<string, string>
}

let toastId = 0

export const useUiStore = defineStore('ui', () => {
  const toasts = ref<ToastMessage[]>([])
  const modal = ref<ModalState | null>(null)

  function toast(message: string, type: ToastType = 'ok') {
    const id = ++toastId
    toasts.value.push({ id, message, type })
    window.setTimeout(() => {
      toasts.value = toasts.value.filter((item) => item.id !== id)
    }, 2800)
  }

  function openModal(type: ModalType, title: string, payload?: Record<string, string>) {
    modal.value = { type, title, payload }
  }

  function closeModal() {
    modal.value = null
  }

  return {
    toasts,
    modal,
    toast,
    openModal,
    closeModal,
  }
})
