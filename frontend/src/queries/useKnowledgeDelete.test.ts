import { afterEach, expect, it, vi } from 'vitest';
import apiClient from '@/api/client';
import { deleteKnowledgeSelection } from './useKnowledge';
import type { KnowledgeWriteOwner } from '@/api/knowledge';

afterEach(() => vi.restoreAllMocks());
const inventory = (total: number, ids: string[]) => ({ data: { success: true, data: { total, datasets: ids.map((id) => ({ id, name: 'Same name' })) } } });
const empty = inventory(0, []);

it('confirms selected missing IDs through a complete directory even though filtered GET would fail', async () => {
  const remove = vi.spyOn(apiClient, 'delete').mockResolvedValue({ data: { success: true } });
  const get = vi.spyOn(apiClient, 'get').mockImplementation(async (url) => url.includes('id=') ? { data: { success: false, error: 'User lacks permission for dataset' } } : inventory(1, ['b']));
  const result = await deleteKnowledgeSelection(['a', 'b', 'a']);
  expect(remove.mock.calls.map(([, config]) => config?.data)).toEqual([{ ids: ['a'] }, { ids: ['b'] }]);
  expect(get.mock.calls.map(([url]) => url)).toEqual(['/api/v1/admin/knowledge/datasets?page=1&page_size=100']);
  expect(result.deletedIds).toEqual(['a']);
  expect(result.failed.map(({ id }) => id)).toEqual(['b']);
});

it('reads every unfiltered page before asserting absence, comparing IDs rather than names', async () => {
  vi.spyOn(apiClient, 'delete').mockResolvedValue({ data: { success: true } });
  const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce(inventory(101, Array.from({ length: 100 }, (_, i) => `unselected-${i}`))).mockResolvedValueOnce(inventory(101, ['last-page']));
  expect((await deleteKnowledgeSelection(['deleted', 'last-page'])).deletedIds).toEqual(['deleted']);
  expect(get.mock.calls.map(([url]) => url)).toEqual(['/api/v1/admin/knowledge/datasets?page=1&page_size=100', '/api/v1/admin/knowledge/datasets?page=2&page_size=100']);
});

it.each(['total change', 'duplicate IDs', 'truncated page', 'second page permission failure'])('confirms nothing when the complete directory has %s', async (failure) => {
  vi.spyOn(apiClient, 'delete').mockResolvedValue({ data: { success: true } });
  const get = vi.spyOn(apiClient, 'get').mockResolvedValueOnce(inventory(101, Array.from({ length: 100 }, (_, i) => `id-${i}`)));
  if (failure === 'total change') get.mockResolvedValueOnce(inventory(100, []));
  else if (failure === 'duplicate IDs') get.mockResolvedValueOnce(inventory(101, ['id-0']));
  else if (failure === 'truncated page') get.mockResolvedValueOnce(inventory(101, []));
  else get.mockResolvedValueOnce({ data: { success: false, error: 'No authorization' } });
  const result = await deleteKnowledgeSelection(['deleted']);
  expect(result.deletedIds).toEqual([]);
  expect(result.failed.map(({ id }) => id)).toEqual(['deleted']);
});

it.each(['read failure', 'malformed total', 'safety limit'])('keeps a business failure and successful DELETE unconfirmed on %s', async (failure) => {
  vi.spyOn(apiClient, 'delete').mockResolvedValueOnce({ data: { success: false, error: 'Bound dataset' } }).mockResolvedValue({ data: { success: true } });
  const get = vi.spyOn(apiClient, 'get');
  if (failure === 'read failure') get.mockRejectedValueOnce(new Error('Offline'));
  else if (failure === 'malformed total') get.mockResolvedValueOnce({ data: { success: true, data: { datasets: [] } } });
  else get.mockResolvedValueOnce(inventory(20001, []));
  const result = await deleteKnowledgeSelection(['bound', 'uncertain']);
  expect(result.deletedIds).toEqual([]);
  expect(result.failed.map(({ id }) => id)).toEqual(['bound', 'uncertain']);
});

it('stops before readback or another DELETE when the initiating account changes', async () => {
  let current = true;
  const owner: KnowledgeWriteOwner = { signal: new AbortController().signal, isCurrent: () => current, assertCurrent: () => { if (!current) throw new Error('Owner changed'); } };
  const remove = vi.spyOn(apiClient, 'delete').mockImplementation(async () => { current = false; return { data: { success: true } }; });
  const get = vi.spyOn(apiClient, 'get');
  await expect(deleteKnowledgeSelection(['a', 'b'], owner)).rejects.toThrow('Owner changed');
  expect(remove).toHaveBeenCalledTimes(1);
  expect(get).not.toHaveBeenCalled();
});

it('rejects an empty selection instead of issuing an unscoped DELETE', async () => {
  const remove = vi.spyOn(apiClient, 'delete');
  await expect(deleteKnowledgeSelection([])).rejects.toThrow('Empty dataset selection');
  await expect(deleteKnowledgeSelection([''])).rejects.toThrow('Empty dataset selection');
  expect(remove).not.toHaveBeenCalled();
});

it('rejects invalid IDs in a supposedly complete inventory', async () => {
  vi.spyOn(apiClient, 'delete').mockResolvedValue({ data: { success: true } });
  vi.spyOn(apiClient, 'get').mockResolvedValueOnce(inventory(1, ['']));
  expect((await deleteKnowledgeSelection(['deleted'])).deletedIds).toEqual([]);
});

it('accepts a trusted empty unfiltered directory', async () => {
  vi.spyOn(apiClient, 'delete').mockResolvedValue({ data: { success: true } });
  vi.spyOn(apiClient, 'get').mockResolvedValueOnce(empty);
  expect(await deleteKnowledgeSelection(['last'])).toEqual({ deletedIds: ['last'], failed: [] });
});
