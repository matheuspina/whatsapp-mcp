"use client";

import { useState } from "react";
import { Check, Copy, Terminal } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface CodeBlockProps {
  code: string;
  label?: string;
  language?: string;
  className?: string;
}

export function CodeBlock({ code, label, language, className }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      toast.success("Copiado para a área de transferência");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Não foi possível copiar", { description: "Selecione o texto manualmente para copiar." });
    }
  };

  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-center justify-between text-xs text-muted-foreground px-0.5">
        {label ? (
          <span className="font-medium text-foreground">{label}</span>
        ) : (
          <span className="flex items-center gap-1 font-mono text-[11px]">
            <Terminal className="size-3" />
            Configuração
          </span>
        )}
        {language && (
          <span className="uppercase text-[10px] font-mono tracking-wider text-muted-foreground bg-muted px-1.5 py-0.5 rounded">
            {language}
          </span>
        )}
      </div>

      <div className="relative rounded-lg border border-border/80 bg-zinc-950 text-zinc-100 shadow-sm overflow-hidden dark:bg-zinc-900/90">
        <div className="flex items-center justify-between border-b border-zinc-800/80 bg-zinc-900/60 px-3 py-1.5 text-[11px] font-mono text-zinc-400">
          <span>instruções</span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-6 gap-1.5 px-2 text-xs text-zinc-300 hover:text-white hover:bg-zinc-800"
            onClick={copy}
            aria-label="Copiar código"
          >
            {copied ? (
              <>
                <Check className="size-3 text-emerald-400" />
                <span className="text-emerald-400 font-medium">Copiado!</span>
              </>
            ) : (
              <>
                <Copy className="size-3" />
                <span>Copiar</span>
              </>
            )}
          </Button>
        </div>

        <pre className="overflow-x-auto p-3 font-mono text-xs leading-relaxed text-zinc-200">
          <code>{code}</code>
        </pre>
      </div>
    </div>
  );
}
