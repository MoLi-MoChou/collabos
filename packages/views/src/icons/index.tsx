/** Minimal stroke icons (currentColor only, no raw colors). */
import type { SVGProps } from "react";

function I({ d, ...p }: SVGProps<SVGSVGElement> & { d: string }) {
  return (
    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden {...p}>
      <path d={d} />
    </svg>
  );
}
export const IconPanel = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M4 5h16v14H4zM15 5v14" />;
export const IconSend = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M12 19V5M6 11l6-6 6 6" />;
export const IconPlus = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M12 5v14M5 12h14" />;
export const IconSun = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M12 4v1.5M12 18.5V20M4 12h1.5M18.5 12H20M6.3 6.3l1 1M16.7 16.7l1 1M6.3 17.7l1-1M16.7 7.3l1-1M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z" />;
export const IconMoon = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M19 14.5A7.5 7.5 0 0 1 9.5 5a7.5 7.5 0 1 0 9.5 9.5z" />;
export const IconLogout = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4M10 16l-4-4 4-4M6 12h10" />;
export const IconHash = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M9 4 7 20M17 4l-2 16M4 9h16M4 15h16" />;
export const IconDevice = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M4 5h16v11H4zM9 20h6M12 16v4" />;
export const IconClose = (p: SVGProps<SVGSVGElement>) => <I {...p} d="M6 6l12 12M18 6 6 18" />;
export const IconLogo = (p: SVGProps<SVGSVGElement>) => (
  <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden {...p}>
    <rect x="2" y="5" width="13" height="11" rx="4" fill="currentColor" opacity="0.35" />
    <rect x="8" y="9" width="14" height="11" rx="4" fill="currentColor" />
  </svg>
);
