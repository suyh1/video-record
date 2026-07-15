import type { SVGProps } from 'react'

type BrandMarkProps = Omit<SVGProps<SVGSVGElement>, 'children'> & {
  size?: number | string
}

export function BrandMark({ size = 24, ...props }: BrandMarkProps) {
  return (
    <svg
      {...props}
      aria-hidden="true"
      data-brand-mark="film-archive"
      fill="none"
      focusable="false"
      height={size}
      viewBox="0 0 24 24"
      width={size}
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M6 3h12a3 3 0 0 1 3 3v12a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3V6a3 3 0 0 1 3-3Zm2 3.5v11h8v-11H8ZM4.5 6h2v3h-2V6Zm13 0h2v3h-2V6Zm-13 9h2v3h-2v-3Zm13 0h2v3h-2v-3Z"
        fill="currentColor"
        fillRule="evenodd"
      />
      <path d="M10 12h4" stroke="currentColor" strokeLinecap="round" strokeWidth="2" />
    </svg>
  )
}
