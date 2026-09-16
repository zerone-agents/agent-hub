import { Agent, KnowledgeDocument, KnowledgeFolder, ScenarioPrompt } from '../types';

export const INITIAL_SCENARIO_PROMPTS: ScenarioPrompt[] = [
  {
    id: 'sc-1',
    icon: '💊',
    title: '处方前置审核与配伍禁忌',
    prompt: '请帮我审核该份联合用药处方：阿托伐他汀 40mg qd + 克拉霉素 500mg bid + 地高辛 0.125mg qd，分析潜在药物相互作用、肌溶解风险及处置建议。',
    category: 'prescription',
  },
  {
    id: 'sc-2',
    icon: '🦠',
    title: '抗菌药物分级管理与合规评估',
    prompt: '请针对ICU患者培养出产碳青霉烯酶肺炎克雷伯菌(CRKP)的病例，评估多粘菌素B联合替加环素治疗方案的选药指征、负荷剂量与特殊使用级审批合规性。',
    category: 'antibiotic',
  },
  {
    id: 'sc-3',
    icon: '⚠️',
    title: '药品不良反应(ADR)因果评价',
    prompt: '患者静滴头孢哌酮舒巴坦期间发生双硫仑样反应，请协助进行 Naranjo 评分因果关系推断，并生成国家 ADR 直报系统填报草案。',
    category: 'adr',
  },
  {
    id: 'sc-4',
    icon: '🔐',
    title: '麻精药品五专管理与残液销毁SOP',
    prompt: '请输出一套符合三甲评审标准的住院药房芬太尼与吗啡注射液空安瓿回收、双人残液核销及专用账册红处方归档的精细化SOP。',
    category: 'narcotics',
  },
  {
    id: 'sc-5',
    icon: '📊',
    title: '药剂科质控指标与合理用药分析',
    prompt: '请针对我院上季度药占比、门诊处方点评合格率、抗菌药物使用强度(AUD)进行多维度数据统计并给出改善策略。',
    category: 'data',
  },
  {
    id: 'sc-6',
    icon: '💉',
    title: 'PIVAS肠外营养(TPN)处方核算',
    prompt: '请核算一份包含20%脂肪乳250ml、8.5%复方氨基酸500ml、50%葡萄糖300ml及电解质的重症患者TPN热氮比、糖脂比、渗透压与钙磷沉淀风险。',
    category: 'pivas',
  },
  {
    id: 'sc-7',
    icon: '🫁',
    title: '慢病用药教育与吸入剂宣教指导',
    prompt: '请为新开具布地奈德福莫特罗吸入粉雾剂的72岁慢性阻塞性肺疾病(COPD)合并高血压患者，制作一份用药时间表与吸入技巧宣教清单。',
    category: 'chronic',
  },
  {
    id: 'sc-8',
    icon: '🍵',
    title: '中药处方十八反十九畏审查',
    prompt: '请审核门诊中药饮片处方：半夏12g、川乌(制)6g、附子(先煎)9g、白芍15g、炙甘草6g，排查十八反及毒性饮片先煎久煎医嘱规范。',
    category: 'tcm',
  },
];

