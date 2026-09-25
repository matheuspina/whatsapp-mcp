"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";

interface CodeBlockProps {
  code: string;
  label?: string;
}

/** A snippet with a copy button. */
export function CodeBlock({ code, label }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // navigator.clipboard is missing on plain http pages that are not localhost.
      toast.error("Could not copy", { description: "Select the text and copy it by hand." });
    }
  };

  return (
    <div className="space-y-1.5">
      {label && <p className="text-xs text-muted-foreground">{label}</p>}
      <div className="relative rounded-md border bg-muted/50">
        <pre className="overflow-x-auto p-3 pr-12 font-mono text-xs leading-relaxed">
          <code>{code}</code>
        </pre>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="absolute right-1.5 top-1.5"
          onClick={copy}
          aria-label="Copy to clipboard"
        >
          {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
        </Button>
      </div>
    </div>
  );
}
