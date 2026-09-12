import { useState } from 'react'
import { Alert, Button, Empty, Input, Skeleton, Tag } from 'antd'
import {
  CheckCircleIcon,
  CodeIcon,
  PackageIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react'
import { createStyles } from 'antd-style'
import PrimaryButton from '@/components/PrimaryButton'
import { parseApiError } from '@/api/client'
import type { ExtensionValidationResult } from '@/api/extensions'
import { useH0AcceptanceInfo, useValidateExtension } from '@/queries/useExtensions'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  page: css`
    width: 100%;
    max-width: 1440px;
    margin: 0 auto;
    animation: extensionPageIn 0.24s ease;
    @keyframes extensionPageIn {
      from { opacity: 0; transform: translateY(4px); }
    }
    @media (prefers-reduced-motion: reduce) { animation: none; }
  `,
  header: css`
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 20px;
    @media (max-width: 720px) { flex-direction: column; gap: 12px; }
  `,
  title: css`
    margin: 0;
    color: ${t.text};
    font-size: ${t.text2xl};
    font-weight: 650;
    letter-spacing: -0.025em;
  `,
  subtitle: css`
    max-width: 70ch;
    margin: 5px 0 0;
    color: ${t.textTertiary};
    font-size: ${t.textBase};
    line-height: 1.6;
  `,
  versionStrip: css`
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
    padding: 12px 16px;
    margin-bottom: 16px;
    border: 1px solid var(--border);
    border-radius: ${t.radiusSm}px;
    background: var(--card);
  `,
  versionItem: css`
    display: flex;
    align-items: center;
    gap: 7px;
    color: ${t.textSecondary};
    font-size: ${t.textSm};
    strong { color: ${t.text}; font-weight: 600; }
    code { font-family: ${t.fontMono}; font-size: 12px; }
  `,
  workspace: css`
    display: grid;
    grid-template-columns: minmax(0, 1.45fr) minmax(300px, 0.75fr);
    min-height: 630px;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: ${t.radius}px;
    background: var(--card);
    box-shadow: ${t.elevation1};
    @media (max-width: 980px) { grid-template-columns: 1fr; }
  `,
  editor: css`
    min-width: 0;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
    @media (max-width: 980px) { border-right: 0; border-bottom: 1px solid var(--border); }
  `,
  toolbar: css`
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
    padding: 16px;
    border-bottom: 1px solid var(--border);
  `,
  field: css`
    min-width: min(100%, 300px);
    label { display: block; margin-bottom: 6px; color: ${t.textSecondary}; font-size: ${t.textSm}; font-weight: 600; }
  `,
  helper: css`
    margin-top: 5px;
    color: ${t.textMuted};
    font-size: 12px;
  `,
  editorBody: css`
    min-height: 430px;
    flex: 1;
    padding: 0;
    background: color-mix(in srgb, var(--background) 76%, var(--card));
    .ant-input { border: 0; border-radius: 0; background: transparent; box-shadow: none; resize: none; }
    .ant-input:focus { box-shadow: inset 3px 0 0 var(--primary); }
    textarea { min-height: 430px !important; padding: 18px 20px; font-family: ${t.fontMono}; font-size: 13px; line-height: 1.65; }
  `,
  editorFooter: css`
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 12px 16px;
    border-top: 1px solid var(--border);
    color: ${t.textMuted};
    font-size: 12px;
  `,
  inspector: css`
    min-width: 0;
    padding: 20px;
    background: var(--background);
  `,
  sectionTitle: css`
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0 0 5px;
    color: ${t.text};
    font-size: ${t.textLg};
    font-weight: 650;
  `,
  sectionHint: css`
    margin: 0 0 16px;
    color: ${t.textTertiary};
    font-size: ${t.textSm};
    line-height: 1.55;
  `,
  result: css`
    margin-bottom: 20px;
  `,
  packageMeta: css`
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 8px 14px;
    padding: 14px 0 18px;
    border-bottom: 1px solid var(--border);
    font-size: ${t.textSm};
    dt { color: ${t.textMuted}; }
    dd { min-width: 0; margin: 0; color: ${t.text}; font-family: ${t.fontMono}; overflow-wrap: anywhere; }
  `,
  categoryList: css`
    margin-top: 16px;
    border-top: 1px solid var(--border);
  `,
  category: css`
    display: grid;
    grid-template-columns: minmax(86px, 0.35fr) minmax(0, 1fr);
    gap: 12px;
    padding: 11px 0;
    border-bottom: 1px solid var(--border);
  `,
  categoryName: css`
    color: ${t.text};
    font-size: ${t.textSm};
    font-weight: 600;
  `,
  categoryDescription: css`
    color: ${t.textTertiary};
    font-size: 12px;
    line-height: 1.5;
  `,
  errorList: css`
    margin: 12px 0 0;
    padding: 0;
    list-style: none;
    li { padding: 8px 0; border-top: 1px solid var(--border); color: ${t.textSecondary}; font-size: ${t.textSm}; line-height: 1.5; }
    code { margin-right: 6px; color: var(--destructive); font-family: ${t.fontMono}; }
  `,
  empty: css`
    display: grid;
    min-height: 360px;
    place-items: center;
  `,
}))