export const INITIAL_AGENTS: Agent[] = [
  {
    id: 'agent-clinical-review',
    name: '临床药师·处方前置审核专家 (陆审方)',
    title: '三甲医院主任药师 / 临床药学学科带头人',
    avatar: '💊',
    category: '处方审核',
    usageCount: '12.68万次调用',
    description:
      '专注处方与医嘱前置合规性拦截审核。擅长排查药物配伍禁忌、严重药物相互作用(DDI)、超说明书用药循证证据级别、肝肾功能不全患者的肌酐清除率剂量调整以及特殊人群用药禁忌。',
    capabilities:
      '前置拦截高风险处方、相互作用等级评定、肝肾清除率剂量阶梯换算、超说明书循证评估报告输出。',
    tags: ['处方前置审核', '配伍禁忌', '剂量换算', '超说明书用药'],
    suggestedQuestions: [
      '患者68岁，肌酐清除率28ml/min，拟用头孢哌酮舒巴坦和依诺肝素，请评估剂量及用药合理性。',
      '请帮我审核这张处方：阿托伐他汀40mg qd 联用克拉霉素500mg bid，是否存在显著相互作用风险？',
      '请针对肿瘤科提交的贝伐珠单抗超说明书用药申请，出具循证药学证据等级与风险告知评估。',
    ],
    featured: true,
  },
  {
    id: 'agent-antibiotic-steward',
    name: '抗微生物药物管理药师 (林抗生)',
    title: '副主任药师 / 感染专科临床药师',
    avatar: '🦠',
    category: '抗菌药物',
    usageCount: '9.45万次调用',
    description:
      '专注抗菌药物临床合理应用管理(AMS)。精通非限制级、限制级、特殊使用级抗菌药物分级会诊机制、多重耐药菌(CRKP/MRSA/VRE)抗感染降阶梯路径、围手术期预防性抗菌药物规范及PK/PD治疗药物监测(TDM)。',
    capabilities:
      '耐药菌联合用药方案拟定、抗菌药物使用强度(AUD)核算与通报、围术期预防用药时机合规审查。',
    tags: ['抗微生物管理', '耐药菌方案', 'PK/PD监测', '分级审核'],
    suggestedQuestions: [
      'ICU重症肺炎患者培养出泛耐药鲍曼不动杆菌，目前多粘菌素B联合替加环素的用药方案与滴速怎么制定？',
      '请制定三甲综合医院外科Ⅰ类切口手术预防使用抗菌药物的品种选择、给药时机与停药标准。',
      '万古霉素谷浓度监测结果为22mg/L，患者出现轻度血肌酐升高，请给出具体的减量或延长给药间隔方案。',
    ],
    featured: true,
  },
  {
    id: 'agent-adr-vigilance',
    name: '药品不良反应(ADR)安全警戒专家 (严警戒)',
    title: '副主任药师 / 药物警戒国家级评价师',
    avatar: '⚠️',
    category: '不良反应',
    usageCount: '7.82万次调用',
    description:
      '专注于药品不良反应(ADR)的早筛、甄别与闭环处置。熟练运用 Naranjo 算法与国家药物警戒评价准则进行因果关联性判定，擅长药源性器官损伤（如DILI、DINI）溯源与国家直报系统报告草案生成。',
    capabilities:
      '严重不良反应因果推断、过敏反应与毒副反应鉴别、国家不良反应直报表单撰写、风险信号警示。',
    tags: ['不良反应监测', '因果关联推断', '药源性损害', '直报表生成'],
    suggestedQuestions: [
      '患者静滴万古霉素过程中出现面部及颈部充血红斑伴低血压，请评估是否为红人综合征及应急处置步骤。',
      '请根据国家ADR监测中心填报规范，帮我起草一份免疫检查点抑制剂相关心肌炎的严重不良反应报告。',
      '住院患者联用多种降压药与非甾体抗炎药后急性肌酐倍增，请评估药源性急性肾损伤(DINI)的责任药物。',
    ],
    featured: true,
  },
  {
    id: 'agent-narcotics-manager',
    name: '特殊药品与麻精药品质控顾问 (顾精麻)',
    title: '主管药师 / 药房麻精药品专管员',
    avatar: '🔐',
    category: '麻精特殊药',
    usageCount: '8.13万次调用',
    description:
      '严格落实《麻醉药品和精神药品管理条例》及医疗机构麻精药品“五专”管理（专人负责、专柜加锁、专用账册、专用处方、专册登记）。涵盖双人双锁、残液销毁、高危药品A/B/C分级警示与追溯防盗核查。',
    capabilities:
      '麻精药品红处方合规查验、空安瓿回收双人核销、高危药品专区存放标准、突发药品盗失应急响应。',
    tags: ['麻精药品五专', '双人双锁', '残液销毁SOP', '高危药质量控制'],
    suggestedQuestions: [
      '请梳理住院药房麻醉药品注射剂（吗啡、芬太尼）空安瓿回收、残液双人核销及专册登记的完整SOP。',
      '高危药品（如10%氯化钾注射液、高浓度硫酸镁）在急诊药房与病区小药柜的警示标识与双人双核对标准是什么？',
      '癌症慢性重度疼痛患者门诊开具盐酸羟考酮缓释片，红处方最大开药天数与病历建档管理要求是什么？',
    ],
    featured: true,
  },
  {
    id: 'agent-chronic-care',
    name: '慢病用药管理与药学门诊药师 (许同心)',
    title: '专科药师 / 慢病药学门诊出诊专家',
    avatar: '🫁',
    category: '慢病门诊',
    usageCount: '6.90万次调用',
    description:
      '致力于门诊多病共存患者的用药重整与健康素养提升。擅长针对老年高血压、2型糖尿病、慢阻肺(COPD)、痛风患者提供易懂的用药时间表，并指导胰岛素笔、吸入剂等复杂给药装置的标准化操作。',
    capabilities:
      '用药重整与减药建议、特殊给药装置规范宣教、药物食物相互作用规避、用药依从性干预。',
    tags: ['慢病药学门诊', '用药重整', '吸入剂宣教', '依从性管理'],
    suggestedQuestions: [
      '请为一位合并2型糖尿病和痛风的72岁患者出具一份通俗易懂的用药时间表与饮食注意事项。',
      '慢性阻塞性肺疾病(COPD)患者吸入噻托溴铵和布地奈德福莫特罗时，如何指导其正确吸入技巧与口腔漱口要点？',
      '冠心病术后患者服用阿司匹林+替格瑞洛期间出现轻度皮下瘀斑，应如何指导患者居家观察与自我监测？',
    ],
  },
  {
    id: 'agent-pivas-tpn',
    name: 'PIVAS静脉用药调配与营养药师 (赵静配)',
    title: '主管药师 / 静脉用药调配中心(PIVAS)专科质控',
    avatar: '💉',
    category: 'PIVAS静配',
    usageCount: '5.74万次调用',
    description:
      '精通静脉用药调配中心(PIVAS)全流程质控。涵盖全静脉营养(TPN)配方的非蛋白热氮比、糖脂比、电解质浓度及钙磷沉淀溶度积核算，化疗细胞毒性药物负压生物安全柜调配防护与外溢应急处置。',
    capabilities:
      'TPN配方稳定性与渗透压核算、配伍相容性智能审核、细胞毒药物泄漏应急方案、洁净区感控质控。',
    tags: ['PIVAS静配中心', 'TPN营养核算', '细胞毒化疗药防护', '沉淀风险排查'],
    suggestedQuestions: [
      '请帮我计算该重症术后患者TPN配方的非蛋白热氮比、糖脂比及渗透压，并评估钙磷沉淀风险。',
      '静脉用药调配中心(PIVAS)生物安全柜中调配顺铂与氟尿嘧啶时的外溢应急处理预案怎么制定？',
      '前列地尔注射液、硝普钠等见光易分解药品的PIVAS配制避光要求与输液时限规定有哪些？',
    ],
  },
  {
    id: 'agent-tcm-pharmacy',
    name: '中药房处方调剂与饮品质控专家 (孙草堂)',
    title: '主管中药师 / 国家执业中药师',
    avatar: '🍵',
    category: '中药质控',
    usageCount: '4.88万次调用',
    description:
      '精研中药饮片与配方颗粒的辨证调剂审核。严格把关“十八反、十九畏”、妊娠禁用与慎用中药，指导毒性中药（草乌、附子、马钱子）的煎服法（先煎久煎、包煎、后下、烊化），确保传统用药安全。',
    capabilities:
      '十八反十九畏实时预警、毒性中药先煎后下审核、配方颗粒折算系数核查、中药煎煮与服药禁忌宣教。',
    tags: ['十八反十九畏', '中药饮片调剂', '毒性中药煎服法', '妊娠用药禁忌'],
    suggestedQuestions: [
      '请审核此张门诊中药处方：半夏12g、川乌(制)6g、附子(先煎)9g、白芍15g、炙甘草6g，排查十八反及毒性饮片风险。',
      '中药配方颗粒与中药饮片的临床当量换算系数如何标准化，并如何防范发药差错？',
      '请为开具十全大补汤膏方的患者出具一份规范的服用方法、温服时机及忌口禁忌指导。',
    ],
  },
  {
    id: 'agent-pharm-director',
    name: '医院药学综合管理与科研教学顾问 (周科长)',
    title: '主任药师 / 药事管理委员会(DTC)秘书长',
    avatar: '🏥',
    category: '药事管理',
    usageCount: '6.12万次调用',
    description:
      '协助药事管理与药物治疗学委员会(DTC)日常运转。熟谙国家基本药物制度、国家组织药品集中带量采购(VBP)执行监测、重点监控合理用药目录评估、三甲评审药事指标达成及药学科研立项指导。',
    capabilities:
      '国家集采药品用量监测分析、药事委员会立项评审、基本药物配备比例评估、药学论文与专利开题。',
    tags: ['药事管理委员会', '集采监测(VBP)', '基本药物制度', '三甲评审标准'],
    suggestedQuestions: [
      '请帮我起草一份本季度国家集采中选药品使用进度监测通报与未达标临床科室整改督办单。',
      '三甲医院评审关于重点监控药品目录调整与用药预警机制的考核要点有哪些？',
      '请针对临床药师主导的“基于真实世界数据的抗肿瘤药物超说明书用药评估”，梳理一份科研课题申报书框架。',
    ],
  },
];

