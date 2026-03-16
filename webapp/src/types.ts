export type HostConfig = {
  name: string
  kind: 'local' | 'ssh'
  alias?: string
  host?: string
  user?: string
  port?: number
  identity_file?: string
}

export type ContainerRef = {
  id: string
  name: string
  status: string
  running: boolean
}

export type VolumeSummary = {
  name: string
  driver: string
  labels?: Record<string, string>
  attached_containers?: ContainerRef[]
  running_containers: number
  attached_containers_count: number
}

export type VolumeDetail = {
  summary: VolumeSummary
  size_bytes: number
  containers: ContainerRef[]
}

export type TransferOperation = 'copy' | 'clone' | 'move'
export type JobStatus = 'queued' | 'validating' | 'running' | 'cancelling' | 'completed' | 'failed' | 'cancelled'

export type TransferJobItem = {
  index: number
  source_volume: string
  destination_volume: string
  status: JobStatus
  bytes_estimated: number
  bytes_copied: number
  warnings?: string[]
  error?: string
  source_cleanup?: string
}

export type TransferJobEvent = {
  id: number
  job_id: string
  item_index: number
  level: string
  step: string
  message: string
  created_at: string
}

export type TransferJobSummary = {
  total_items: number
  completed_items: number
  failed_items: number
  cancelled_items: number
  running_items: number
  queued_items: number
  bytes_estimated: number
  bytes_copied: number
}

export type TransferJob = {
  id: string
  operation: TransferOperation
  source_host: string
  destination_host: string
  status: JobStatus
  allow_live: boolean
  quiesce_source: boolean
  requested_by?: string
  error?: string
  created_at: string
  started_at?: string
  finished_at?: string
  items: TransferJobItem[]
  events?: TransferJobEvent[]
  summary: TransferJobSummary
}

export type CreateTransferJobRequest = {
  operation: TransferOperation
  sourceHost: string
  destinationHost: string
  allowLive: boolean
  quiesceSource: boolean
  items: Array<{ sourceVolume: string; destinationVolume: string }>
}
