import { Result } from 'antd'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import PrimaryButton from '@/components/PrimaryButton'

export default function NotFound() {
  const navigate = useNavigate()
  const { t } = useTranslation()
  return (
    <Result
      status="404"
      title="404"
      subTitle={t('components.notFound.subtitle')}
      extra={
        <PrimaryButton onClick={async () => { await navigate('/dashboard'); }}>
          {t('components.notFound.back')}
        </PrimaryButton>
      }
    />
  )
}
