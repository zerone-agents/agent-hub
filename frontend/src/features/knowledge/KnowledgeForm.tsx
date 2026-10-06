import { useEffect, useMemo, useState, useRef } from "react";
import { useTranslation } from 'react-i18next'
// 模块级纯函数/Rule：直调 i18next
import i18next from '@/i18n'
import { getAccessToken, parseApiError } from '@/api/client'
import { queryClient } from '@/lib/query-client'
import { useAuthStore } from '@/stores/auth'
import { Link } from 'react-router'
import {
  Alert,
  Button,
  Collapse,
  Divider,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Col,
  Select,
  Switch,
  Tag,
  Typography,
} from "antd";
import { usePrimaryButtonStyle } from "@/components/PrimaryButton";
import type { Rule } from "antd/es/form";
import { LockKeyIcon } from "@phosphor-icons/react";
import { useCreateKnowledge, useUpdateKnowledge } from "@/queries/useKnowledge";
import { useProviders, useSyncProviderMultiRAG } from "@/queries/useProviders";
import { useMultiragModels } from "@/queries/useMultirag";
import type { KnowledgeDataset, DatasetFormInput, KnowledgeWriteOwner } from "@/api/knowledge";
import {
  buildEmbeddingCandidates,
  buildLayoutCandidates,
  decodeCandidateValue,
  isAllowedLayoutSelection,
  retainLayoutCandidates,
  type CandidateGroup,
} from "./candidates";
import LayoutModelHint from './LayoutModelHint';
import { datasetGraphFields, mergeDatasetGraphFields, datasetGraphDelta, type DatasetGraphFields } from './datasetGraphConfig';

export interface DatasetFormValues extends DatasetGraphFields {
  name: string;
  description: string;
  permission: string;
  parser_id: string;
  embd_id: string;
  layout_recognize: string;
  chunk_token_num: number;
  delimiter: string;
  enable_children: boolean;
  children_delimiter: string;
  image_table_context_window: number;
  auto_keywords: number;
  auto_questions: number;
  toc_extraction: boolean;
  html4excel: boolean;
  mineru_parse_method: string;
  mineru_lang: string;
  mineru_formula_enable: boolean;
  mineru_table_enable: boolean;
  parser_config_extra: string;
}

const LAYOUT_OPTIONS = [
  { label: "DeepDOC", value: "DeepDOC" },
  { label: "Plain Text", value: "Plain Text" },
];

// layoutIsMinerU returns true when the (possibly encoded) layout_recognize
// form value refers to MinerU — either a multirag factory ("MinerU") or a
// local provider model_id ("mineru"). Legacy raw values are handled too.
function layoutIsMinerU(value: string | undefined | null): boolean {
  if (!value) return false;
  const decoded = decodeCandidateValue(value);
  const raw = (decoded?.rawValue ?? value).toLowerCase();
  return raw === "mineru" || raw.endsWith("@mineru");
}

// buildRawToValueMap indexes candidate options by their rawValue so a saved
// (raw) embd_id / layout_recognize can be matched back to its encoded option.
export function buildRawToValueMap(
  groups: CandidateGroup[],
): Map<string, string> {
  const map = new Map<string, string>();
  for (const g of groups) {
    for (const opt of g.options) {
      if (!map.has(opt.rawValue)) map.set(opt.rawValue, opt.value);
    }
  }
  return map;
}

// antd Select option-group shape. Extra CandidateOption metadata is dropped —
// it is recovered at submit time via decodeCandidateValue.
export interface SelectOptionGroup {
  label: string;
  options: { label: string; value: string; disabled?: boolean }[];
}

// 分组 label 是 i18n 资源 key（candidates.ts 构造期写入），此处直调 i18next
// 翻译（模块级纯函数拿不到 hook t；调用方的 useMemo 已依赖 t，语言切换
// 会触发重算）。选项 label 是动态数据（模型名等），不经 t()。
export function groupsToAntdOptions(
  groups: CandidateGroup[],
  translate = i18next.t,
): SelectOptionGroup[] {
  return groups.map((g) => ({
    label: translate(g.label),
    options: g.options.map((o) => ({ label: o.unavailable ? translate('knowledge.form.layoutRetainedLabel', { saved: o.label }) : o.label, value: o.value, ...(o.disabled ? { disabled: true } : {}) })),
  }));
}


const MINERU_LANG_OPTIONS = [
  { label: "English", value: "English" },
  { label: "Chinese", value: "Chinese" },
  { label: "Traditional Chinese", value: "Traditional Chinese" },
  { label: "Japanese", value: "Japanese" },
  { label: "Korean", value: "Korean" },
];

const KNOWN_PARSER_CONFIG_KEYS = new Set([
  "layout_recognize",
  "chunk_token_num",
  "delimiter",
  "parent_child",
  "enable_children",
  "children_delimiter",
  "image_table_context_window",
  "image_context_size",
  "table_context_size",
  "auto_keywords",
  "auto_questions",
  "toc_extraction",
  "html4excel",
  "mineru_parse_method",
  "mineru_lang",
  "mineru_formula_enable",
  "mineru_table_enable",
]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown, fallback: string): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
  return fallback;
}

