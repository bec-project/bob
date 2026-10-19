import type { ReactNode, SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function make(paths: ReactNode) {
  return function Icon({ size = 16, ...props }: IconProps) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={1.8}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        {...props}
      >
        {paths}
      </svg>
    );
  };
}

export const IconPlus = make(<path d="M12 5v14M5 12h14" />);
export const IconSend = make(<path d="M5 12h13M13 6l6 6-6 6" />);
export const IconStop = make(<rect x="6" y="6" width="12" height="12" rx="2" />);
export const IconTrash = make(
  <>
    <path d="M4 7h16M10 11v6M14 11v6" />
    <path d="M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3" />
  </>,
);
export const IconEdit = make(<path d="M4 20h4L19 9l-4-4L4 16v4zM14 6l4 4" />);
export const IconSettings = make(
  <>
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z" />
  </>,
);
export const IconChevron = make(<path d="M9 6l6 6-6 6" />);
export const IconCheck = make(<path d="M5 12l5 5L20 7" />);
export const IconX = make(<path d="M6 6l12 12M18 6L6 18" />);
export const IconBrain = make(
  <>
    <path d="M9 4a3 3 0 0 0-3 3v.5A3 3 0 0 0 4 10.4 3 3 0 0 0 5 15a3 3 0 0 0 4 4.8V4z" />
    <path d="M15 4a3 3 0 0 1 3 3v.5a3 3 0 0 1 2 2.9 3 3 0 0 1-1 4.6 3 3 0 0 1-4 4.8V4z" />
  </>,
);
export const IconTool = make(
  <path d="M14.7 6.3a4 4 0 0 0-5.4 5.4L4 17l3 3 5.3-5.3a4 4 0 0 0 5.4-5.4l-2.5 2.5-2.4-.6-.6-2.4 2.5-2.5z" />,
);
export const IconCopy = make(
  <>
    <rect x="9" y="9" width="11" height="11" rx="2" />
    <path d="M5 15V5a2 2 0 0 1 2-2h8" />
  </>,
);
export const IconSidebar = make(
  <>
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M9 4v16" />
  </>,
);
export const IconRefresh = make(<path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7" />);
export const IconDown = make(<path d="M12 5v14M6 13l6 6 6-6" />);
export const IconClock = make(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </>,
);
export const IconShield = make(<path d="M12 3l8 3v6c0 4.5-3.4 8.3-8 9-4.6-.7-8-4.5-8-9V6l8-3z" />);
export const IconQuestion = make(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M9.5 9.5a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.6V14M12 17.5v.01" />
  </>,
);
export const IconChip = make(
  <>
    <rect x="6" y="6" width="12" height="12" rx="2" />
    <path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4" />
  </>,
);
export const IconLogo = make(
  <>
    <rect x="4" y="7" width="16" height="12" rx="4" />
    <circle cx="9.5" cy="13" r="1.4" fill="currentColor" stroke="none" />
    <circle cx="14.5" cy="13" r="1.4" fill="currentColor" stroke="none" />
    <path d="M12 7V4M10 4h4" />
  </>,
);
export const IconAlert = make(
  <>
    <path d="M12 3l9.5 16.5H2.5L12 3z" />
    <path d="M12 10v4M12 17v.01" />
  </>,
);
export const IconQueue = make(<path d="M4 6h16M4 12h16M4 18h10" />);
export const IconFile = make(
  <>
    <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5z" />
    <path d="M14 3v5h5" />
  </>,
);
export const IconUndo = make(<path d="M9 14L4 9l5-5M4 9h10a6 6 0 0 1 0 12h-3" />);
export const IconFork = make(
  <>
    <circle cx="6" cy="5" r="2" />
    <circle cx="18" cy="5" r="2" />
    <circle cx="12" cy="19" r="2" />
    <path d="M6 7v2a4 4 0 0 0 4 4h4a4 4 0 0 0 4-4V7M12 13v4" />
  </>,
);
export const IconDownload = make(<path d="M12 4v11M7 10l5 5 5-5M5 20h14" />);
export const IconList = make(<path d="M9 6h11M9 12h11M9 18h11M4 6h.01M4 12h.01M4 18h.01" />);
export const IconCompress = make(<path d="M4 9h5V4M20 9h-5V4M4 15h5v5M20 15h-5v5" />);