function ValidationResult({ result }: { result: ExtensionValidationResult }) {
  const { styles } = useStyles()

  if (!result.valid) {
    return (
      <div className={styles.result} data-testid="validation-errors">
        <Alert
          type="error"
          showIcon
          icon={<WarningCircleIcon size={18} weight="fill" />}
          title="校验未通过"
          description="请按下面的定位修改 Manifest 后重新校验。"
        />
        <ul className={styles.errorList}>
          {(result.errors ?? []).map((error, index) => (
            <li key={`${error.path ?? 'manifest'}-${index}`}>
              <code>{error.path ?? 'manifest'}</code>{error.message}
            </li>
          ))}
        </ul>
      </div>
    )
  }

  return (
    <div className={styles.result} data-testid="validation-success">
      <Alert
        type="success"
        showIcon
        icon={<CheckCircleIcon size={18} weight="fill" />}
        title="扩展包校验通过"
        description="Manifest 符合当前平台扩展协议，可作为能力包开发和安装前检查结果。"
      />
      {result.package && (
        <dl className={styles.packageMeta}>
          <dt>名称</dt><dd>{result.package.name ?? '—'}</dd>
          <dt>命名空间</dt><dd>{result.package.namespace ?? '—'}</dd>
          <dt>版本</dt><dd>{result.package.version ?? '—'}</dd>
        </dl>
      )}
    </div>
  )
}

export default function ExtensionAcceptancePage() {
  const { styles } = useStyles()
  const info = useH0AcceptanceInfo()
  const validation = useValidateExtension()
  const [manifest, setManifest] = useState('')

  const contributions = (info.data?.contributionCategories ?? []).map((category) => ({
    ...category,
    count: validation.data?.valid
      ? validation.data.contributions?.find((item) => item.key === category.key)?.count
      : undefined,
  }))

  if (info.isLoading) {
    return <div className={styles.page}><Skeleton active paragraph={{ rows: 12 }} /></div>
  }

  if (info.isError || !info.data) {
    return (
      <div className={styles.page}>
        <Alert
          type="error"
          showIcon
          title="无法读取扩展协议"
          description={parseApiError(info.error)}
          action={<Button size="small" onClick={() => void info.refetch()}>重新加载</Button>}
        />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div>
          <h1 className={styles.title}>能力包检查</h1>
          <p className={styles.subtitle}>在安装前检查能力包的身份、权限和扩展内容。平台不内置任何垂直应用的业务规则。</p>
        </div>
        <Tag color="green" icon={<CheckCircleIcon size={14} weight="fill" />}>{info.data.status}</Tag>
      </header>

      <div className={styles.versionStrip} aria-label="平台协议版本">
        <span className={styles.versionItem}><PackageIcon size={17} /><span>平台 <strong><code>{info.data.platformVersion}</code></strong></span></span>
        <span className={styles.versionItem}><CodeIcon size={17} /><span>扩展协议 <strong><code>{info.data.protocolVersion}</code></strong></span></span>
      </div>

      <div className={styles.workspace}>
        <section className={styles.editor} aria-labelledby="manifest-title">
          <div className={styles.toolbar}>
            <div className={styles.field}>
              <label htmlFor="extension-manifest">能力包 Manifest</label>
              <div className={styles.helper}>粘贴 extension.yaml，平台只做协议校验，不会读取其中的本地文件路径。</div>
            </div>
          </div>

          <div className={styles.editorBody}>
            <Input.TextArea
              id="extension-manifest"
              name="extension-manifest"
              aria-label="extension.yaml"
              value={manifest}
              onChange={(event) => { setManifest(event.target.value); validation.reset() }}
              spellCheck={false}
            />
          </div>
          <div className={styles.editorFooter}>
            <span>{manifest.length.toLocaleString('zh-CN')} 字符</span>
            <PrimaryButton
              icon={<CheckCircleIcon size={16} weight="bold" />}
              loading={validation.isPending}
              disabled={!manifest.trim()}
              onClick={() => { validation.mutate(manifest) }}
            >
              校验扩展包
            </PrimaryButton>
          </div>
        </section>

        <aside className={styles.inspector} aria-labelledby="manifest-title">
          <h2 id="manifest-title" className={styles.sectionTitle}>校验结果</h2>
          <p className={styles.sectionHint}>结果来自后端使用同一份协议 Schema 的真实校验。</p>

          {validation.isError && (
            <Alert className={styles.result} type="error" showIcon title="校验请求失败" description={parseApiError(validation.error)} />
          )}
          {validation.data && <ValidationResult result={validation.data} />}
          {!validation.data && !validation.isError && (
            <div className={styles.empty}>
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="选择示例或编辑 YAML，然后点击校验" />
            </div>
          )}

          <h2 className={styles.sectionTitle}>协议贡献项</h2>
          <p className={styles.sectionHint}>能力包可以声明以下平台扩展点，但不能直接修改 Hub 核心状态。</p>
          <div className={styles.categoryList}>
            {contributions.map((category) => (
              <div className={styles.category} key={category.key}>
                <div className={styles.categoryName}>
                  {category.label}
                  {category.count !== undefined && <Tag variant="filled">{category.count}</Tag>}
                </div>
                <div className={styles.categoryDescription}>{category.description ?? category.key}</div>
              </div>
            ))}
          </div>
        </aside>
      </div>
    </div>
  )
}