export const INITIAL_FOLDERS: KnowledgeFolder[] = [
  {
    id: 'folder-test-0707',
    name: '测试0707',
    description: '测试中文名知识库',
    docCount: 2,
    chunkCount: 144,
    parseMethod: 'naive',
    createdAt: '2026-07-10',
    updatedAt: '2026-07-10',
    category: 'mine',
    tags: ['测试', '基础库'],
  },
  {
    id: 'folder-cbt',
    name: '认知行为疗法',
    description: '关于认知行为流派的各类知识——例如，三角模型、五因素概念化、自动思维监控与临床干预案例',
    docCount: 10,
    chunkCount: 347,
    parseMethod: 'naive',
    createdAt: '2026-09-10',
    updatedAt: '1 小时前',
    category: 'team',
    tags: ['CBT', '干预模型'],
  },
  {
    id: 'folder-video',
    name: '新知识库-视频',
    description: 'CBT/SFBT/ACT/DBT/MI/人本等流派的视频课程，包括常见精神药理与心身医学交叉指导',
    docCount: 4,
    chunkCount: 3,
    parseMethod: 'naive',
    createdAt: '2026-09-02',
    updatedAt: '12 天前',
    category: 'team',
    tags: ['视频课程', '培训资料'],
  },
  {
    id: 'folder-counselor',
    name: '咨询师知识库',
    description: '用于给 AI 查找专业的药学与心理门诊咨询知识库，包含伦理守则与常见处方释疑',
    docCount: 49,
    chunkCount: 3250,
    parseMethod: 'naive',
    createdAt: '2026-08-20',
    updatedAt: '20 小时前',
    category: 'team',
    tags: ['咨询', '知识检索'],
  },
  {
    id: 'folder-kb01',
    name: 'KB01 管理规范',
    description: '指导原则、分级管理、超说明书规定、药师职责与药事管理质控规范',
    docCount: 2,
    chunkCount: 98,
    parseMethod: 'naive',
    createdAt: '2026-09-13',
    updatedAt: '21 小时前',
    category: 'team',
    tags: ['管理规范', '药师职责'],
  },
  {
    id: 'folder-test-0702',
    name: 'test_0702',
    description: '云环境首个知识库，用于系统连通性与检索分块测试',
    docCount: 1,
    chunkCount: 54,
    parseMethod: 'naive',
    createdAt: '2026-07-02',
    updatedAt: '2026-07-02',
    category: 'mine',
    tags: ['测试', '云端'],
  },
  {
    id: 'folder-narcotics',
    name: '麻精与特殊药品管理专库',
    description: '麻精药品五专管理制度、双人双锁、残液销毁及高危药品ABC级管理规范',
    docCount: 1,
    chunkCount: 86,
    parseMethod: 'naive',
    category: 'team',
    icon: '🔐',
    color: 'emerald',
    createdAt: '2026-09-14',
    updatedAt: '2 小时前',
    tags: ['麻精药品', '高危管控', '质控SOP'],
  },
  {
    id: 'folder-ams',
    name: '临床抗菌药物管理(AMS)知识库',
    description: '抗菌药物三级分级目录、多重耐药菌联合方案、PK/PD治疗药物监测规范',
    docCount: 1,
    chunkCount: 112,
    parseMethod: 'naive',
    category: 'team',
    icon: '🦠',
    color: 'blue',
    createdAt: '2026-09-12',
    updatedAt: '1 天前',
    tags: ['抗菌药物', 'AMS项目', '耐药菌方案'],
  },
  {
    id: 'folder-prescript-review',
    name: '处方智能前审与合理用药库',
    description: '重点药物相互作用(DDI)、配伍禁忌、妊娠用药禁忌与医嘱前置拦截标准',
    docCount: 2,
    chunkCount: 168,
    parseMethod: 'naive',
    category: 'mine',
    icon: '📋',
    color: 'purple',
    createdAt: '2026-09-10',
    updatedAt: '3 天前',
    tags: ['合理用药', '处方前审', 'DDI禁忌'],
  },
  {
    id: 'folder-pivas-tcm',
    name: '静配中心(PIVAS)与中药专库',
    description: '肠外营养相容性计算、抗肿瘤配置防护、中药饮片十八反十九畏审查',
    docCount: 2,
    chunkCount: 130,
    parseMethod: 'naive',
    category: 'mine',
    icon: '🧪',
    color: 'amber',
    createdAt: '2026-09-02',
    updatedAt: '5 天前',
    tags: ['PIVAS', '中药处方', '调剂质控'],
  },
];

