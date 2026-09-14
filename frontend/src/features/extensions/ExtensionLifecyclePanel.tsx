// H7.1 扩展详情页 · 生命周期操作区。
//
// 安装（选版本）/启用/停用/升级（选目标版本）/回滚/卸载；卸载前必显
// 影响范围预览弹窗（谁依赖它、它依赖谁、新增 stateSchemas/权限），
// 所有操作结果用中文 toast 反馈。
import { useState } from 'react'
import { Alert, Button, Card, Descriptions, Modal, Select, Space, Tag, message } from 'antd'
import { createStyles } from 'antd-style'
import { parseApiError } from '@/api/client'
import type { ExtensionDetail } from '@/api/extensionRegistry'
import {
  useDisableExtension,
  useEnableExtension,
  useExtensionImpact,
  useInstallExtension,
  useRollbackExtension,
  useUninstallExtension,
  useUpgradeExtension
} from '@/queries/useExtensionRegistry'
import { tokens as t } from '@/styles/tokens'

const useStyles = createStyles(({ css }) => ({
  card: css`
    margin-bottom: 24px;
  `,
  row: css`
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  `
}))

export default function ExtensionLifecyclePanel({
  id,
  data
}: {
  id: number
  data: ExtensionDetail
}) {
  const { styles } = useStyles()
  const [installVersion, setInstallVersion] = useState<string>()
  const [upgradeTarget, setUpgradeTarget] = useState<string>()
  const [rollbackTarget, setRollbackTarget] = useState<string>()
  const [impactOpen, setImpactOpen] = useState(false)
  const [uninstallMode, setUninstallMode] = useState<{ force: boolean; purge: boolean }>()

  const install = useInstallExtension(id)
  const enable = useEnableExtension(id)
  const disable = useDisableExtension(id)
  const upgrade = useUpgradeExtension(id)
  const rollback = useRollbackExtension(id)
  const uninstall = useUninstallExtension(id)
  const impactQuery = useExtensionImpact(impactOpen ? id : undefined)
  const impact = impactQuery.data

  const installed = data.installed
  const currentVersion = data.installedVersion
  const versions = data.versions.map((v) => v.version)
  const otherVersions = versions.filter((v) => v !== currentVersion)
  const lowerVersions = otherVersions.filter(
    (v) => versions.indexOf(v) > -1 && v < (currentVersion ?? '')
  )
  const busy =
    install.isPending ||
    enable.isPending ||
    disable.isPending ||
    upgrade.isPending ||
    rollback.isPending ||
    uninstall.isPending

  const run = async (fn: () => Promise<unknown>, successText: string) => {
    try {
      await fn()
      message.success(successText)
    } catch (e) {
      message.error(parseApiError(e))
    }
  }

  const openImpact = (mode: { force: boolean; purge: boolean }) => {
    setUninstallMode(mode)
    setImpactOpen(true)
  }

  return (
    <Card title="生命周期" className={styles.card}>
      {installed ? (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Descriptions column={3} size="small">
            <Descriptions.Item label="已安装版本">
              <Tag color={data.installedStatus === 'enabled' ? 'green' : 'default'}>
                {currentVersion} · {data.installedStatus === 'enabled' ? '已启用' : '已停用'}
              </Tag>
            </Descriptions.Item>
          </Descriptions>
          <div className={styles.row}>
            {data.installedStatus === 'enabled' ? (
              <Button
                loading={disable.isPending}
                onClick={() =>
                  run(async () => {
                    await disable.mutateAsync()
                  }, '扩展已停用：新事件不再生效，历史数据依旧可读')
                }
              >
                停用
              </Button>
            ) : (
              <Button
                type="primary"
                loading={enable.isPending}
                onClick={() =>
                  run(async () => {
                    await enable.mutateAsync()
                  }, '扩展已重新启用')
                }
              >
                启用
              </Button>
            )}
            <span>
              <Select
                placeholder="升级目标版本"
                style={{ width: 180 }}
                value={upgradeTarget}
                onChange={setUpgradeTarget}
                options={otherVersions.map((v) => ({ value: v, label: `升级到 ${v}` }))}
              />
              <Button
                style={{ marginLeft: 8 }}
                disabled={!upgradeTarget}
                loading={upgrade.isPending}
                onClick={() =>
                  run(async () => {
                    await upgrade.mutateAsync(upgradeTarget as string)
                  }, `已升级到 ${upgradeTarget}（含数据迁移）`)
                }
              >
                升级
              </Button>
            </span>
            <span>
              <Select
                placeholder="回滚目标版本"
                style={{ width: 180 }}
                value={rollbackTarget}
                onChange={setRollbackTarget}
                options={(lowerVersions.length > 0 ? lowerVersions : otherVersions).map((v) => ({
                  value: v,
                  label: `回滚到 ${v}`
                }))}
              />
              <Button
                style={{ marginLeft: 8 }}
                disabled={!rollbackTarget}
                loading={rollback.isPending}
                onClick={() =>
                  run(async () => {
                    await rollback.mutateAsync(rollbackTarget)
                  }, `已回滚到 ${rollbackTarget}`)
                }
              >
                回滚
              </Button>
            </span>
            <Button danger onClick={() => openImpact({ force: false, purge: false })}>
              卸载
            </Button>
            <Button danger onClick={() => openImpact({ force: false, purge: true })}>
              卸载并清除版本数据
            </Button>
          </div>
        </Space>
      ) : (
        <div className={styles.row}>
          <Select
            placeholder="选择安装版本"
            style={{ width: 200 }}
            value={installVersion}
            onChange={setInstallVersion}
            options={versions.map((v) => ({ value: v, label: v }))}
          />
          <Button
            type="primary"
            disabled={!installVersion}
            loading={install.isPending}
            onClick={() =>
              run(async () => {
                await install.mutateAsync(installVersion as string)
              }, `扩展已安装并启用（${installVersion}）`)
            }
          >
            安装
          </Button>
          {versions.length === 0 && (
            <span style={{ color: t.textTertiary }}>尚未注册任何版本，无法安装</span>
          )}
        </div>
      )}

      <Modal
        title={`卸载前影响范围预览 · ${data.name}`}
        open={impactOpen}
        onCancel={() => setImpactOpen(false)}
        okText={impact && impact.dependents.length > 0 ? '停用依赖方并卸载' : '确认卸载'}
        cancelText="取消"
        okButtonProps={{ danger: true, loading: busy }}
        onOk={() => {
          const mode = uninstallMode ?? { force: false, purge: false }
          run(async () => {
            await uninstall.mutateAsync({
              force: mode.force || (impact?.dependents.length ?? 0) > 0,
              purge: mode.purge
            })
          }, '扩展已卸载：历史运行状态数据依旧保留').then(() => setImpactOpen(false))
        }}
      >
        {impactQuery.isLoading && <div>加载影响范围…</div>}
        {impactQuery.error && (
          <Alert type="error" showIcon message={parseApiError(impactQuery.error)} />
        )}
        {impact && (
          <Space direction="vertical" size={12} style={{ width: '100%' }}>
            <Alert
              type={impact.dependents.length > 0 ? 'warning' : 'info'}
              showIcon
              message={
                impact.dependents.length > 0
                  ? `以下扩展依赖它：${impact.dependents.join('、')}。卸载将自动停用这些依赖方。`
                  : '没有其他扩展依赖它，可以安全卸载。'
              }
            />
            <div>
              <strong>它依赖谁</strong>
              {impact.dependencies.length === 0 ? (
                <div style={{ color: t.textTertiary }}>未声明依赖</div>
              ) : (
                <ul style={{ margin: '6px 0 0', paddingLeft: 20 }}>
                  {impact.dependencies.map((d) => (
                    <li key={d.name}>
                      {d.name}@{d.range}
                      {d.optional ? '（可选）' : ''}：
                      {d.satisfied ? (
                        <Tag color="green">已满足{d.actual ? `（${d.actual}）` : ''}</Tag>
                      ) : (
                        <Tag color="red">未满足{d.actual ? `（${d.actual}）` : ''}</Tag>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div>
              <strong>新增 stateSchemas</strong>
              {impact.newStateSchemas.length === 0 ? (
                <div style={{ color: t.textTertiary }}>无新增</div>
              ) : (
                <div style={{ marginTop: 6 }}>
                  {impact.newStateSchemas.map((s) => (
                    <Tag key={s}>{s}</Tag>
                  ))}
                </div>
              )}
            </div>
            <div>
              <strong>权限声明</strong>
              {impact.permissions.length === 0 ? (
                <div style={{ color: t.textTertiary }}>未声明权限</div>
              ) : (
                <div style={{ marginTop: 6 }}>
                  {impact.permissions.map((p) => (
                    <Tag key={`${p.permission}:${p.scope}`}>
                      {p.permission}:{p.scope}（{p.actions.join('/')}）
                    </Tag>
                  ))}
                </div>
              )}
            </div>
          </Space>
        )}
      </Modal>
    </Card>
  )
}
