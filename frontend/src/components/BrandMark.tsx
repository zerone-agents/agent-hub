interface BrandMarkProps {
  size?: number
  className?: string
}

export default function BrandMark({ size = 32, className }: BrandMarkProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
    >
      <path d="M14 30L18 26L31 39L27 43Z" fill="var(--primary)" />
      <path d="M27 39L31 43L51 23L47 19Z" fill="var(--primary)" />
      <circle cx="16" cy="28" r="5" fill="var(--primary)" />
      <circle cx="29" cy="41" r="5" fill="var(--primary)" />
      <circle cx="49" cy="21" r="5" fill="var(--primary)" />
      <circle cx="16" cy="28" r="2" fill="var(--primary-foreground)" />
      <circle cx="29" cy="41" r="2" fill="var(--primary-foreground)" />
      <circle cx="49" cy="21" r="2" fill="var(--primary-foreground)" />
    </svg>
  )
}