function numberValue(value: unknown, fallback: number): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function booleanValue(value: unknown, fallback: boolean): boolean {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    if (normalized === "true" || normalized === "1") return true;
    if (normalized === "false" || normalized === "0") return false;
  }
  return fallback;
}

function extractExtraParserConfig(
  config: Record<string, unknown>,
): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(config).filter(
      ([key]) => !KNOWN_PARSER_CONFIG_KEYS.has(key),
    ),
  );
}

// decodeRefValue strips the candidate source prefix (`builtin:` / `multirag:` /
// `local:<pid>:`) added by the merged-candidate Select. Values that aren't
// encoded (e.g. legacy raw ids, or the saved-value fallback) pass through
// unchanged so the shared helper keeps working for callers that don't use
// the candidate UI (e.g. KnowledgeSettingsPage).
function decodeRefValue(value: string | undefined | null): string {
  if (value == null) return "";
  const decoded = decodeCandidateValue(value);
  return decoded?.rawValue ?? value;
}

function parseExtraParserConfig(
  value: string | undefined,
): Record<string, unknown> {
  const text = (value ?? "").trim();
  if (text === "") return {};
  const parsed = JSON.parse(text) as unknown;
  if (!isRecord(parsed)) {
    throw new Error(i18next.t('knowledge.form.advJsonNotObject'));
  }
  return parsed;
}

// 选项 label 存 i18n key，使用处 map t()（模块级拿不到 hook；ThemeControls 同款约定）
const MINERU_PARSE_METHOD_OPTIONS = [
  { label: 'knowledge.form.embedAuto', value: "auto" },
  { label: 'knowledge.form.embedTxt', value: "txt" },
  { label: "OCR", value: "ocr" },
];
const PARSER_OPTIONS = [
  { label: 'knowledge.form.parserGeneric', value: "naive" },
  { label: 'knowledge.form.parserQa', value: "qa" },
  { label: 'knowledge.form.parserPaper', value: "paper" },
  { label: 'knowledge.form.parserBook', value: "book" },
  { label: 'knowledge.form.parserLaws', value: "laws" },
  { label: 'knowledge.form.parserManual', value: "manual" },
  { label: 'knowledge.form.parserTable', value: "table" },
  { label: 'knowledge.form.parserPresentation', value: "presentation" },
  { label: 'knowledge.form.parserPicture', value: "picture" },
  { label: 'knowledge.form.parserOne', value: "one" },
  { label: 'knowledge.form.parserEmail', value: "email" },
  { label: 'knowledge.form.parserResume', value: "resume" },
  { label: 'knowledge.form.parserTag', value: "tag" },
];

/** Advanced parser_config must be a JSON object (or empty). */
const parserConfigExtraRule: Rule = {
  validator: (_rule, value: string) => {
    const text = (value).trim();
    if (text === "") return Promise.resolve();
    try {
      parseExtraParserConfig(text);
      return Promise.resolve();
    } catch (error) {
      return Promise.reject(
        error instanceof Error ? error : new Error(i18next.t('knowledge.form.advJsonInvalid')),
      );
    }
  },
};

export function datasetToFormValues(
  ds?: KnowledgeDataset | null,
): DatasetFormValues {
  const config = ds?.parser_config ?? {};
  const extraConfig = extractExtraParserConfig(config);
  const parentChildConfig = isRecord(config.parent_child)
    ? config.parent_child
    : {};
  const contextWindow = numberValue(
    config.image_table_context_window ??
      config.image_context_size ??
      config.table_context_size,
    0,
  );
  const savedLayout = stringValue(config.layout_recognize, "DeepDOC");
  return {
    ...datasetGraphFields(config.graphrag),
    name: ds?.name ?? "",
    description: ds?.description ?? "",
    permission: ds?.permission ?? "me",
    parser_id: ds?.parser_id ?? "naive",
    embd_id: ds?.embd_id ?? "",
    // A real model name may itself start with a candidate source prefix.
    layout_recognize: /^(builtin:|multirag:|local:)/.test(savedLayout) ? `multirag:${savedLayout}` : savedLayout,
    chunk_token_num: numberValue(config.chunk_token_num, 512),
    delimiter: stringValue(config.delimiter, "\n!?。；！？"),
    enable_children: booleanValue(
      config.enable_children ?? parentChildConfig.use_parent_child,
      false,
    ),
    children_delimiter: stringValue(
      config.children_delimiter ?? parentChildConfig.children_delimiter,
      "\n",
    ),
    image_table_context_window: contextWindow,
    auto_keywords: numberValue(config.auto_keywords, 0),
    auto_questions: numberValue(config.auto_questions, 0),
    toc_extraction: booleanValue(config.toc_extraction, false),
    html4excel: booleanValue(config.html4excel, false),
    mineru_parse_method: stringValue(config.mineru_parse_method, "auto"),
    mineru_lang: stringValue(config.mineru_lang, "English"),
    mineru_formula_enable: booleanValue(config.mineru_formula_enable, true),
    mineru_table_enable: booleanValue(config.mineru_table_enable, true),
    parser_config_extra:
      Object.keys(extraConfig).length > 0
        ? JSON.stringify(extraConfig, null, 2)
        : "",
  };
}

