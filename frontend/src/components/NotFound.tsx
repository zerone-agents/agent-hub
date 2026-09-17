import { Result, Button } from 'antd'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'

export default function NotFound() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  return (
    <Result
      status="404"
      title="404"
      subTitle={t('components.notFound.subTitle')}
      extra={
        <Button type="primary" onClick={async () => { await navigate('/dashboard'); }}>
          {t('components.notFound.back')}
        </Button>
      }
    />
  )
}
