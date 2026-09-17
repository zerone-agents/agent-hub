import { Dropdown } from 'antd'
import { TranslateIcon } from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import { useTranslation } from 'react-i18next'
import type { MenuProps } from 'antd'
import { useLanguage } from '@/hooks/useLanguage'
import { normalizeLanguage } from '@/i18n'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  trigger: css`
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: transparent;
    border-radius: ${t.radiusSm}px;
    color: ${t.text};
    cursor: pointer;
    transition: background 0.15s;
    &:hover {
      background: ${t.inkSubtle};
    }
  `
}))

/** 语言切换器：Translate 图标 + Dropdown 两项（中文/English）。 */
export default function LanguageSwitch() {
  const { styles } = useStyles()
  const { t } = useTranslation()
  const { language, setLanguage } = useLanguage()

  const items: MenuProps['items'] = [
    { key: 'zh', label: t('language.zh') },
    { key: 'en', label: t('language.en') }
  ]

  return (
    <Dropdown
      menu={{
        items,
        selectedKeys: [language],
        onClick: ({ key }) => {
          setLanguage(normalizeLanguage(key))
        }
      }}
      trigger={['click']}
    >
      <button
        type="button"
        className={styles.trigger}
        aria-label={t('language.switch')}
        title={t('language.switch')}
      >
        <TranslateIcon size={18} />
      </button>
    </Dropdown>
  )
}
