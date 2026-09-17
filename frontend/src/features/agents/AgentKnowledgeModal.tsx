import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Modal, Spin, Transfer, Button } from 'antd'
import type { TransferProps } from 'antd'
import { BooksIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import type { Agent } from '@/api/agents'
import { useAgentKnowledgeDatasets, useUpdateAgentKnowledgeDatasets } from '@/queries/useAgents'
import { useKnowledgeListAll } from '@/queries/useKnowledge'

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
  const { t } = useTranslation()
  const name = agent?.name ?? ''

  const { data: boundIds = [], isLoading: isLoadingBound } = useAgentKnowledgeDatasets(name)
  const { data: listData, error: listError, isLoading: isLoadingList } = useKnowledgeListAll()
  const updateMutation = useUpdateAgentKnowledgeDatasets()

  const [targetKeys, setTargetKeys] = useState<string[]>([])

  // issue #122 review P2：ghost 判定依赖完整目录——列表请求失败或分页未取全
  //（total > 已取数）时 liveness 未知，宁缺勿假，不注入 ghost。
  const listComplete = !listError && !!listData && listData.datasets.length >= listData.total

  const dataSource: TransferItem[] = useMemo(() => {
    const items = (listData?.datasets ?? []).map((ds) => ({
      key: ds.id,
      title: ds.name || t('agents.knowledgeModal.unnamed'),
      description: ds.description || ''
    }))
    if (listComplete) {
      // issue #122：绑定指向但已不在存活列表的库注入为 ghost 项——可见、
      // 可左移解除、可保存。不加 disabled（disabled 项不可移动 = 重新不可删）。
      const liveKeys = new Set(items.map((item) => item.key))
      for (const id of boundIds) {
        if (!liveKeys.has(id)) {
          items.push({
            key: id,
            title: t('agents.knowledgeModal.deletedKb', { id: id.slice(0, 8) }),
            description: t('agents.knowledgeModal.deletedKbDesc')
          })
        }
      }
    }
    return items
  }, [t, listData, boundIds, listComplete])

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
      <span>{agent ? t('agents.knowledgeModal.configureFor', { name: agent.config.title?.zh ?? agent.name }) : t('agents.knowledgeModal.configure')}</span>
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
          <Button onClick={handleCancel}>{t('common.cancel')}</Button>
          {canWrite && (
            <PrimaryButton onClick={handleOk} loading={updateMutation.isPending}>{t('agents.knowledgeModal.save')}</PrimaryButton>
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
          titles={[t('agents.knowledgeModal.source'), t('agents.knowledgeModal.target')]}
          render={(item) => item.title}
          disabled={!canWrite}
          styles={{ section: { width: 280, height: 360 } }}
        />
      )}
    </Modal>
  )
}
