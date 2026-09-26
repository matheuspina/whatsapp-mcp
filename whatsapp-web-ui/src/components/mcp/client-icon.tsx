import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

interface ClientIconProps {
  id: string;
  className?: string;
}

interface ClientMarkProps {
  children: ReactNode;
  className?: string;
  color?: string;
  fill?: string;
}

function ClientMark({ children, className, color = "text-foreground", fill = "none" }: ClientMarkProps) {
  return (
    <svg
      aria-hidden="true"
      className={cn("size-4 shrink-0", color, className)}
      fill={fill}
      viewBox="0 0 24 24"
      xmlns="http://www.w3.org/2000/svg"
    >
      {children}
    </svg>
  );
}

/** Official or brand-specific marks used by the MCP client selector. */
export function ClientIcon({ id, className }: ClientIconProps) {
  switch (id) {
    case "claude-code":
    case "claude":
      return (
        <ClientMark className={className} color="text-[#D97757]" fill="currentColor">
          <path d="M21 10.5h3v3h-3v3h-1.5v3H18v-3h-1.5v3H15v-3H9v3H7.5v-3H6v3H4.5v-3H3v-3H0v-3h3v-6h18Zm-15 0h1.5v-3H6Zm10.5 0H18v-3h-1.5z" />
        </ClientMark>
      );
    case "codex":
    case "chatgpt":
      return (
        <ClientMark className={className} color="text-foreground" fill="currentColor">
          <path d="M22.282 9.821a6 6 0 0 0-.516-4.91a6.05 6.05 0 0 0-6.51-2.9A6.065 6.065 0 0 0 4.981 4.18a6 6 0 0 0-3.998 2.9a6.05 6.05 0 0 0 .743 7.097a5.98 5.98 0 0 0 .51 4.911a6.05 6.05 0 0 0 6.515 2.9A6 6 0 0 0 13.26 24a6.06 6.06 0 0 0 5.772-4.206a6 6 0 0 0 3.997-2.9a6.06 6.06 0 0 0-.747-7.073M13.26 22.43a4.48 4.48 0 0 1-2.876-1.04l.141-.081l4.779-2.758a.8.8 0 0 0 .392-.681v-6.737l2.02 1.168a.07.07 0 0 1 .038.052v5.583a4.504 4.504 0 0 1-4.494 4.494M3.6 18.304a4.47 4.47 0 0 1-.535-3.014l.142.085l4.783 2.759a.77.77 0 0 0 .78 0l5.843-3.369v2.332a.08.08 0 0 1-.033.062L9.74 19.95a4.5 4.5 0 0 1-6.14-1.646M2.34 7.896a4.5 4.5 0 0 1 2.366-1.973V11.6a.77.77 0 0 0 .388.677l5.815 3.354l-2.02 1.168a.08.08 0 0 1-.071 0l-4.83-2.786A4.504 4.504 0 0 1 2.34 7.872zm16.597 3.855l-5.833-3.387L15.119 7.2a.08.08 0 0 1 .071 0l4.83 2.791a4.494 4.494 0 0 1-.676 8.105v-5.678a.79.79 0 0 0-.407-.667m2.01-3.023l-.141-.085l-4.774-2.782a.78.78 0 0 0-.785 0L9.409 9.23V6.897a.07.07 0 0 1 .028-.061l4.83-2.787a4.5 4.5 0 0 1 6.68 4.66zm-12.64 4.135l-2.02-1.164a.08.08 0 0 1-.038-.057V6.075a4.5 4.5 0 0 1 7.375-3.453l-.142.08L8.704 5.46a.8.8 0 0 0-.393.681zm1.097-2.365l2.602-1.5l2.607 1.5v2.999l-2.597 1.5l-2.607-1.5Z" />
        </ClientMark>
      );
    case "cursor":
      return (
        <ClientMark className={className} color="text-foreground" fill="currentColor">
          <path d="M11.503.131 1.891 5.678a.84.84 0 0 0-.42.726v11.188c0 .3.162.575.42.724l9.609 5.55a1 1 0 0 0 .998 0l9.61-5.55a.84.84 0 0 0 .42-.724V6.404a.84.84 0 0 0-.42-.726L12.497.131a1.01 1.01 0 0 0-.996 0M2.657 6.338h18.55c.263 0 .43.287.297.515L12.23 22.918c-.062.107-.229.064-.229-.06V12.335a.59.59 0 0 0-.295-.51l-9.11-5.257c-.109-.063-.064-.23.061-.23" />
        </ClientMark>
      );
    case "vscode":
      return (
        <ClientMark className={className} color="text-[#23A8F2]" fill="currentColor">
          <path d="M23.15 2.587 18.21.21a1.49 1.49 0 0 0-1.705.29l-9.46 8.63l-4.12-3.128a1 1 0 0 0-1.276.057L.327 7.261A1 1 0 0 0 .326 8.74L3.899 12L.326 15.26a1 1 0 0 0 .001 1.479L1.65 17.94a1 1 0 0 0 1.276.057l4.12-3.128l9.46 8.63a1.49 1.49 0 0 0 1.704.29l4.942-2.377A1.5 1.5 0 0 0 24 20.06V3.939a1.5 1.5 0 0 0-.85-1.352m-5.146 14.861L10.826 12l7.178-5.448z" />
        </ClientMark>
      );
    case "windsurf":
      return (
        <ClientMark className={className} color="text-[#45B8AC]" fill="currentColor">
          <path d="M23.55 5.067c-1.2038-.002-2.1806.973-2.1806 2.1765v4.8676c0 .972-.8035 1.7594-1.7597 1.7594-.568 0-1.1352-.286-1.4718-.7659l-4.9713-7.1003c-.4125-.5896-1.0837-.941-1.8103-.941-1.1334 0-2.1533.9635-2.1533 2.153v4.8957c0 .972-.7969 1.7594-1.7596 1.7594-.57 0-1.1363-.286-1.4728-.7658L.4076 5.1598C.2822 4.9798 0 5.0688 0 5.2882v4.2452c0 .2147.0656.4228.1884.599l5.4748 7.8183c.3234.462.8006.8052 1.3509.9298 1.3771.313 2.6446-.747 2.6446-2.0977v-4.893c0-.972.7875-1.7593 1.7596-1.7593h.003a1.798 1.798 0 0 1 1.4718.7658l4.9723 7.0994c.4135.5905 1.05.941 1.8093.941 1.1587 0 2.1515-.9645 2.1515-2.153v-4.8948c0-.972.7875-1.7594 1.7596-1.7594h.194a.22.22 0 0 0 .2204-.2202v-4.622a.22.22 0 0 0-.2203-.2203Z" />
        </ClientMark>
      );
    case "antigravity":
      return (
        <ClientMark className={className} color="text-[#4D8DFF]" fill="currentColor">
          <path d="m12 2 3.2 7.2L22 12l-6.8 2.8L12 22l-3.2-7.2L2 12l6.8-2.8Z" />
          <path d="m18.5 3 .8 1.7L21 5.5l-1.7.8-.8 1.7-.8-1.7-1.7-.8 1.7-.8Z" fill="#EA4335" />
        </ClientMark>
      );
    case "gemini-cli":
      return (
        <ClientMark className={className} color="text-[#4285F4]" fill="currentColor">
          <path d="M4 5.5h16v13H4z" opacity=".18" />
          <path d="m8 8 4 4-4 4M13.5 16h3" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.7" />
          <path d="M4 5.5h16v13H4z" stroke="currentColor" strokeWidth="1.5" />
          <path d="M17.5 5.5v3" stroke="#EA4335" strokeLinecap="round" strokeWidth="1.7" />
        </ClientMark>
      );
    case "grok":
      return (
        <ClientMark className={className} color="text-foreground" fill="currentColor">
          <path d="M3.3 12.3c0-4.9 3.8-8.7 8.7-8.7 3.4 0 6.4 1.9 7.8 4.8l-2.2 1.1a6.3 6.3 0 1 0 .3 5.9l2.2 1.1A8.7 8.7 0 1 1 3.3 12.3Z" />
          <path d="m8.4 15.9 7.2-7.5h2.1l-7.2 7.5Z" fill="var(--background)" />
        </ClientMark>
      );
    case "github-copilot":
      return (
        <ClientMark className={className} color="text-foreground" fill="currentColor">
          <path d="M23.922 16.997C23.061 18.492 18.063 22.02 12 22.02 5.937 22.02.939 18.492.078 16.997A.641.641 0 0 1 0 16.741v-2.869a.883.883 0 0 1 .053-.22c.372-.935 1.347-2.292 2.605-2.656.167-.429.414-1.055.644-1.517a10.098 10.098 0 0 1-.052-1.086c0-1.331.282-2.499 1.132-3.368.397-.406.89-.717 1.474-.952C7.255 2.937 9.248 1.98 11.978 1.98c2.731 0 4.767.957 6.166 2.093.584.235 1.077.546 1.474.952.85.869 1.132 2.037 1.132 3.368 0 .368-.014.733-.052 1.086.23.462.477 1.088.644 1.517 1.258.364 2.233 1.721 2.605 2.656a.841.841 0 0 1 .053.22v2.869a.641.641 0 0 1-.078.256Z" />
        </ClientMark>
      );
    case "opencode":
      return (
        <ClientMark className={className} color="text-foreground" fill="currentColor">
          <path d="M22 24H2V0h20zM17 4.8H7v14.4h10z" />
        </ClientMark>
      );
    default:
      return (
        <ClientMark className={className} color="text-muted-foreground">
          <path d="M6.5 18.5h11a4 4 0 0 0 .6-7.95A6.5 6.5 0 0 0 5.6 9.4a4.5 4.5 0 0 0 .9 9.1Z" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.7" />
        </ClientMark>
      );
  }
}
