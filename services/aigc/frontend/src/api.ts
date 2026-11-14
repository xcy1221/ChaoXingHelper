export interface Job { id: string; status: string; message?: string; summary?: string; reportAvailable?: boolean }
export interface Health { ready: boolean; paused: boolean; canViewRecords: boolean; browserState?: string }
export const terminal = new Set(['succeeded', 'failed', 'uncertain']);
export const stages: Record<string, number> = { queued: 0, uploading: 1, submitting: 1, detecting: 2, waiting: 2, succeeded: 3 };
export const labels: Record<string, string> = { queued: '排队中', uploading: '上传中', submitting: '提交中', detecting: '检测中', waiting: '等待结果', succeeded: '已完成', failed: '失败', uncertain: '待核对' };
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch('api/' + path, { ...options, cache: 'no-store', credentials: 'same-origin' });
  if (response.status === 204) return undefined as T;
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || '服务暂时不可用');
  return data;
}
export function storedJob() {
  try { return new URLSearchParams(location.search).get('job') || sessionStorage.getItem('aigc-job'); } catch { return null; }
}
export function remember(id: string) {
  try { sessionStorage.setItem('aigc-job', id); } catch { /* The current page remains usable. */ }
  const url = new URL(location.href); url.searchParams.set('job', id); history.replaceState(null, '', url);
}
