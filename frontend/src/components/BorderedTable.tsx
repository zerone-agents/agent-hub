import { Table, type TableProps } from 'antd'
import { createStyles } from 'antd-style'

const useStyles = createStyles(({ css }) => ({
  tableWrap: css`
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;

    .ant-table,
    .ant-table-container,
    .ant-table-content {
      border-radius: 0 !important;
    }

    .ant-table-thead > tr:first-child > th:first-child,
    .ant-table-thead > tr:first-child > th:last-child {
      border-radius: 0 !important;
    }
  `
}))

/**
 * antd Table 包装组件，统一表格的外边框 + 圆角样式。
 * 用法与 antd Table 完全一致，直接替换 <Table /> 即可。
 */
function BorderedTable<T extends object>(props: TableProps<T>) {
  const { styles } = useStyles()
  return (
    <div className={styles.tableWrap}>
      <Table<T> {...props} />
    </div>
  )
}

export default BorderedTable
