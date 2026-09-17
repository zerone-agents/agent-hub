import {
  Form,
  Input,
  InputNumber,
  Switch,
  Tag,
  Empty,
  Spin,
  Tooltip
} from 'antd'
import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useParams } from 'react-router'
import PrimaryButton from '@/components/PrimaryButton'
import { useCanWrite } from '@/hooks/useCanWrite'
import { useRetrievalTest } from '@/queries/useKnowledge'
import { tokens as tk } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  form: css`
    background: ${tk.surface}; border-radius: ${tk.radius}px; box-shadow: ${tk.elevation1};
    padding: 20px 24px; margin: 8px 0 20px;
  `,
  params: css`
    display: flex; gap: 16px; flex-wrap: wrap; align-items: flex-end;
  `,
  resultHead: css`
    font-size: ${tk.textSm}; color: ${tk.textTertiary}; margin-bottom: 12px;
  `,
  card: css`
    background: ${tk.surface}; border-radius: ${tk.radius}px; box-shadow: ${tk.elevation1};
    padding: 16px 18px; margin-bottom: 12px;
    animation: cardUp 0.3s ease backwards;
    @keyframes cardUp {
      from { opacity: 0; transform: translateY(6px); }
      to { opacity: 1; transform: translateY(0); }
    }
  `,
  cardMeta: css`
    display: flex; align-items: center; gap: 10px; margin-bottom: 8px; flex-wrap: wrap;
  `,
  docName: css`
    font-size: ${tk.textSm}; font-weight: 600; color: ${tk.text};
  `,
  docId: css`
    font-size: ${tk.textXs}; color: ${tk.textMuted};
  `,
  content: css`
    font-size: ${tk.textSm}; color: ${tk.textSecondary}; line-height: 1.6; white-space: pre-wrap;
  `,
  loadingWrap: css`
    display: flex; justify-content: center; padding: 60px 0;
  `
}))

interface RetrievalFormValues {
  question: string
  top_k: number
  similarity_threshold: number
  vector_similarity_weight: number
  highlight: boolean
}

const DEFAULTS: RetrievalFormValues = {
  question: '',
  top_k: 1024,
  similarity_threshold: 0.2,
  vector_similarity_weight: 0.3,
  highlight: false
}

export default function KnowledgeRetrievalPage() {
  const { t } = useTranslation()
  const { styles } = useStyles()
  const { id = '' } = useParams()
  const [form] = Form.useForm<RetrievalFormValues>()
  const retrieval = useRetrievalTest()
  const canWrite = useCanWrite()

  const handleFinish = (values: RetrievalFormValues) => {
    retrieval.mutate({
      question: values.question.trim(),
      dataset_ids: [id],
      top_k: values.top_k,
      similarity_threshold: values.similarity_threshold,
      vector_similarity_weight: values.vector_similarity_weight,
      highlight: values.highlight
    })
  }

  const result = retrieval.data

  return (
    <div>
      <Form
        form={form}
        layout="vertical"
        className={styles.form}
        initialValues={DEFAULTS}
        onFinish={handleFinish}
      >
        <Form.Item
          label={t('knowledge.retrieval.question')}
          name="question"
          rules={[{ required: true, message: t('knowledge.retrieval.questionRequired') }]}
        >
          <Input.TextArea rows={2} placeholder={t('knowledge.retrieval.questionPh')} />
        </Form.Item>
        <div className={styles.params}>
          <Form.Item label="top_k" name="top_k" style={{ marginBottom: 0 }}>
            <InputNumber min={1} max={4096} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item label={t('knowledge.retrieval.threshold')} name="similarity_threshold" style={{ marginBottom: 0 }}>
            <InputNumber min={0} max={1} step={0.05} style={{ width: 120 }} />
          </Form.Item>
          <Form.Item label={t('knowledge.retrieval.vectorWeight')} name="vector_similarity_weight" style={{ marginBottom: 0 }}>
            <InputNumber min={0} max={1} step={0.05} style={{ width: 140 }} />
          </Form.Item>
          <Form.Item label={t('knowledge.retrieval.highlight')} name="highlight" valuePropName="checked" style={{ marginBottom: 0 }}>
            <Switch />
          </Form.Item>
          {canWrite && (
            <Form.Item style={{ marginBottom: 0 }}>
              <PrimaryButton
                htmlType="submit"
                icon={<MagnifyingGlassIcon size={16} />}
                loading={retrieval.isPending}
              >
                {t('knowledge.retrieval.testBtn')}
              </PrimaryButton>
            </Form.Item>
          )}
        </div>
      </Form>

      {retrieval.isPending ? (
        <div className={styles.loadingWrap}>
          <Spin />
        </div>
      ) : result ? (
        result.chunks.length > 0 ? (
          <div>
            <div className={styles.resultHead}>{t('knowledge.retrieval.recalled', { n: result.total })}</div>
            {result.chunks.map((chunk) => (
              <div key={chunk.id} className={styles.card}>
                <div className={styles.cardMeta}>
                  <Tag color="blue">{t('knowledge.retrieval.similarity', { v: chunk.similarity.toFixed(3) })}</Tag>
                  <Tooltip title={t('knowledge.retrieval.simTooltip')}>
                    <Tag>
                      {t('knowledge.retrieval.vectorTerm', { v: chunk.vector_similarity.toFixed(2), t: chunk.term_similarity.toFixed(2) })}
                    </Tag>
                  </Tooltip>
                  <span className={styles.docName}>{chunk.document_name || t('knowledge.retrieval.unknownDoc')}</span>
                  <span className={styles.docId}>{chunk.document_id}</span>
                </div>
                <div className={styles.content}>{chunk.content}</div>
              </div>
            ))}
          </div>
        ) : (
          <Empty description={t('knowledge.retrieval.emptyNoResult')} />
        )
      ) : (
        <Empty description={t('knowledge.retrieval.emptyHint')} />
      )}
    </div>
  )
}
