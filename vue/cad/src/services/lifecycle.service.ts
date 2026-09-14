import { getApiBaseUrl } from '@/services/api-base.service'
import { isTauri } from '@tauri-apps/api/core'
import { saveDownloadFile } from '@/services/tauri/cad-edit.service'

export async function lifecycleFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const token = localStorage.getItem('cad_access_token') || sessionStorage.getItem('cad_access_token') || ''
  const headers = new Headers(init.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  const response = await fetch(`${getApiBaseUrl()}${path}`, { ...init, headers })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.message || `请求失败（${response.status}）`)
  }
  return response
}
export async function lifecycleApi<T>(path: string, init: RequestInit = {}): Promise<T> {
  const body = await (await lifecycleFetch(path, init)).json()
  return body.data as T
}
export async function downloadEvidence(path: string, name: string) {
  const blob = await (await lifecycleFetch(path)).blob()
  if (isTauri()) {
    await saveDownloadFile(name,new Uint8Array(await blob.arrayBuffer()))
    return
  }
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a'); link.href = url; link.download = name
  document.body.appendChild(link); link.click(); link.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}
export interface EvidenceDocument {
  folderPath: string
  id: string; title: string; category: string; description: string; fileName: string; size: number
  sha256: string; changeRequestId?: string; createdAt: string; createdBy: string
}
export interface LifecycleNode { name: string; assignedName: string; status: string; opinion: string; reviewedAt?: string }
export interface LifecycleSubmission {
  id: string; round: number; actualChanges: string; proposedAttributes: Record<string, string>; status: string; createdAt: string
  files: { attachmentId: string; name: string; baseVersionId?: string; submittedVersionId?: string }[]
  review?: { id: string; status: string; nodes: LifecycleNode[] }
}
export interface LifecycleTree {
  id: string; drawingNo: string; name: string; status: string; version: string
  releases: { id:string; version:string; source:string; createdAt:string; snapshot:{ drawing:{name:string;material:string;vendor:string}; files:{attachmentId:string;name:string;versionId:string}[]; structure:unknown[]; bom:{item_no:number;name:string;spec:string;quantity:number|string;remark:string}[]; signers:{role:string;signer_name:string}[] } }[]
  changes: { id: string; requestNo: string; reason: string; scope: string; status: string; createdAt: string; terminalFiles: { name:string; versionId:string }[]; submissions: LifecycleSubmission[]; actions: { action: string; actor: string; opinion: string; createdAt: string }[] }[]
  reviews: { id: string; status: string; startedAt: string; nodes: LifecycleNode[] }[]
}
export interface PatentRecord {
  id: string; number: string; title: string; patentType: string; jurisdiction: string; ownerName: string
  responsibleId: string; responsibleName: string; drawingId: string | null; feeDue: string | null; expiresOn: string | null
  deadlineSource: string; reminderDays: number; notes: string; revision: number; feeDays: number | null; expiryDays: number | null
}
export function patentAlerts(p: PatentRecord) {
  const alerts: string[] = []
  if (p.feeDays !== null && p.feeDays <= p.reminderDays) alerts.push(p.feeDays < 0 ? `缴费期限已过 ${-p.feeDays} 天，请核实处理` : `距缴费期限 ${p.feeDays} 天`)
  if (p.expiryDays !== null && p.expiryDays <= p.reminderDays) alerts.push(p.expiryDays < 0 ? `登记的权利期限已届满 ${-p.expiryDays} 天` : `距权利到期 ${p.expiryDays} 天`)
  if (!p.feeDue || !p.expiresOn) alerts.push('期限信息未完整登记')
  return alerts
}
