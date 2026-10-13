import { fireEvent, screen } from '@testing-library/react'

/**
 * 通过 antd 的「每页条数」下拉切换 pageSize。
 *
 * 注意：antd 只在 total > 50 时才渲染这个下拉（totalBoundaryShowSizeChanger 默认值），
 * 所以调用前 mock 的 total 必须大于 50。选项文本形如 "50 / page" / "50 条/页"，
 * 这里按数字前缀匹配，不依赖 antd 文案语言。
 */
export async function switchPageSizeTo(size: number) {
  const combo = document.querySelector(
    '.ant-pagination-options-size-changer .ant-select-content'
  ) as HTMLElement | null
  if (!combo) throw new Error('未找到「每页条数」下拉（total 是否 > 50？）')
  fireEvent.mouseDown(combo)
  const option = (await screen.findAllByRole('option')).find((o) =>
    (o.getAttribute('title') ?? '').trim().startsWith(String(size))
  )
  if (!option) throw new Error(`下拉中没有 ${size} 条/页 选项`)
  fireEvent.click(option)
}