import { APP_CONFIG } from "@/config/app-config";

export function BrandMark({ className }: { className?: string }) {
  return <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none" role="img" aria-label={APP_CONFIG.fullName} className={className}>
    <rect x="3" y="3" width="58" height="58" rx="12" fill="var(--sidebar)" stroke="var(--border)" />
    <g stroke="var(--primary)" strokeLinecap="round">
      <path d="M18 37a14 14 0 0 1 28 0" strokeWidth="4" />
      <path d="M13 39h38" strokeWidth="3" />
      <path d="M19 46h26M25 52h14" strokeWidth="2.5" />
    </g>
  </svg>;
}