export const INITIAL_DOCUMENTS: KnowledgeDocument[] = [
  {
    id: 'doc-pharm-1',
    name: '三甲医院麻精药品与高危药品“五专”管理SOP.docx',
    type: 'docx',
    category: 'team',
    folderId: 'folder-narcotics',
    folderName: '麻精与特殊药品管理专库',
    size: '185.4 KB',
    updatedAt: '2026-09-14 09:30',
    tags: ['麻精药品', '高危药品', '五专SOP', '药剂科质控'],
    content: `三级甲等综合医院麻醉药品与精神药品“五专”管理标准操作规程 (SOP-PHA-008)
1. 管理总则
严格遵循《麻醉药品和精神药品管理条例》，实行“专人负责、专柜加锁、专用账册、专用处方、专册登记”管理。
2. 药品入库与双人核对
麻精药品实行双人开箱验收、双人签字入库。账物核对准确无误后存入双人双锁保险柜。
3. 处方查验与限量
- 门诊麻醉药品注射剂：1次常用量；控缓释制剂：不超过7日常用量；其他剂型：不超过3日常用量。
- 处方必须使用红色麻醉药品专用处方，签署专职执业医师及审核药师全名。
4. 空安瓿回收与残液销毁
使用后空安瓿实行100%回收计数，双人核对签名后封存；注射剂剩余残液由两位专管药师在专用记录本签名后依感控规程销毁。`,
    summary: '麻精药品入库核对、红处方审核开药限制、空安瓿回收、双人残液销毁及高危分级存放的规范手册。',
  },
  {
    id: 'doc-pharm-2',
    name: '2026年临床抗微生物药物分级管理与PK-PD指导目录.xlsx',
    type: 'xlsx',
    category: 'team',
    folderId: 'folder-ams',
    folderName: '临床抗菌药物管理(AMS)知识库',
    size: '96.2 KB',
    updatedAt: '2026-09-12 15:20',
    tags: ['抗菌药物', '分级管理', 'PK/PD', '会诊目录'],
    content: `抗微生物药物分级与使用权限对照表：
1. 非限制使用级：青霉素类（阿莫西林）、一代头孢（头孢唑林）、氨基糖苷类（庆大霉素）。所有具有处方权的医师可开具。
2. 限制使用级：二三代头孢（头孢呋辛、头孢曲松、头孢他啶）、喹诺酮类（莫西沙星、左氧氟沙星）。主治医师及以上开具。
3. 特殊使用级：碳青霉烯类（美罗培南、亚胺培南）、糖肽类（万古霉素、替考拉宁）、多粘菌素B、替加环素、恶唑烷酮类（利奈唑胺）。必须经抗感染临床药师或专家会诊同意，副主任医师以上开具。
4. PK/PD 优化参数：
- 时间依赖型（β-内酰胺类）：延长输注时间至3小时，维持 %T>MIC > 60%
- 浓度依赖型（氨基糖苷类、氟喹诺酮类）：日单次给药，追求 Cmax/MIC 与 AUC/MIC 最大化`,
    summary: '医院抗菌药物非限制/限制/特殊使用三级管理准入目录，以及多重耐药菌PK/PD时间依赖与浓度依赖参数。',
  },
  {
    id: 'doc-pharm-3',
    name: '心血管与内分泌重点药物相互作用(DDI)筛查指引.html',
    type: 'html',
    category: 'mine',
    folderId: 'folder-prescript-review',
    folderName: '处方智能前审与合理用药库',
    size: '112.0 KB',
    updatedAt: '2026-09-10 11:45',
    tags: ['处方审核', 'DDI禁忌', '心血管用药', '肌溶解风险'],
    content: `<!DOCTYPE html>
<html>
<head><title>重点药物相互作用(DDI)及配伍禁忌前置审查指引</title></head>
<body>
<h2>一、CYP3A4 代谢酶强相互作用预警</h2>
<p><strong>阿托伐他汀 / 辛伐他汀 + 克拉霉素 / 伊曲康唑 / 环孢素</strong>：强效抑制CYP3A4，显著提升他汀血药浓度，横纹肌溶解风险激增 10~20 倍。应避免联用或换用瑞舒伐他汀/匹伐他汀，并监测CK与肌痛。</p>
<h2>二、抗血小板药与抑酸剂相互作用</h2>
<p><strong>氯吡格雷 + 奥美拉唑 / 埃索美拉唑</strong>：竞争CYP2C19活性，削弱氯吡格雷抗血小板活化效率，增加支架内血栓风险。推荐联用泮托拉唑或雷贝拉唑。</p>
<h2>三、地高辛中毒高危组合</h2>
<p>地高辛 + 胺碘酮 / 维拉帕米 / 螺内酯：胺碘酮使地高辛血浓度倍增，联用时地高辛剂量需常规减半并监测心电图及地高辛血药浓度。</p>
</body>
</html>`,
    summary: '针对他汀类肌溶解高危联用、氯吡格雷与质子泵抑制剂、地高辛中毒高危组合的前置审查技术标准。',
  },
  {
    id: 'doc-pharm-4',
    name: '国家药品不良反应(ADR)严重事件快速甄别与填报规范.pdf',
    type: 'pdf',
    category: 'mine',
    folderId: 'folder-prescript-review',
    folderName: '处方智能前审与合理用药库',
    size: '1.2 MB',
    updatedAt: '2026-09-06 14:10',
    tags: ['ADR直报', '严重不良反应', '因果评价', '国家标准'],
    content: `国家药品不良反应(ADR)严重事件上报流程与评定准则：
1. 严重药品不良反应定义：导致死亡、危及生命、致癌致畸、显著致残或导致住院/延长住院时间的反应。
2. 上报时限：死亡病例必须自发现起15日内直报并调查；严重不良反应15日内上报；一般反应30日内上报。
3. Naranjo 不良反应因果关系评分体系：
- 评分 >= 9 分：肯定（Definite）
- 评分 5 ~ 8 分：很可能（Probable）
- 评分 1 ~ 4 分：可能（Possible）
- 评分 <= 0 分：存疑（Doubtful）`,
    summary: '严重药品不良反应直报时限、死亡病例应急调查以及 Naranjo 因果关系判定量表规范。',
  },
  {
    id: 'doc-pharm-5',
    name: '全静脉营养(TPN)处方相容性与渗透压智能审核规范.md',
    type: 'md',
    category: 'team',
    folderId: 'folder-pivas-tcm',
    folderName: '静配中心(PIVAS)与中药专库',
    size: '24.6 KB',
    updatedAt: '2026-09-02 16:30',
    tags: ['PIVAS', 'TPN配方', '沉淀规避', '热氮比计算'],
    content: `# PIVAS 肠外营养(TPN)处方审核标准
1. 热氮比范围：重症应激患者 100~120 kcal : 1 g 氮；稳定期患者 120~150 kcal : 1 g 氮。
2. 糖脂比范围：葡萄糖供能 60%~70%，脂肪乳供能 30%~40%。
3. 钙磷沉淀风险控制：
   - 磷酸盐与葡萄糖酸钙的摩尔浓度乘积不可过高。
   - 必须遵循加入顺序：磷酸盐加入氨基酸稀释，葡萄糖酸钙加入葡萄糖稀释，最后混合，严禁直接接触。
4. 渗透压计算：外周静脉输注渗透压上限为 900 mOsm/L；中心静脉(CVC/PICC)可承受 > 900 mOsm/L。`,
    summary: 'PIVAS肠外营养处方非蛋白热氮比、糖脂比计算公式、钙磷沉淀规避加入顺序与渗透压输注途径限制。',
  },
  {
    id: 'doc-pharm-6',
    name: '中药饮片十八反十九畏与毒性中药煎服质控标准.docx',
    type: 'docx',
    category: 'mine',
    folderId: 'folder-pivas-tcm',
    folderName: '静配中心(PIVAS)与中药专库',
    size: '88.3 KB',
    updatedAt: '2026-08-25 10:15',
    tags: ['中药调剂', '十八反', '毒性饮片', '先煎久煎'],
    content: `医院中药房中药饮片调剂质控核心要则：
1. 十八反禁忌排查：
   - 乌头（川乌、草乌、附子）反半夏、瓜蒌、贝母、白蔹、白及。
   - 甘草反甘遂、大戟、海藻、芫花。
   - 藜芦反人参、沙参、丹参、玄参、细辛、芍药。
2. 毒性中药煎服法：
   - 附子、草乌、川乌：必须标注【先煎】，煎煮1~2小时以口尝无麻舌感为度，降低乌头碱毒性。
   - 细辛：谨遵“细辛不过钱”之古训，散剂严格控制用量。
3. 妊娠用药禁忌：三棱、莪术、水蛭、巴豆、麝香列为妊娠禁用；红花、桃仁列为妊娠慎用。`,
    summary: '中药房中药饮片处方调剂十八反十九畏核对、毒性饮片先煎久煎医嘱审查及妊娠禁忌药物目录。',
  },
];
