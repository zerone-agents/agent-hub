import { Space, Tag, Typography } from 'antd'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import type { LayoutModelKind } from './candidates'

/** Configuration status does not establish that a parser worker can run it. */
export default function LayoutModelHint({ kind, value }: { kind?: LayoutModelKind; value?: string }) {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" size={4} style={{ width: '100%' }}>
      <Typography.Text type="secondary">{t('knowledge.form.parseLayoutAvailability')}</Typography.Text>
      <Space size={4} wrap>
        <Tag>{t(kind === 'builtin' ? 'knowledge.form.layoutBuiltinStatus' : kind ? 'knowledge.form.layoutConfiguredStatus' : 'knowledge.form.layoutRetainedStatus')}</Tag>
        {kind === 'image2text' && <Tag color="warning">{t('knowledge.form.layoutExperimental')}</Tag>}
      </Space>
      <Typography.Text type={kind ? 'secondary' : 'warning'}>
        {t(kind === 'image2text' ? 'knowledge.form.layoutVisionHint' : kind === 'ocr' ? 'knowledge.form.layoutOcrHint' : kind === 'builtin' ? 'knowledge.form.layoutBuiltinHint' : 'knowledge.form.layoutRetainedHint', { value })}
      </Typography.Text>
      <Link to="/providers" target="_blank" rel="noopener noreferrer">{t('knowledge.form.configureParser')}</Link>
    </Space>
  )
}