export function formValuesToInput(
  values: DatasetFormValues,
  { includeEmbeddingModel = true, original }: { includeEmbeddingModel?: boolean; original?: KnowledgeDataset } = {},
): DatasetFormInput {
  const contextWindow = numberValue(values.image_table_context_window, 0);
  const parserConfig: Record<string, unknown> = {
    ...mergeDatasetGraphFields(parseExtraParserConfig(values.parser_config_extra), values),
    layout_recognize: decodeRefValue(values.layout_recognize) || "DeepDOC",
    chunk_token_num: numberValue(values.chunk_token_num, 512),
    delimiter: values.delimiter,
    enable_children: booleanValue(values.enable_children, false),
    image_table_context_window: contextWindow,
    image_context_size: contextWindow,
    table_context_size: contextWindow,
    auto_keywords: numberValue(values.auto_keywords, 0),
    auto_questions: numberValue(values.auto_questions, 0),
    toc_extraction: booleanValue(values.toc_extraction, false),
    html4excel: booleanValue(values.html4excel, false),
    mineru_parse_method: values.mineru_parse_method,
    mineru_lang: values.mineru_lang,
    mineru_formula_enable: booleanValue(values.mineru_formula_enable, true),
    mineru_table_enable: booleanValue(values.mineru_table_enable, true),
  };
  if (
    booleanValue(values.enable_children, false) ||
    (values.children_delimiter).trim()
  ) {
    parserConfig.children_delimiter = values.children_delimiter;
  }
  const input: DatasetFormInput = {
    name: values.name.trim(),
    description: values.description,
    permission: values.permission,
    parser_id: values.parser_id,
    parser_config: parserConfig,
  };
  if (includeEmbeddingModel) {
    const embeddingModel = decodeRefValue(values.embd_id).trim();
    // MultiRAG selects the tenant's default embedding model when omitted.
    // Sending an empty model also makes post-create readback expect an empty
    // value even though MultiRAG has persisted its actual default model.
    if (embeddingModel) input.embd_id = embeddingModel;
  }
  if (original) {
    // Compare against the same hydration defaults; an unrelated edit must not
    // install defaults, change parser mode, or normalize legacy extension data.
    const baseline = formValuesToInput(datasetToFormValues(original));
    const changes: DatasetFormInput = {};
    for (const key of ["name", "description", "permission", "parser_id", "embd_id"] as const) {
      if (input[key] !== undefined && input[key] !== baseline[key]) changes[key] = input[key];
    }
    const configChanges = Object.fromEntries(Object.entries(parserConfig).filter(
      ([key, value]) => JSON.stringify(value) !== JSON.stringify(baseline.parser_config?.[key]),
    ));
    if (isRecord(configChanges.graphrag) && isRecord(baseline.parser_config?.graphrag)) {
      const delta = datasetGraphDelta(configChanges.graphrag, baseline.parser_config.graphrag);
      if (Object.keys(delta).length) configChanges.graphrag = delta;
      else delete configChanges.graphrag;
    }
    if (Object.keys(configChanges).length) changes.parser_config = configChanges;
    return changes;
  }
  return input;
}

