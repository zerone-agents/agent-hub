import { Popconfirm, Button } from 'antd'
import { TrashIcon } from '@phosphor-icons/react'
import { useTranslation } from 'react-i18next'

interface ConfirmDeleteProps {
  onConfirm: () => void
  title?: string
  description?: string
  buttonText?: string
}

// 默认值走 t() 的哨兵模式：props 不传时组件体内回退 i18n 默认文案
// （函数参数默认值作用域拿不到 hook 的 t）。
export default function ConfirmDelete({
  onConfirm,
  title,
  description,
  buttonText
}: ConfirmDeleteProps) {
  const { t } = useTranslation()
  return (
    <Popconfirm
      title={title ?? t('components.confirmDelete.title')}
      description={description ?? t('components.confirmDelete.description')}
      okText={t('common.delete')}
      okButtonProps={{ danger: true }}
      cancelText={t('common.cancel')}
      onConfirm={onConfirm}
    >
      <Button type="text" danger icon={<TrashIcon size={16} />}>
        {buttonText ?? t('common.delete')}
      </Button>
    </Popconfirm>
  )
}
