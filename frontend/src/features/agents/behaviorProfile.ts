import type { BehaviorProfile } from '@/api/agents'

export type BehaviorTraitKey = Exclude<keyof BehaviorProfile, 'version'>

export interface BehaviorTraitDefinition {
  key: BehaviorTraitKey
  label: string
  description: string
  low: string
  high: string
}

export const BEHAVIOR_TRAITS: BehaviorTraitDefinition[] = [
  {
    key: 'hierarchyCompliance',
    label: '层级服从',
    description: '遵循正式汇报链的程度',
    low: '灵活绕行',
    high: '严格守序',
  },
  {
    key: 'ambition',
    label: '权力野心',
    description: '争取影响力与可见度的主动性',
    low: '淡泊',
    high: '进取',
  },
  {
    key: 'whistleblowing',
    label: '揭弊倾向',
    description: '发现问题后主动报告的意愿',
    low: '保持沉默',
    high: '主动揭弊',
  },
  {
    key: 'riskTolerance',
    label: '风险承受',
    description: '在不确定性中采取行动的意愿',
    low: '谨慎',
    high: '冒险',
  },
  {
    key: 'conflictAvoidance',
    label: '冲突回避',
    description: '缓和或避开正面冲突的倾向',
    low: '正面对抗',
    high: '避免冲突',
  },
  {
    key: 'secrecy',
    label: '保密倾向',
    description: '控制敏感信息传播范围的程度',
    low: '开放共享',
    high: '严守信息',
  },
  {
    key: 'selfInterest',
    label: '自利倾向',
    description: '决策中维护自身利益的权重',
    low: '集体优先',
    high: '自身优先',
  },
  {
    key: 'escalationThreshold',
    label: '越级阈值',
    description: '触发越级汇报所需的事态严重度',
    low: '容易越级',
    high: '重大才越级',
  },
]

export const DEFAULT_BEHAVIOR_PROFILE: BehaviorProfile = {
  version: 1,
  hierarchyCompliance: 75,
  ambition: 30,
  whistleblowing: 55,
  riskTolerance: 35,
  conflictAvoidance: 60,
  secrecy: 55,
  selfInterest: 35,
  escalationThreshold: 75,
}

export const BEHAVIOR_PRESETS: {
  id: string
  name: string
  description: string
  profile: BehaviorProfile
}[] = [
  {
    id: 'steady-operator',
    name: '稳健执行者',
    description: '守层级、控风险，遇重大问题才越级',
    profile: DEFAULT_BEHAVIOR_PROFILE,
  },
  {
    id: 'duty-whistleblower',
    name: '尽职揭弊者',
    description: '重事实与公共责任，敢于暴露问题',
    profile: {
      version: 1,
      hierarchyCompliance: 55,
      ambition: 25,
      whistleblowing: 90,
      riskTolerance: 70,
      conflictAvoidance: 20,
      secrecy: 35,
      selfInterest: 15,
      escalationThreshold: 40,
    },
  },
  {
    id: 'political-climber',
    name: '政治投机者',
    description: '追逐影响力，善于利用信息和汇报路径',
    profile: {
      version: 1,
      hierarchyCompliance: 30,
      ambition: 90,
      whistleblowing: 45,
      riskTolerance: 75,
      conflictAvoidance: 25,
      secrecy: 75,
      selfInterest: 85,
      escalationThreshold: 35,
    },
  },
  {
    id: 'self-preserver',
    name: '谨慎自保者',
    description: '回避冲突并控制信息，优先降低个人风险',
    profile: {
      version: 1,
      hierarchyCompliance: 70,
      ambition: 20,
      whistleblowing: 20,
      riskTolerance: 15,
      conflictAvoidance: 85,
      secrecy: 80,
      selfInterest: 75,
      escalationThreshold: 90,
    },
  },
]

export function cloneBehaviorProfile(
  profile: BehaviorProfile = DEFAULT_BEHAVIOR_PROFILE,
): BehaviorProfile {
  return { ...profile }
}

export function profilesEqual(a: BehaviorProfile, b: BehaviorProfile): boolean {
  return BEHAVIOR_TRAITS.every(({ key }) => a[key] === b[key])
}
