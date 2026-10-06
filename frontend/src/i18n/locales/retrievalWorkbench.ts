import i18next from "@/i18n";

// A retrieval-owned namespace keeps this delivery independent of shared locale edits.
const en = {
  intro:
    "Test a real question, then check whether its sources support the answer.",
  scope: "Sources and metadata",
  metadata: "Metadata filter",
  mode: "Filter mode",
  disabled: "No filter",
  manual: "Manual conditions",
  auto: "Automatic",
  semi_auto: "Selected fields",
  autoHint:
    "The configured chat model derives conditions from your question. If it derives none, the selected document scope is kept.",
  semiHint:
    "The chat model uses only these fields. Choose an operator or let the model decide.",
  generatedExclusionHint:
    "The service generates these conditions internally. Negative comparisons on fields with multiple values may include documents you meant to exclude; this page cannot validate the generated conditions. Check the sources before relying on the results.",
  logic: "Match conditions",
  and: "All (AND)",
  or: "Any (OR)",
  field: "Field",
  operator: "Operator",
  value: "Value",
  valueType: "Value type",
  text: "Text / date",
  number: "Number",
  list: "Text list",
  add: "Add condition",
  remove: "Remove condition",
  modelOperator: "Model decides",
  advancedJSON: "Advanced JSON",
  invalid: "Check the filter: enter valid fields, operators and values.",
  filterDraftInvalid: "The filter draft is invalid. Fix it before testing.",
  unknownMetadataFields:
    "Some selected fields are not present in this dataset's metadata. Choose existing fields before testing.",
  metadataDirectoryRequired:
    "The metadata field directory must load successfully before selected-field inference can run. Retry or choose another filter mode.",
  invalidMetadataList:
    "Lists must contain at least one text, finite number, or boolean value. Null, objects, nested lists and non-finite numbers are invalid.",
  unsupportedMetadataList:
    "Only In list accepts array values. Your draft is kept; correct the operator before testing.",
  unsupportedMetadataExclusion:
    "Negative filters (Not in list, Does not equal, Does not contain) are unavailable: documents with multiple values may remain even when they contain an excluded value. Your draft is kept. Use In list or an explicit document selection.",
  listHint:
    "In list accepts text tags. To mix text, numbers and booleans, edit Advanced JSON; existing typed tags keep their types here. Not in list remains unavailable for multi-value metadata. Empty lists are invalid.",
  operators: {
    equals: "Equals",
    notEquals: "Does not equal (unavailable)",
    contains: "Contains",
    notContains: "Does not contain (unavailable)",
    starts: "Starts with",
    ends: "Ends with",
    empty: "Empty",
    notEmpty: "Not empty",
    in: "In list",
    notIn: "Not in list (unavailable)",
  },
  keysError:
    "Metadata fields could not be loaded. Manual conditions can still use known fields; selected-field inference requires a successful retry.",
  retry: "Retry",
  more: "Load more documents",
  morePage: "Load more documents (next page)",
  loaded: "{{loaded}} of {{total}} documents loaded",
  sourcePlaceholder: "All documents; type to search",
  sourceHint:
    "Search by document name. Selections stay selected across pages and searches.",
  sourceIncomplete:
    "The document list ended before its reported total. Retry to refresh the directory.",
  quality: "Retrieval quality",
  selectedDocumentThresholdHint:
    "With documents selected, the service bypasses this threshold for final scores. In-scope chunks with a similarity of 0 may still be returned.",
  candidates: "Candidate limit",
  candidatesHint:
    "Candidate pool before ranking, up to 2048. This is not the number of results displayed.",
  strategy: "Retrieval strategy and models",
  presentation: "Display and citations",
  strategies: {
    default: "Service default",
    sparse: "Keyword search",
    dense: "Semantic search",
    hybrid: "Weighted hybrid",
    fusion: "Fusion",
  },
  rerankNone: "No reranking",
  rerankHint: "Configured rerank models. Clear to use the original ranking.",
  rerankError: "Rerank models could not be loaded.",
  rerankEmpty: "No enabled rerank model is configured.",
  rerankUnavailable:
    "The selected rerank model is unavailable. Choose another model or clear it.",
  weightHint: "Semantic score share in weighted hybrid retrieval.",
  fusionKeywordWeight: "Fusion keyword weight",
  fusionSemanticWeight: "Fusion semantic weight",
  fusionInvalid: "Fusion weights must be non-negative, with a positive sum.",
  strategyHelp: {
    default:
      "Use the retrieval service default: vector candidates, followed by ranking.",
    sparse: "Retrieve by lexical terms without vector candidates.",
    dense: "Retrieve embedding candidates without lexical candidates.",
    hybrid:
      "Combine independent lexical and vector candidates using the semantic weight below.",
    fusion:
      "Combine independent lexical and vector candidates. Weights are keyword first, semantic second, and normalized by their sum.",
  },
  scopeMismatch:
    "The service returned sources outside the requested scope. Results are hidden. Check the retrieval service version before retrying.",
  evidence: "Source evidence",
  testedQuestion: "Tested question",
  changed:
    "The form has changed since this test. The results below belong to the tested question and scope.",
  running: "Retrieving and ranking sources…",
  notRun: "Ask a question to inspect its source evidence.",
  emptyHint:
    "Check that documents have finished parsing and have enabled chunks, or adjust the threshold and filter.",
  applied: "Requested scope",
  noExpansion:
    "Document and metadata filters are intersected. No matches will not expand to the whole dataset.",
  positions: "Source positions",
  chunkId: "Chunk ID",
  citation: "Citation metadata",
  sourceNewTab: "View source chunks (new tab)",
  resultsError: "Retrieval failed. Your question and filters are kept.",
};
const zh: typeof en = {
  intro: "用真实问题验证检索，再核对来源是否足以支持回答。",
  scope: "来源与元数据",
  metadata: "元数据筛选",
  mode: "筛选方式",
  disabled: "不筛选",
  manual: "手动条件",
  auto: "自动推断",
  semi_auto: "限定字段自动推断",
  autoHint:
    "使用已配置的对话模型从问题推断条件。没有生成条件时，保留所选文档范围。",
  semiHint: "对话模型仅使用以下字段。可指定比较方式，或由模型决定。",
  generatedExclusionHint:
    "条件由检索服务内部生成。多值字段的负向比较可能扩大结果，保留本应排除的文档；本页无法校验内部生成的条件，请核对来源后使用结果。",
  logic: "条件关系",
  and: "全部满足（AND）",
  or: "任一满足（OR）",
  field: "字段",
  operator: "比较方式",
  value: "值",
  valueType: "值类型",
  text: "文本 / 日期",
  number: "数字",
  list: "文本列表",
  add: "添加条件",
  remove: "移除条件",
  modelOperator: "由模型决定",
  advancedJSON: "高级 JSON",
  invalid: "请检查筛选条件：字段、比较方式和值必须有效。",
  filterDraftInvalid: "筛选草稿无效，请修正后再检索。",
  unknownMetadataFields:
    "所选字段不存在于本库元数据中，请选择已有字段后再检索。",
  metadataDirectoryRequired:
    "限定字段自动推断需先成功加载元数据字段目录。请重试或选择其它筛选方式。",
  invalidMetadataList:
    "列表至少包含一个文本、有限数字或布尔值，不能包含 null、对象、嵌套列表或非有限数字。",
  unsupportedMetadataList:
    "只有“属于列表”支持数组值。草稿已保留，请修改运算符后再检索。",
  unsupportedMetadataExclusion:
    "负向筛选（不属于列表、不等于、不包含）暂不可用：多值文档即使包含被排除值仍可能命中。草稿已保留，请改用正向属于列表或明确选择文档范围。",
  listHint:
    "“属于列表”可直接添加文本标签；混合数字和布尔值请编辑高级 JSON，现有类型标签会在此保留原类型。“不属于列表”因多值文档排除不可靠仍不可用。空列表无效。",
  operators: {
    equals: "等于",
    notEquals: "不等于（暂不可用）",
    contains: "包含",
    notContains: "不包含（暂不可用）",
    starts: "开头是",
    ends: "结尾是",
    empty: "为空",
    notEmpty: "非空",
    in: "属于列表",
    notIn: "不属于列表（暂不可用）",
  },
  keysError:
    "元数据字段加载失败。手动条件仍可输入已知字段，限定字段自动推断需先重试成功。",
  retry: "重试",
  more: "加载更多文档",
  morePage: "加载更多文档（下一页）",
  loaded: "已加载 {{loaded}} / {{total}} 个文档",
  sourcePlaceholder: "全部文档，可按名称搜索",
  sourceHint: "按文档名称搜索，跨页和切换搜索均保留已选来源。",
  sourceIncomplete: "文档列表提前结束，尚未达到返回的总数。请重试刷新目录。",
  quality: "检索质量",
  selectedDocumentThresholdHint:
    "已选择文档时，服务不会按此阈值过滤最终得分；范围内相似度为 0 的切片仍可能返回。",
  candidates: "候选切片上限",
  candidatesHint: "排序前的候选池，最多 2048 条，并非展示结果数量。",
  strategy: "检索策略与模型",
  presentation: "展示与引用",
  strategies: {
    default: "检索服务默认",
    sparse: "关键词检索",
    dense: "语义检索",
    hybrid: "加权混合检索",
    fusion: "融合检索",
  },
  rerankNone: "不重排序",
  rerankHint: "已配置的重排序模型，清空后使用原始排序。",
  rerankError: "重排序模型加载失败。",
  rerankEmpty: "尚未配置启用的重排序模型。",
  rerankUnavailable: "所选重排序模型不可用，请选择其它模型或清空选择。",
  weightHint: "加权混合检索中语义得分所占比例。",
  fusionKeywordWeight: "融合关键词权重",
  fusionSemanticWeight: "融合语义权重",
  fusionInvalid: "融合权重必须非负，且总和大于零。",
  strategyHelp: {
    default: "使用检索服务默认的向量召回，再进行排序。",
    sparse: "按关键词召回，不使用向量候选。",
    dense: "按向量召回，不使用关键词候选。",
    hybrid: "独立召回关键词与向量候选，按下方语义权重合并。",
    fusion: "独立召回关键词与向量候选；先关键词、后语义，权重按总和归一化。",
  },
  scopeMismatch:
    "检索服务返回了请求范围之外的来源，结果已隐藏。请核对检索服务版本后再重试。",
  evidence: "来源证据",
  testedQuestion: "已检索的问题",
  changed: "表单已修改，下方结果对应上次检索的问题与范围。",
  running: "正在检索与排序来源…",
  notRun: "输入问题，查看可核对的来源证据。",
  emptyHint: "检查文档是否解析完成且有启用切片，或调整阈值与筛选条件。",
  applied: "请求范围",
  noExpansion: "文档范围与元数据条件取交集。没有匹配时，不会扩大到全库。",
  positions: "来源位置",
  chunkId: "切片 ID",
  citation: "引用元数据",
  sourceNewTab: "查看来源切片（新标签页）",
  resultsError: "检索失败，问题与筛选条件已保留。",
};
i18next.addResourceBundle("en", "retrievalWorkbench", en);
i18next.addResourceBundle("zh", "retrievalWorkbench", zh);
