/** Standard API envelope returned by the Go backend. */
export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data: T
}

/** Paginated payload shape used by chat session/message endpoints. */
export interface PaginatedData<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  total_pages: number
}