function ParserConfigFields({
  layoutOptions,
  layoutLoading,
  layoutGroups,
  chunkTokenMax = 8192,
}: {
  layoutOptions?: SelectOptionGroup[];
  layoutLoading?: boolean;
  layoutGroups?: CandidateGroup[];
  chunkTokenMax?: number;
}) {
  const { t } = useTranslation()
  const watchedLayout: unknown = Form.useWatch('layout_recognize');
  const selectedLayout = typeof watchedLayout === 'string' ? watchedLayout : undefined;
  const layoutKind = layoutGroups?.flatMap(group => group.options).find(option => option.value === selectedLayout || option.rawValue === selectedLayout)?.kind;
  return (
    <>
      <Divider titlePlacement="left" plain>
        {t('knowledge.form.parseSection')}
      </Divider>
      <Typography.Paragraph type="secondary" style={{ marginTop: -6 }}>
        {t('knowledge.form.parseHint')}
      </Typography.Paragraph>
      <Row gutter={12}>
        <Col xs={24} md={12}>
          <Form.Item
            label={t('knowledge.form.parseLayout')}
            name="layout_recognize"
            extra={layoutOptions ? <LayoutModelHint kind={layoutKind} value={decodeCandidateValue(selectedLayout)?.rawValue ?? selectedLayout} /> : undefined}
          >
            {layoutOptions ? (
              <Select
                options={layoutOptions}
                loading={layoutLoading}
                placeholder={t('knowledge.form.parseLayoutPh')}
                showSearch={{ optionFilterProp: 'label' }}
              />
            ) : (
              <Select options={LAYOUT_OPTIONS} />
            )}
          </Form.Item>
        </Col>
        <Col xs={24} md={12}>
          <Form.Item label={t('knowledge.form.chunkToken')} name="chunk_token_num" rules={[{ type: "number", min: 1, max: chunkTokenMax }]} >
            <InputNumber min={1} max={chunkTokenMax} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
      </Row>
      <Form.Item label={t('knowledge.form.delimiter')} name="delimiter">
        <Input placeholder={t('knowledge.form.delimiterPh')} />
      </Form.Item>

      <Collapse ghost items={[{
        key: 'enhancement', label: t('knowledge.form.enhancementSection'), forceRender: true,
        children: <><Typography.Paragraph type="secondary">{t('knowledge.form.enhancementHint')}</Typography.Paragraph>
      <Form.Item
        noStyle
        shouldUpdate={(prev, current) =>
          (prev as DatasetFormValues).enable_children !== (current as DatasetFormValues).enable_children
        }
      >
        {({ getFieldValue }) => (
          <Row gutter={12}>
            <Col xs={24} md={12}>
              <Form.Item
                label={t('knowledge.form.childChunks')}
                name="enable_children"
                valuePropName="checked"
              >
                <Switch />
              </Form.Item>
            </Col>
            <Col xs={24} md={12}>
              <Form.Item label={t('knowledge.form.childDelimiter')} name="children_delimiter">
                <Input
                  disabled={!getFieldValue("enable_children") || undefined}
                  placeholder={t('knowledge.form.childDelimiterPh')}
                />
              </Form.Item>
            </Col>
          </Row>
        )}
      </Form.Item>

      <Row gutter={12}>
        <Col xs={24} md={8}>
          <Form.Item label={t('knowledge.form.imgTableCtx')} name="image_table_context_window">
            <InputNumber min={0} max={20} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
        <Col xs={24} md={8}>
          <Form.Item label={t('knowledge.form.autoKeywords')} name="auto_keywords">
            <InputNumber min={0} max={30} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
        <Col xs={24} md={8}>
          <Form.Item label={t('knowledge.form.autoQuestions')} name="auto_questions">
            <InputNumber min={0} max={30} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
      </Row>
      <Row gutter={12}>
        <Col xs={24} md={12}>
          <Form.Item
            label={t('knowledge.form.tocExtract')}
            name="toc_extraction"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
        </Col>
        <Col xs={24} md={12}>
          <Form.Item
            label={t('knowledge.form.excelHtml')}
            name="html4excel"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
        </Col>
      </Row>

      <Form.Item
        noStyle
        shouldUpdate={(prev, current) =>
          (prev as DatasetFormValues).layout_recognize !== (current as DatasetFormValues).layout_recognize
        }
      >
        {({ getFieldValue }) =>
          layoutIsMinerU(getFieldValue("layout_recognize") as string | undefined | null) ? (
            <>
              <Divider titlePlacement="left" plain>
                MinerU
              </Divider>
              <Row gutter={12}>
                <Col xs={24} md={12}>
                  <Form.Item label={t('knowledge.form.parseMethod')} name="mineru_parse_method">
                    <Select options={MINERU_PARSE_METHOD_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item label={t('knowledge.form.lang')} name="mineru_lang">
                    <Select options={MINERU_LANG_OPTIONS} />
                  </Form.Item>
                </Col>
              </Row>
              <Row gutter={12}>
                <Col xs={24} md={12}>
                  <Form.Item
                    label={t('knowledge.form.formula')}
                    name="mineru_formula_enable"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
                <Col xs={24} md={12}>
                  <Form.Item
                    label={t('knowledge.form.tableReco')}
                    name="mineru_table_enable"
                    valuePropName="checked"
                  >
                    <Switch />
                  </Form.Item>
                </Col>
              </Row>
            </>
          ) : null
        }
      </Form.Item>
        </>,
      }]} />

      <Collapse ghost items={[{
        key: 'graphrag', label: t('knowledge.form.graphSection'), forceRender: true,
        children: <Form.Item noStyle shouldUpdate={(before, after) => (before as DatasetFormValues).graphrag_compatible !== (after as DatasetFormValues).graphrag_compatible}>
          {({ getFieldValue }) => getFieldValue('graphrag_compatible') ? <>
            <Typography.Paragraph type="secondary">{t('knowledge.form.graphHint')}</Typography.Paragraph>
            <Form.Item name="graphrag_enabled" label={t('knowledge.form.graphEnabled')} valuePropName="checked"><Switch /></Form.Item>
            <Row gutter={12}>
              <Col xs={24} md={12}><Form.Item name="graphrag_method" label={t('knowledge.form.graphMethod')}>
                <Select options={[{ value: 'light', label: t('knowledge.form.graphLight') }, { value: 'general', label: t('knowledge.form.graphGeneral') }]} />
              </Form.Item></Col>
              <Col xs={24} md={12}><Form.Item name="graphrag_entity_types" label={t('knowledge.form.graphEntityTypes')} extra={t('knowledge.form.graphEntityHint')}>
                <Select mode="tags" allowClear tokenSeparators={[',']} options={['organization', 'person', 'geo', 'event', 'category'].map((value) => ({ label: value, value }))} />
              </Form.Item></Col>
            </Row>
            <Row gutter={12}>
              <Col xs={24} md={12}><Form.Item name="graphrag_community" label={t('knowledge.form.graphCommunity')} valuePropName="checked"><Switch /></Form.Item></Col>
              <Col xs={24} md={12}><Form.Item name="graphrag_resolution" label={t('knowledge.form.graphResolution')} valuePropName="checked"><Switch /></Form.Item></Col>
            </Row>
          </> : <Alert type="info" showIcon title={t('knowledge.form.graphAdvancedOnly')} />}
        </Form.Item>,
      }]} />

      <Collapse
        ghost
        items={[
          {
            key: "advanced",
            forceRender: true,
            label: t('knowledge.form.advJson'),
            children: (
              <Form.Item
                name="parser_config_extra"
                rules={[parserConfigExtraRule]}
                tooltip={t('knowledge.form.advJsonTip')}
              >
                <Input.TextArea
                  rows={5}
                  placeholder='{"raptor": {"use_raptor": true}}'
                  style={{ fontFamily: "monospace" }}
                />
              </Form.Item>
            ),
          },
        ]}
      />
    </>
  );
}

/** Dataset form fields — rendered inside a parent-owned `Form`. */
export function DatasetFields({
  nameDisabled = false,
  embeddingOptions,
  embeddingLoading,
  embeddingLocked = false,
  embeddingChunkCount = 0,
  layoutOptions,
  layoutLoading,
  layoutGroups,
  chunkTokenMax = 8192,
  savedParser,
  preserveParserMode = false,
  compact = false,
}: {
  savedParser?: string;
  preserveParserMode?: boolean;
  compact?: boolean;
  nameDisabled?: boolean;
  // When provided, embd_id renders as a merged-candidate Select instead of a
  // free-form Input. Omit only for callers that intentionally need raw model IDs.
  embeddingOptions?: SelectOptionGroup[];
  embeddingLoading?: boolean;
  embeddingLocked?: boolean;
  embeddingChunkCount?: number;
  layoutOptions?: SelectOptionGroup[];
  layoutLoading?: boolean;
  layoutGroups?: CandidateGroup[];
  chunkTokenMax?: number;
}) {
  const { t } = useTranslation()
  const parser: unknown = Form.useWatch("parser_id");
  const parserOptions = PARSER_OPTIONS.map((o) => ({ ...o, label: t(o.label) }));
  if (savedParser && !parserOptions.some((option) => option.value === savedParser)) {
    parserOptions.unshift({ label: t('knowledge.form.retainedParser', { value: savedParser }), value: savedParser });
  }
  const embeddingField = (
    <Form.Item
      label={t('knowledge.form.embedModel')}
      name="embd_id"
      extra={
        embeddingLocked ? (
          <Typography.Text type="secondary">{t('knowledge.form.embedLockedNote', { n: embeddingChunkCount })}</Typography.Text>
        ) : embeddingOptions?.every((group) => group.options.length === 0) ? t('knowledge.form.noEmbeddingOptions') : undefined
      }
    >
      {embeddingLocked ? (
        <Input readOnly suffix={<Tag variant="filled" icon={<LockKeyIcon size={12} />}>{t('knowledge.form.lockedBadge')}</Tag>} />
      ) : embeddingOptions ? (
        <Select options={embeddingOptions} loading={embeddingLoading} placeholder={t('knowledge.form.embedPh')} showSearch={{ optionFilterProp: 'label' }} />
      ) : (
        <Input placeholder={t('knowledge.form.embedIdPh')} />
      )}
    </Form.Item>
  );
  const advancedFields = <ParserConfigFields layoutOptions={layoutOptions} layoutLoading={layoutLoading} layoutGroups={layoutGroups} chunkTokenMax={chunkTokenMax} />;
  return (
    <>
      <Divider titlePlacement="left" plain>{t('knowledge.form.identitySection')}</Divider>
      <Form.Item
        label={t('knowledge.form.name')}
        name="name"
        rules={[
          { required: true, message: t('knowledge.form.nameRequired') },
          { max: 128, message: t('knowledge.form.nameMax') },
        ]}
      >
        <Input placeholder={t('knowledge.form.namePh')} disabled={nameDisabled || undefined} />
      </Form.Item>
      <Form.Item label={t('knowledge.form.desc')} name="description">
        <Input.TextArea rows={2} placeholder={t('knowledge.form.descPh')} maxLength={512} />
      </Form.Item>
      <Form.Item name="permission" hidden><Input /></Form.Item>
      <Divider titlePlacement="left" plain>{t('knowledge.form.indexSection')}</Divider>
      <Form.Item label={t('knowledge.form.parserLabel')} name="parser_id" extra={preserveParserMode ? t('knowledge.form.preservedMode') : parser === 'tag' ? t('knowledge.form.tagRestriction') : t('knowledge.form.parserScopeHint')}>
        <Select disabled={preserveParserMode || undefined} options={parserOptions} showSearch={{ optionFilterProp: 'label' }} />
      </Form.Item>
      {embeddingField}
      {compact ? <Collapse ghost items={[{
        key: 'advanced-dataset', label: t('knowledge.form.advancedSettings'), forceRender: true, children: advancedFields,
      }]} /> : advancedFields}
    </>
  );
}

interface KnowledgeFormOperation {
  ownerId: string | undefined;
  cancelled: boolean;
  controller: AbortController;
  guard: KnowledgeWriteOwner;
}

interface KnowledgeFormProps {
  open: boolean;
  editing: KnowledgeDataset | null;
  onClose: () => void;
  onCreated?: (id: string) => void;
  onResume?: () => void;
}

export default function KnowledgeForm({
  open,
  editing,
  onClose,
  onCreated,
  onResume,
}: KnowledgeFormProps) {
  const { t } = useTranslation()



  const [form] = Form.useForm<DatasetFormValues>();
  const watchedLayout: unknown = Form.useWatch('layout_recognize', form);
  const selectedLayout = typeof watchedLayout === 'string' ? watchedLayout : undefined;
  const [modal, contextHolder] = Modal.useModal();
  const [syncing, setSyncing] = useState(false);
  const ownerId = useAuthStore((state) => state.user?.id);
  const [recovery, setRecovery] = useState<{ ownerId: string | undefined; id: string; values: DatasetFormValues } | null>(null);
  const activeRecovery = recovery?.ownerId === ownerId ? recovery : null;
  const createdDatasetId = activeRecovery?.id ?? "";
  const formOwner = useRef({ ownerId, token: getAccessToken(), valid: true });
  const hydratedTarget = useRef<{ editing: KnowledgeDataset | null; ownerId: string | undefined } | null>(null);
  const operation = useRef<KnowledgeFormOperation | null>(null);
  const [working, setWorking] = useState(false);
  const [ownershipInterrupted, setOwnershipInterrupted] = useState(false);
  const [submitError, setSubmitError] = useState('');
  useEffect(() => {
    const cancel = () => {
      const active = operation.current;
      if (!active) return;
      active.cancelled = true;
      active.controller.abort();
      setOwnershipInterrupted(true);
    };
    const unsubscribeUser = useAuthStore.subscribe((next, previous) => { if (next.user?.id !== previous.user?.id) { formOwner.current.valid = false; cancel(); } });
    const unsubscribeCache = queryClient.getQueryCache().subscribe((event) => {
      const key = event.query.queryKey as readonly unknown[];
      if (event.type === 'removed' && key[0] === 'userinfo') { formOwner.current.valid = false; cancel(); }
    });
    return () => {
      if (operation.current) { operation.current.cancelled = true; operation.current.controller.abort(); }
      unsubscribeUser(); unsubscribeCache();
    };
  }, []);
  const getOperationOwner = () => operation.current?.guard;
  const primaryStyle = usePrimaryButtonStyle();
  const createKnowledge = useCreateKnowledge({ getOwner: getOperationOwner });
  const updateKnowledge = useUpdateKnowledge({ getOwner: getOperationOwner });
  const syncProvider = useSyncProviderMultiRAG({ getOwner: getOperationOwner });

  const providers = useProviders();
  const multiragEmbedding = useMultiragModels("embedding");
  const multiragLayout = useMultiragModels("ocr");
  const multiragVision = useMultiragModels("image2text");

  const embeddingGroups = useMemo(
    () =>
      buildEmbeddingCandidates(
        multiragEmbedding.data ?? [],
        providers.data ?? [],
      ),
    [multiragEmbedding.data, providers.data],
  );
  const layoutGroups = useMemo(
    () => buildLayoutCandidates([...(multiragLayout.isError ? [] : multiragLayout.data ?? []), ...(multiragVision.isError ? [] : multiragVision.data ?? [])]),
    [multiragLayout.data, multiragLayout.isError, multiragVision.data, multiragVision.isError],
  );

  const embdRawToValue = useMemo(
    () => buildRawToValueMap(embeddingGroups),
    [embeddingGroups],
  );
  const layoutRawToValue = useMemo(
    () => buildRawToValueMap(layoutGroups),
    [layoutGroups],
  );
  const embeddingLocked = Boolean(editing && editing.chunk_num > 0);

  const submitting = createKnowledge.isPending || updateKnowledge.isPending;

  // Associate visible form values with the identity that hydrated them.
  useEffect(() => {
    if (!open) return;
    const previous = hydratedTarget.current;
    if (editing && previous?.editing === editing && previous.ownerId !== ownerId) {
      form.resetFields();
      form.setFieldsValue(datasetToFormValues(null));
      formOwner.current.valid = false;
      return;
    }
    form.setFieldsValue(!editing && activeRecovery ? activeRecovery.values : datasetToFormValues(editing));
    formOwner.current = { ownerId, token: getAccessToken(), valid: true };
    hydratedTarget.current = { editing, ownerId };
  }, [open, editing, form, activeRecovery, ownerId]);

  // Once candidate data is available, remap saved raw embd_id / layout values
  // to their encoded option values so the Select can show them as selected.
  // Values already encoded (or unknown raw values) are left untouched; the
  // latter surface via the synthetic "unavailable" option below.
  useEffect(() => {
    if (!open) return;
    const embd = form.getFieldValue("embd_id") as unknown;
    if (
      !embeddingLocked &&
      typeof embd === "string" &&
      embd &&
      embdRawToValue.has(embd)
    ) {
      form.setFieldValue("embd_id", embdRawToValue.get(embd));
    }
    const layout = form.getFieldValue("layout_recognize") as unknown;
    if (typeof layout === "string" && layout && layoutRawToValue.has(layout)) {
      form.setFieldValue("layout_recognize", layoutRawToValue.get(layout));
    }
  }, [open, embeddingLocked, embdRawToValue, layoutRawToValue, form]);

  // Saved-value fallback: if the persisted embd_id isn't offered by any
  // candidate, surface it as a synthetic "unavailable" option so the Select
  // can render the current value and prompt the user to re-pick.
  const embeddingOptions = useMemo<SelectOptionGroup[]>(() => {
    const saved = editing?.embd_id;
    if (typeof saved === "string" && saved && !embdRawToValue.has(saved)) {
      return [
        {
          label: t('knowledge.settings.currentValue'),
          options: [
            {
              label: t('knowledge.settings.modelUnavailable', { saved }),
              value: saved,
            },
          ],
        },
        ...groupsToAntdOptions(embeddingGroups),
      ];
    }
    return groupsToAntdOptions(embeddingGroups);
  }, [t, editing?.embd_id, embdRawToValue, embeddingGroups]);

  const layoutOptions = useMemo<SelectOptionGroup[]>(() => {
    const saved = editing?.parser_config.layout_recognize;
    return groupsToAntdOptions(retainLayoutCandidates(layoutGroups, typeof saved === 'string' ? saved : undefined, selectedLayout), t);
  }, [
    t,
    editing?.parser_config.layout_recognize,
    selectedLayout,
    layoutGroups,
  ]);

  const embeddingLoading = providers.isLoading || multiragEmbedding.isLoading;
  const layoutLoading = multiragLayout.isLoading || multiragVision.isLoading;

  const handleOk = async () => {
    if (operation.current) return;
    if (!formOwner.current.valid || formOwner.current.ownerId !== ownerId || useAuthStore.getState().user?.id !== ownerId || formOwner.current.token !== getAccessToken()) {
      setOwnershipInterrupted(true);
      return;
    }
    let token = getAccessToken();
    const controller = new AbortController();
    const currentIdentity = () => !current.cancelled && operation.current === current && useAuthStore.getState().user?.id === current.ownerId;
    const guard: KnowledgeWriteOwner = {
      signal: controller.signal,
      isCurrent: () => currentIdentity() && getAccessToken() === token,
      assertCurrent: (credentialRefresh = false) => {
        // Only the client's explicit refresh retry may adopt new credentials;
        // login/logout removes userinfo and invalidates the operation first.
        if (credentialRefresh && currentIdentity()) { token = getAccessToken(); formOwner.current.token = token; }
        if (!guard.isCurrent()) {
          current.cancelled = true; controller.abort();
          setOwnershipInterrupted(true);
          throw new Error(t('knowledge.form.ownerChanged'));
        }
      },
    };
    const current: KnowledgeFormOperation = { ownerId, controller, cancelled: false, guard };
    operation.current = current;
    setWorking(true);
    setOwnershipInterrupted(false);
    setSubmitError('');
    try {
    let values: DatasetFormValues;
    try {
      const validated = await form.validateFields();
      guard.assertCurrent();
      values = { ...(form.getFieldsValue(true) as DatasetFormValues), ...validated };
    } catch {
      return;
    }

    // Validate structured/JSON conflicts before any provider or dataset write.
    const input = formValuesToInput(values, {
      includeEmbeddingModel: !embeddingLocked,
      original: editing ?? undefined,
    });

    if (!isAllowedLayoutSelection(values.layout_recognize, layoutGroups, editing?.parser_config.layout_recognize as string | undefined)) {
      form.setFields([{ name: 'layout_recognize', errors: [t('knowledge.form.parseLayoutUnavailable')] }]);
      return;
    }

    // Decode the selected candidates to decide which providers need syncing.
    const embd = embeddingLocked ? null : decodeCandidateValue(values.embd_id);

    // Only embedding models may be synced on save. OCR parsers must already
    // have been verified and registered before appearing in this form.
    const modelMap = new Map<number, Set<string>>();
    const addTarget = (decoded: ReturnType<typeof decodeCandidateValue>) => {
      if (
        decoded?.source !== "local" ||
        !decoded.providerId ||
        !decoded.rawValue
      )
        return;
      let set = modelMap.get(decoded.providerId);
      if (!set) {
        set = new Set();
        modelMap.set(decoded.providerId, set);
      }
      set.add(decoded.rawValue);
    };
    addTarget(embd);
    const syncTargets = Array.from(modelMap.entries()).map(([id, models]) => ({
      id,
      modelIds: Array.from(models),
    }));

    // Sync each targeted provider before submitting. A sync failure aborts
    // the submit and keeps the modal open (no KB create/update attempted).
    if (syncTargets.length > 0) {
      setSyncing(true);
      try {
        for (const target of syncTargets) {
          try {
            guard.assertCurrent();
            await syncProvider.mutateAsync({
              id: target.id,
              verifyOnly: false,
              modelIds: target.modelIds,
            });
            guard.assertCurrent();
          } catch {
            return;
          }
        }
      } finally {
        if (operation.current === current) setSyncing(false);
      }
    }

    guard.assertCurrent();
    try {
      if (editing || createdDatasetId) {
        guard.assertCurrent();
        await updateKnowledge.mutateAsync({ id: editing?.id ?? createdDatasetId, data: input });
        guard.assertCurrent();
        if (!editing && createdDatasetId) onCreated?.(createdDatasetId);
      } else {
        guard.assertCurrent();
        const created = await createKnowledge.mutateAsync(input);
        guard.assertCurrent();
        if (created.id) onCreated?.(created.id);
      }
      if (!editing) setRecovery(null);
      onClose();
    } catch (error: unknown) {
      if (!guard.isCurrent()) return;
      const response = error as { response?: { data?: { code?: string; data?: { dataset?: { id?: string } } } } };
      if (response.response?.data?.code === "knowledge_configuration_pending") {
        const datasetId = response.response.data.data?.dataset?.id;
        if (datasetId) setRecovery({ ownerId: current.ownerId, id: datasetId, values: { ...(form.getFieldsValue(true) as DatasetFormValues) } });
      }
      // mutation hooks already surface a toast
    }
    } catch (error) { if (!guard.isCurrent()) setOwnershipInterrupted(true); else setSubmitError(parseApiError(error)); }
    finally {
      if (operation.current === current) { operation.current = null; setWorking(false); setSyncing(false); }
    }
  };

  const busy = submitting || syncing || working;
  const recoveryPending = Boolean(activeRecovery && !editing);
  const handleCancel = () => {
    if (busy) return;
    if (activeRecovery && !editing) {
      setRecovery({ ...activeRecovery, values: { ...(form.getFieldsValue(true) as DatasetFormValues) } });
      onClose();
      return;
    }
    let dirty = true;
    try {
      dirty = JSON.stringify(formValuesToInput(form.getFieldsValue(true) as DatasetFormValues)) !== JSON.stringify(formValuesToInput(datasetToFormValues(editing)));
    } catch { /* Invalid drafts also require an explicit discard decision. */ }
    if (!dirty) { setSubmitError(''); onClose(); return; }
    modal.confirm({
      title: t('knowledge.form.discardTitle'),
      content: t('knowledge.form.discardHint'),
      okText: t('knowledge.form.discardDraft'),
      cancelText: t('knowledge.form.keepEditing'),
      okButtonProps: { className: primaryStyle.root },
      onOk: () => { form.resetFields(); setSubmitError(''); onClose(); },
    });
  };

  return (
    <>{contextHolder}
    {!open && activeRecovery ? <Alert type="warning" showIcon title={t('knowledge.form.recoveryDeferred')} description={<>{t('knowledge.form.recoveryDeferredHint')} <Link to={`/knowledge/${activeRecovery.id}/settings`}>{t('knowledge.form.openCreatedDataset')}</Link></>} action={onResume ? <Button onClick={onResume}>{t('knowledge.form.resumeConfiguration')}</Button> : undefined} style={{ marginTop: 16 }} /> : null}
    <Modal
      title={editing ? t('knowledge.form.editTitle') : recoveryPending ? t('knowledge.form.resumeConfiguration') : t('knowledge.form.createTitle')}
      open={open}
      onOk={handleOk}
      onCancel={handleCancel}
      closable={!busy}
      mask={{ closable: !busy }}
      keyboard={!busy}
      cancelButtonProps={{ disabled: busy }}
      confirmLoading={submitting || syncing}
      okButtonProps={{ className: primaryStyle.root }}
      okText={editing || createdDatasetId ? t('knowledge.form.saveBtn') : t('knowledge.form.createBtn')}
      cancelText={recoveryPending ? t('knowledge.form.handleLater') : t('common.cancel')}
      destroyOnHidden
      width={760}
    >
      {ownershipInterrupted ? <Alert type="warning" showIcon title={t('knowledge.form.ownerChanged')} style={{ marginTop: 12 }} /> : null}
      {submitError ? <Alert type="error" showIcon title={submitError} style={{ marginTop: 12 }} /> : null}
      {providers.isError || multiragEmbedding.isError || multiragLayout.isError || multiragVision.isError ? <Alert type="warning" showIcon title={t('knowledge.form.candidatesFailed')} description={t('knowledge.form.candidatesFailedHint')} action={<Button onClick={() => { void providers.refetch(); void multiragEmbedding.refetch(); void multiragLayout.refetch(); void multiragVision.refetch(); }}>{t('knowledge.states.retry')}</Button>} style={{ marginTop: 12 }} /> : null}
      {recoveryPending ? <Alert type="warning" showIcon title={t('knowledge.form.configurationPending')} style={{ marginTop: 12 }} /> : null}
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        disabled={busy}
        onValuesChange={() => { if (submitError) setSubmitError(''); }}
        style={{ marginTop: 12 }}
      >
        <DatasetFields
          compact
          savedParser={editing?.parser_id}
          preserveParserMode={typeof editing?.pipeline_id === "string" && Boolean(editing.pipeline_id)}
          chunkTokenMax={editing || createdDatasetId ? 8192 : 2048}
          embeddingOptions={embeddingOptions}
          embeddingLoading={embeddingLoading}
          embeddingLocked={embeddingLocked}
          embeddingChunkCount={editing?.chunk_num ?? 0}
          layoutOptions={layoutOptions}
          layoutLoading={layoutLoading}
          layoutGroups={layoutGroups}
        />
      </Form>
    </Modal></>
  );
}
