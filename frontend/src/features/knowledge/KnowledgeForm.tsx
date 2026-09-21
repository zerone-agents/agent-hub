import { useEffect, useMemo, useState } from "react";
import { useTranslation } from 'react-i18next'
// 模块级纯函数/Rule：直调 i18next
import i18next from '@/i18n'
import {
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
import type { Rule } from "antd/es/form";
import { LockKeyIcon } from "@phosphor-icons/react";
import { useCreateKnowledge, useUpdateKnowledge } from "@/queries/useKnowledge";
import { useProviders, useSyncProviderMultiRAG } from "@/queries/useProviders";
import { useMultiragModels } from "@/queries/useMultirag";
import type { KnowledgeDataset, DatasetFormInput } from "@/api/knowledge";
import {
  buildEmbeddingCandidates,
  buildLayoutCandidates,
  decodeCandidateValue,
  type CandidateGroup,
} from "./candidates";

export interface DatasetFormValues {
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
  { label: "MinerU", value: "MinerU" },
  { label: "Docling", value: "Docling" },
  { label: "TCADP Parser", value: "TCADP Parser" },
];

// layoutIsMinerU returns true when the (possibly encoded) layout_recognize
// form value refers to MinerU — either a multirag factory ("MinerU") or a
// local provider model_id ("mineru"). Legacy raw values are handled too.
function layoutIsMinerU(value: string | undefined | null): boolean {
  if (!value) return false;
  const decoded = decodeCandidateValue(value);
  const raw = (decoded?.rawValue ?? value).toLowerCase();
  return raw === "mineru";
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
  options: { label: string; value: string }[];
}

// 分组 label 是 i18n 资源 key（candidates.ts 构造期写入），此处直调 i18next
// 翻译（模块级纯函数拿不到 hook t；调用方的 useMemo 已依赖 t，语言切换
// 会触发重算）。选项 label 是动态数据（模型名等），不经 t()。
export function groupsToAntdOptions(
  groups: CandidateGroup[],
): SelectOptionGroup[] {
  return groups.map((g) => ({
    label: i18next.t(g.label),
    options: g.options.map((o) => ({ label: o.label, value: o.value })),
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
const PERMISSION_OPTIONS = [
  { label: 'knowledge.form.permMe', value: "me" },
  { label: 'knowledge.form.permTeam', value: "team" },
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
  return {
    name: ds?.name ?? "",
    description: ds?.description ?? "",
    permission: ds?.permission ?? "me",
    parser_id: ds?.parser_id ?? "naive",
    embd_id: ds?.embd_id ?? "",
    layout_recognize: stringValue(config.layout_recognize, "DeepDOC"),
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
  { includeEmbeddingModel = true }: { includeEmbeddingModel?: boolean } = {},
): DatasetFormInput {
  const contextWindow = numberValue(values.image_table_context_window, 0);
  const parserConfig: Record<string, unknown> = {
    ...parseExtraParserConfig(values.parser_config_extra),
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
    input.embd_id = decodeRefValue(values.embd_id).trim();
  }
  return input;
}

function ParserConfigFields({
  layoutOptions,
  layoutLoading,
}: {
  layoutOptions?: SelectOptionGroup[];
  layoutLoading?: boolean;
}) {
  const { t } = useTranslation()
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
          <Form.Item label={t('knowledge.form.parseLayout')} name="layout_recognize">
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
          <Form.Item label={t('knowledge.form.chunkToken')} name="chunk_token_num">
            <InputNumber min={1} max={8192} style={{ width: "100%" }} />
          </Form.Item>
        </Col>
      </Row>
      <Form.Item label={t('knowledge.form.delimiter')} name="delimiter">
        <Input placeholder={t('knowledge.form.delimiterPh')} />
      </Form.Item>

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
                  disabled={!getFieldValue("enable_children")}
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

      <Collapse
        ghost
        items={[
          {
            key: "advanced",
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
}: {
  nameDisabled?: boolean;
  // When provided, embd_id renders as a merged-candidate Select instead of a
  // free-form Input. Omit only for callers that intentionally need raw model IDs.
  embeddingOptions?: SelectOptionGroup[];
  embeddingLoading?: boolean;
  embeddingLocked?: boolean;
  embeddingChunkCount?: number;
  layoutOptions?: SelectOptionGroup[];
  layoutLoading?: boolean;
}) {
  const { t } = useTranslation()
  return (
    <>
      <Form.Item
        label={t('knowledge.form.name')}
        name="name"
        rules={[
          { required: true, message: t('knowledge.form.nameRequired') },
          { max: 128, message: t('knowledge.form.nameMax') },
        ]}
      >
        <Input placeholder={t('knowledge.form.namePh')} disabled={nameDisabled} />
      </Form.Item>
      <Form.Item label={t('knowledge.form.desc')} name="description">
        <Input.TextArea rows={2} placeholder={t('knowledge.form.descPh')} maxLength={512} />
      </Form.Item>
      <Form.Item label={t('knowledge.form.permLabel')} name="permission">
        <Select options={PERMISSION_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} />
      </Form.Item>
      <Form.Item label={t('knowledge.form.parserLabel')} name="parser_id">
        <Select options={PARSER_OPTIONS.map((o) => ({ ...o, label: t(o.label) }))} showSearch={{ optionFilterProp: 'label' }} />
      </Form.Item>
      <Form.Item
        label={t('knowledge.form.embedModel')}
        name="embd_id"
        extra={
          embeddingLocked ? (
            <Typography.Text type="secondary">
              {t('knowledge.form.embedLockedNote', { n: embeddingChunkCount })}
            </Typography.Text>
          ) : undefined
        }
      >
        {embeddingLocked ? (
          <Input
            readOnly
            suffix={
              <Tag variant="filled" icon={<LockKeyIcon size={12} />}>
                {t('knowledge.form.lockedBadge')}
              </Tag>
            }
          />
        ) : embeddingOptions ? (
          <Select
            options={embeddingOptions}
            loading={embeddingLoading}
            placeholder={t('knowledge.form.embedPh')}
            showSearch={{ optionFilterProp: 'label' }}
          />
        ) : (
          <Input placeholder={t('knowledge.form.embedIdPh')} />
        )}
      </Form.Item>
      <ParserConfigFields
        layoutOptions={layoutOptions}
        layoutLoading={layoutLoading}
      />
    </>
  );
}

interface KnowledgeFormProps {
  open: boolean;
  editing: KnowledgeDataset | null;
  onClose: () => void;
}

export default function KnowledgeForm({
  open,
  editing,
  onClose,
}: KnowledgeFormProps) {
  const { t } = useTranslation()



  const [form] = Form.useForm<DatasetFormValues>();
  const [syncing, setSyncing] = useState(false);
  const createKnowledge = useCreateKnowledge();
  const updateKnowledge = useUpdateKnowledge();
  const syncProvider = useSyncProviderMultiRAG();

  const providers = useProviders();
  const multiragEmbedding = useMultiragModels("embedding");
  const multiragLayout = useMultiragModels("ocr");

  const embeddingGroups = useMemo(
    () =>
      buildEmbeddingCandidates(
        multiragEmbedding.data ?? [],
        providers.data ?? [],
      ),
    [multiragEmbedding.data, providers.data],
  );
  const layoutGroups = useMemo(
    () =>
      buildLayoutCandidates(multiragLayout.data ?? [], providers.data ?? []),
    [multiragLayout.data, providers.data],
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

  // Hydrate the form with raw saved values when it opens / the target changes.
  useEffect(() => {
    if (open) form.setFieldsValue(datasetToFormValues(editing));
  }, [open, editing, form]);

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
    if (typeof saved === "string" && saved && !layoutRawToValue.has(saved)) {
      return [
        {
          label: t('knowledge.settings.currentValue'),
          options: [
            {
              label: t('knowledge.settings.unavailable', { saved }),
              value: saved,
            },
          ],
        },
        ...groupsToAntdOptions(layoutGroups),
      ];
    }
    return groupsToAntdOptions(layoutGroups);
  }, [
    t,
    editing?.parser_config.layout_recognize,
    layoutRawToValue,
    layoutGroups,
  ]);

  const candidatesLoading =
    providers.isLoading ||
    multiragEmbedding.isLoading ||
    multiragLayout.isLoading;

  const handleOk = async () => {
    let values: DatasetFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }

    // Decode the selected candidates to decide which providers need syncing.
    const embd = embeddingLocked ? null : decodeCandidateValue(values.embd_id);
    const layout = decodeCandidateValue(values.layout_recognize);

    // Collect (providerId, modelId) pairs for local candidates. Dedup by
    // providerId + modelId — when the same provider supplies both embd
    // and layout via the same model, only sync it once.
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
    addTarget(layout);
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
            await syncProvider.mutateAsync({
              id: target.id,
              verifyOnly: false,
              modelIds: target.modelIds,
            });
          } catch {
            return;
          }
        }
      } finally {
        setSyncing(false);
      }
    }

    const input = formValuesToInput(values, {
      includeEmbeddingModel: !embeddingLocked,
    });
    try {
      if (editing) {
        await updateKnowledge.mutateAsync({ id: editing.id, data: input });
      } else {
        await createKnowledge.mutateAsync(input);
      }
      onClose();
    } catch {
      // mutation hooks already surface a toast
    }
  };

  return (
    <Modal
      title={editing ? t('knowledge.form.editTitle') : t('knowledge.form.createTitle')}
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={submitting || syncing}
      okText={editing ? t('knowledge.form.saveBtn') : t('knowledge.form.createBtn')}
      cancelText={t('common.cancel')}
      destroyOnHidden
      width={760}
    >
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        style={{ marginTop: 12 }}
      >
        <DatasetFields
          embeddingOptions={embeddingOptions}
          embeddingLoading={candidatesLoading}
          embeddingLocked={embeddingLocked}
          embeddingChunkCount={editing?.chunk_num ?? 0}
          layoutOptions={layoutOptions}
          layoutLoading={candidatesLoading}
        />
      </Form>
    </Modal>
  );
}
