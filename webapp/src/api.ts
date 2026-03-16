import type { CreateTransferJobRequest, HostConfig, TransferJob, TransferJobEvent, VolumeDetail, VolumeSummary } from './types'

async function request<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: response.statusText })) as { error?: string }
    throw new Error(payload.error || response.statusText)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

function arrayOrEmpty<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

function normalizeJob(job: TransferJob): TransferJob {
  return {
    ...job,
    items: arrayOrEmpty(job.items),
    events: arrayOrEmpty(job.events),
  }
}

export const api = {
  hosts: async () => {
    const data = await request<{ hosts: HostConfig[] | null }>('/api/v1/hosts')
    return { hosts: arrayOrEmpty(data.hosts) }
  },
  importSSH: async () => {
    const data = await request<{ hosts: HostConfig[] | null }>('/api/v1/hosts/import-ssh', { method: 'POST' })
    return { hosts: arrayOrEmpty(data.hosts) }
  },
  saveHost: (payload: { name: string; kind: string; alias?: string; host?: string; user?: string; port?: number; identityFile?: string }) => request<{ host: HostConfig }>('/api/v1/hosts', { method: 'POST', body: JSON.stringify(payload) }),
  deleteHost: (name: string) => request<void>(`/api/v1/hosts/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  testHost: (name: string) => request<{ version: string }>(`/api/v1/hosts/${encodeURIComponent(name)}/test`, { method: 'POST' }),
  volumes: async (host: string, hideAnonymous: boolean) => {
    const data = await request<{ host: string; volumes: VolumeSummary[] | null }>(`/api/v1/volumes?host=${encodeURIComponent(host)}&hideAnonymous=${hideAnonymous ? '1' : '0'}`)
    return { host: data.host, volumes: arrayOrEmpty(data.volumes) }
  },
  volumeDetail: async (host: string, name: string) => {
    const detail = await request<VolumeDetail>(`/api/v1/volumes/${encodeURIComponent(host)}/${encodeURIComponent(name)}`)
    return {
      ...detail,
      containers: arrayOrEmpty(detail.containers),
    }
  },
  createJob: (payload: CreateTransferJobRequest) => request<{ jobId: string }>('/api/v1/transfers/jobs', { method: 'POST', body: JSON.stringify(payload) }),
  jobs: async (query = '') => {
    const data = await request<{ jobs: TransferJob[] | null }>(`/api/v1/transfers/jobs${query ? `?${query}` : ''}`)
    return { jobs: arrayOrEmpty(data.jobs).map(normalizeJob) }
  },
  job: async (id: string) => normalizeJob(await request<TransferJob>(`/api/v1/transfers/jobs/${encodeURIComponent(id)}`)),
  cancelJob: (id: string) => request<{ status: string }>(`/api/v1/transfers/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  jobEvents: (id: string, onEvent: (event: TransferJobEvent) => void) => {
    const source = new EventSource(`/api/v1/transfers/jobs/${encodeURIComponent(id)}/events`)
    source.onmessage = (message) => {
      onEvent(JSON.parse(message.data) as TransferJobEvent)
    }
    source.addEventListener('warning', (message) => {
      onEvent(JSON.parse((message as MessageEvent).data) as TransferJobEvent)
    })
    source.addEventListener('failed', (message) => {
      onEvent(JSON.parse((message as MessageEvent).data) as TransferJobEvent)
    })
    source.addEventListener('completed', (message) => {
      onEvent(JSON.parse((message as MessageEvent).data) as TransferJobEvent)
    })
    return () => source.close()
  },
}
