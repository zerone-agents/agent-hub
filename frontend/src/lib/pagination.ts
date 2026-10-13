import type { SelectProps } from 'antd'

/** antd「每页条数」下拉的默认可见性阈值（totalBoundaryShowSizeChanger）：total 超过它才渲染。 */
export const SIZE_CHANGER_MIN_TOTAL = 50

/**
 * 关掉搜索的「每页条数」下拉配置。
 *
 * antd 给这个 Select 硬编码了 `showSearch: true`，而它的选项一共只有
 * 10/20/50/100 几项——打开后变成可键入的搜索框既无意义，又让内层 input 变成可编辑
 * （I 型文本光标、内容可拖选）。传对象即可关掉（antd 的用户 props 展开在硬编码之后，
 * 且对象形态不会关掉下拉本身，见 useShowSizeChanger）。
 */
export const SIZE_CHANGER_NO_SEARCH: SelectProps = { showSearch: false }

/**
 * 按 antd 的可见性语义（total > {@link SIZE_CHANGER_MIN_TOTAL} 才出现）返回
 * 「每页条数」下拉配置。显式传对象会让下拉在小数据量下也常显，故不直接传常量。
 */
export function sizeChangerProps(total: number): SelectProps | false {
  return total > SIZE_CHANGER_MIN_TOTAL ? SIZE_CHANGER_NO_SEARCH : false
}
