import {
  ChatCircleDots, ChartBar, ShieldCheck, Crosshair, UserCircle, Terminal,
  Robot, Brain, Lightbulb, Cpu, MagicWand, Detective, Compass, Rocket,
  GearSix, Code, GraduationCap, GlobeHemisphereWest, PuzzlePiece, Eye,
  Megaphone, Notebook, FirstAidKit, Scales, PresentationChart, ClipboardText,
  Headset, Wrench, Lightning, CurrencyDollar,
  type Icon as PhosphorIcon
} from '@phosphor-icons/react'

/** Map AGENT_ICON_OPTIONS names to Phosphor React components. */
const ICON_MAP: Partial<Record<string, PhosphorIcon>> = {
  ChatCircleDots, ChartBar, ShieldCheck, Crosshair, UserCircle, Terminal,
  Robot, Brain, Lightbulb, Cpu, MagicWand, Detective, Compass, Rocket,
  GearSix, Code, GraduationCap, GlobeHemisphereWest, PuzzlePiece, Eye,
  Megaphone, Notebook, FirstAidKit, Scales, PresentationChart, ClipboardText,
  Headset, Wrench, Lightning, CurrencyDollar
}

export function getIconComponent(name: string): PhosphorIcon {
  return ICON_MAP[name] ?? UserCircle
}

/** Lighten a hex color towards white by the given amount (0-1). */
export function lightenHex(hex: string, amount = 0.9): string {
  const match = hex.replace('#', '').match(/.{2}/g)
  if (!match) return '#F3F4F6'
  const [r, g, b] = match.map((v) => {
    const c = parseInt(v, 16)
    return Math.round(c + (255 - c) * amount)
  })
  return '#' + [r, g, b].map((v) => v.toString(16).padStart(2, '0')).join('')
}
