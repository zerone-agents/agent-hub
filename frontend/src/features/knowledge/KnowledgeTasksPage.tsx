import { useTranslation } from 'react-i18next'
import { useParams } from 'react-router'
import KnowledgeIngestionPanel from './KnowledgeIngestionPanel'

export default function KnowledgeTasksPage() {
  const { id = '' } = useParams()
  const { t } = useTranslation()
  return <section aria-label={t('knowledge.tabs.ingestions')}>
    <p>{t('knowledge.tasksHint')}</p>
    <KnowledgeIngestionPanel id={id} />
  </section>
}
