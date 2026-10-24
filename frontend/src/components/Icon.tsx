import type { SVGProps } from 'react'

const paths = {
  compose: 'M20 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h6M15 4l5 5M10 14l1-5 7-7a2.12 2.12 0 0 1 3 3l-7 7-4 2Z',
  search: 'M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z',
  share: 'M12 16V3M7 8l5-5 5 5M8 11H5a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7a2 2 0 0 0-2-2h-3',
  person: 'M16 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0ZM4 21v-2a8 8 0 0 1 16 0v2',
  box: 'm12 3 9 5-9 5-9-5 9-5ZM3 8v9l9 5 9-5V8M12 13v9M7.5 5.5l9 5',
  people: 'M9 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM3 20v-2a6 6 0 0 1 12 0v2M16 5a3 3 0 0 1 0 6M21 20v-2a6 6 0 0 0-4-5.65',
  file: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6ZM14 2v6h6M8 13h8M8 17h5',
  image: 'M4 3h16a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1ZM3 17l5-5 4 4 4-6 5 7M8 7h.01',
  video: 'M4 4h16v16H4V4ZM10 8l6 4-6 4V8Z',
  audio: 'M9 18V5l12-2v13M9 9l12-2M9 18a3 3 0 1 1-3-3c1.66 0 3 1.34 3 3ZM21 16a3 3 0 1 1-3-3c1.66 0 3 1.34 3 3Z',
  check: 'M22 11.1V12a10 10 0 1 1-5.93-9.14M22 4 12 14l-3-3',
  warning: 'm10.3 3.9-8.5 14.7A2 2 0 0 0 3.5 21h17a2 2 0 0 0 1.7-2.4L13.7 3.9a2 2 0 0 0-3.4 0ZM12 9v4M12 17h.01',
  close: 'm6 6 12 12M6 18 18 6',
} as const

export type IconName = keyof typeof paths

export default function Icon({ name, ...props }: SVGProps<SVGSVGElement> & { name: IconName }) {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>
      <path d={paths[name]} />
    </svg>
  )
}
