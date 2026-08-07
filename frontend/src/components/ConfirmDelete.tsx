import { Popconfirm, Button } from 'antd'
import { Trash } from '@phosphor-icons/react'

interface ConfirmDeleteProps {
  onConfirm: () => void
  title?: string
  description?: string
  buttonText?: string
}

export default function ConfirmDelete({
  onConfirm,
  title = '确认删除？',
  description = '删除后不可恢复',
  buttonText = '删除'
}: ConfirmDeleteProps) {
  return (
    <Popconfirm
      title={title}
      description={description}
      okText="删除"
      okButtonProps={{ danger: true }}
      cancelText="取消"
      onConfirm={onConfirm}
    >
      <Button type="text" danger icon={<Trash size={16} />}>
        {buttonText}
      </Button>
    </Popconfirm>
  )
}
