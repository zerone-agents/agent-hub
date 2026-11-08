import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { memoryApi } from '@/api/memory'
import MemoryApplicationConfiguration from './MemoryApplicationConfiguration'

vi.mock('@/api/memory', async (original) => ({
  ...(await original<typeof import('@/api/memory')>()),
  memoryApi: { nativeConfig: vi.fn() }
}))
const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => {
    client.clear()
  })
  clients.length = 0
  vi.resetAllMocks()
})
describe('safe application configuration', () => {
  it('reads only application identity and preserves zero capacity and temperature', async () => {
    vi.mocked(memoryApi.nativeConfig).mockResolvedValue({
      name: 'Synthetic bank',
      memoryTypes: ['raw', 'semantic', 'episodic', 'procedural'],
      chatModel: '356',
      embeddingModel: '337',
      capacityBytes: 0,
      temperature: 0,
      forgettingPolicy: 'FIFO'
    })
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } }
    })
    clients.push(client)
    render(
      <QueryClientProvider client={client}>
        <MemoryApplicationConfiguration
          application={{
            tenantId: 't',
            applicationId: 'app',
            policyVersion: 'synthetic-v1',
            dataClass: 'synthetic',
            automaticExtractionSupported: true,
            nativeConfigurationSupported: true,
            memoryTypes: ['raw', 'semantic', 'episodic', 'procedural']
          }}
          queryPrefix={['configuration-test']}
        />
      </QueryClientProvider>
    )
    await screen.findByText('Synthetic bank')
    expect(screen.getByText('0 B')).toBeInTheDocument()
    expect(screen.getByText('0')).toBeInTheDocument()
    expect(screen.getByText('356')).toBeInTheDocument()
    expect(screen.getByText('337')).toBeInTheDocument()
    expect(memoryApi.nativeConfig).toHaveBeenCalledWith(
      { tenantId: 't', applicationId: 'app' },
      expect.any(AbortSignal)
    )
    expect(screen.queryByText(/使用率/)).not.toBeInTheDocument()
  })
})
