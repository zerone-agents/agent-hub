import { useEffect, useMemo, useState } from 'react'
import { Modal, Spin, Transfer, Button } from 'antd'
import type { TransferProps } from 'antd'
import { BooksIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Agent } from '@/api/agents'
import { useAgentKnowledgeDatasets, useUpdateAgentKnowledgeDatasets } from '@/queries/useAgents'
import { useKnowledgeList } from '@/queries/useKnowledge'

interface AgentKnowledgeModalProps {
  open: boolean
  agent: Agent | null
  /** 只读模式（member）：Transfer 禁改、隐藏保存按钮，仅可查看绑定关系。 */
  canWrite: boolean
  onClose: () => void
}

interface TransferItem {
  key: string
  title: string
  description: string
}

const useStyles = createStyles(({ css }) => ({
  head: css`
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 16px;
    font-weight: 600;
  `,
  loadingWrap: css`
    display: flex;
    justify-content: center;
    padding: 60px 0;
  `,
  transferWrap: css`
    .ant-transfer-list {
      border-radius: 6px;
    }
  `
}))

export default function AgentKnowledgeModal({ open, agent, canWrite, onClose }: AgentKnowledgeModalProps) {
  const { styles } = useStyles()
  const name = agent?.name ?? ''

  const { data: boundIds = [], isLoading: isLoadingBound } = useAgentKnowledgeDatasets(name)
  const { data: listData, isLoading: isLoadingList } = useKnowledgeList({ page_size: 1000 })
  const updateMutation = useUpdateAgentKnowledgeDatasets()

  const [targetKeys, setTargetKeys] = useState<string[]>([])

  const dataSource: TransferItem[] = useMemo(() => {
    const items = (listData?.datasets ?? []).map((ds) => ({
      key: ds.id,
      title: ds.name || '未命名',
      description: ds.description || ''
    }))
    // issue #122：绑定指向但已不在存活列表的库注入为 ghost 项——可见、
    // 可左移解除、可保存。不加 disabled（disabled 项不可移动 = 重新不可删）。
    const liveKeys = new Set(items.map((item) => item.key))
    for (const id of boundIds) {
      if (!liveKeys.has(id)) {
        items.push({
          key: id,
          title: `已删除的知识库（${id.slice(0, 8)}…）`,
          description: '绑定指向的知识库已不存在，左移移除后保存即可恢复部署'
        })
      }
    }
    return items
  }, [listData, boundIds])

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- sync targetKeys with the latest boundIds every time the modal opens or bound ids refetch
      setTargetKeys(boundIds)
    }
  }, [open, boundIds])

  const handleChange: TransferProps<TransferItem>['onChange'] = (nextTargetKeys) => {
    setTargetKeys(nextTargetKeys as string[])
  }

  const handleOk = async () => {
    if (!name) return
    await updateMutation.mutateAsync({ name, datasetIds: targetKeys })
    onClose()
  }

  const handleCancel = () => {
    onClose()
  }

  const titleNode = (
    <div className={styles.head}>
      <BooksIcon size={20} weight="duotone" />
      <span>{agent ? `配置知识库：${agent.config.title?.zh ?? agent.name}` : '配置知识库'}</span>
    </div>
  )

  const isLoading = isLoadingBound || isLoadingList

  return (
    <Modal
      open={open}
      title={titleNode}
      onCancel={handleCancel}
      width={640}
      destroyOnHidden
      footer={
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
          <Button onClick={handleCancel}>取消</Button>
          {canWrite && (
            <PrimaryButton onClick={handleOk} loading={updateMutation.isPending}>保存</PrimaryButton>
          )}
        </div>
      }
    >
      {isLoading ? (
        <div className={styles.loadingWrap}>
          <Spin />
        </div>
      ) : (
        <Transfer<TransferItem>
          className={styles.transferWrap}
          dataSource={dataSource}
          targetKeys={targetKeys}
          onChange={handleChange}
          titles={['可选知识库', '已绑定']}
          render={(item) => item.title}
          disabled={!canWrite}
          styles={{ section: { width: 280, height: 360 } }}
        />
      )}
    </Modal>
  )
}
