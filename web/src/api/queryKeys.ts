import type { ID } from './types'

export interface GenerationJobQuery {
  episode_id?: ID
  status?: string
  capability?: string
  search?: string
  page?: number
  page_size?: number
}

export const queryKeys = {
  generationJobs: (projectID: ID, filters: GenerationJobQuery = {}) => ['generation-jobs', projectID, filters] as const,
  mediaJobs: (projectID: ID, page = 1, pageSize = 100) => ['media-jobs', projectID, { page, page_size: pageSize }] as const,
}
