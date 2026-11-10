import type { ReactNode, SVGProps } from 'react'

interface IconProps extends Omit<SVGProps<SVGSVGElement>, 'children'> {
  size?: number
}

function Icon({ size = 18, children, ...props }: IconProps & { children: ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  )
}

export function PauseIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M9 5v14M15 5v14" />
    </Icon>
  )
}

export function PlayIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M8 5.5v13l10-6.5-10-6.5Z" />
    </Icon>
  )
}

export function ClearIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M4.5 9a8 8 0 1 1 1.8 7.1" />
      <path d="M4 4.5V9h4.5" />
    </Icon>
  )
}

export function PlusIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M12 5v14M5 12h14" />
    </Icon>
  )
}

export function MinusIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M5 12h14" />
    </Icon>
  )
}

export function FitIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M9 4H4v5M15 4h5v5M20 15v5h-5M4 15v5h5" />
    </Icon>
  )
}

export function CloseIcon(props: IconProps) {
  return (
    <Icon {...props}>
      <path d="M6 6l12 12M18 6 6 18" />
    </Icon>
  )
}

export function WeaveMark({ size = 24 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <path
        d="M9 9.5c5 0 7 3.5 7 6.5s2 6.5 7 6.5"
        stroke="var(--accent)"
        strokeWidth="1.7"
        strokeLinecap="round"
      />
      <path
        d="M23 9.5c-5 0-7 3.5-7 6.5s-2 6.5-7 6.5"
        stroke="var(--text)"
        strokeWidth="1.7"
        strokeLinecap="round"
      />
      <circle cx="16" cy="16" r="1.35" fill="var(--accent)" />
    </svg>
  )
}
