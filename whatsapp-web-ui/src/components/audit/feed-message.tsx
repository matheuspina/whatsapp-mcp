"use client";

import { useState } from "react";
import {
  Building2,
  History,
  Pencil,
  Smartphone,
  Trash2,
  User,
  ArrowUpRight,
  ArrowDownLeft,
  Copy,
  Check,
  FileText,
  Image as ImageIcon,
  Mic,
  Video,
} from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { FeedMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

export function whenLabel(value: string): string {
  try {
    const d = new Date(value);
    return d.toLocaleString("pt-BR", {
      day: "2-digit",
      month: "2-digit",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return value;
  }
}

interface FeedMessageCardProps {
  message: FeedMessage;
  onShowVersions: (message: FeedMessage) => void;
}

function getMediaIcon(mediaType?: string) {
  if (!mediaType) return null;
  const mt = mediaType.toLowerCase();
  if (mt.includes("image") || mt.includes("foto")) return <ImageIcon className="size-3.5" />;
  if (mt.includes("audio") || mt.includes("voz")) return <Mic className="size-3.5" />;
  if (mt.includes("video")) return <Video className="size-3.5" />;
  return <FileText className="size-3.5" />;
}

export function FeedMessageCard({ message: m, onShowVersions }: FeedMessageCardProps) {
  const [copied, setCopied] = useState(false);

  const copyContent = async () => {
    if (!m.content) return;
    try {
      await navigator.clipboard.writeText(m.content);
      setCopied(true);
      toast.success("Texto copiado para a área de transferência");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Não foi possível copiar o texto");
    }
  };

  const senderInitial = m.is_from_me
    ? "EU"
    : (m.sender_name || m.sender || "W").slice(0, 2).toUpperCase();

  const chatDisplay = m.chat_name || m.chat_jid.split("@")[0];

  return (
    <Card
      className={cn(
        "group relative transition-all duration-200 hover:shadow-md",
        m.is_deleted_remote
          ? "border-destructive/40 bg-destructive/[0.02]"
          : m.is_from_me
          ? "border-l-4 border-l-emerald-500/80 bg-card"
          : "border-l-4 border-l-sky-500/80 bg-card"
      )}
    >
      <CardContent className="space-y-3 p-4">
        {/* Top Header */}
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 pb-2.5">
          <div className="flex items-center gap-2.5 min-w-0">
            <Avatar className={cn("size-8 text-xs font-semibold shrink-0", m.is_from_me ? "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400" : "bg-sky-500/15 text-sky-700 dark:text-sky-400")}>
              <AvatarFallback>{senderInitial}</AvatarFallback>
            </Avatar>

            <div className="min-w-0">
              <div className="flex items-center gap-1.5 flex-wrap">
                <span className="font-semibold text-sm text-foreground truncate max-w-[200px] sm:max-w-xs">
                  {m.is_from_me ? (
                    <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
                      <ArrowUpRight className="size-3.5 inline" />
                      Enviada {m.employee_name ? `(${m.employee_name})` : ""}
                    </span>
                  ) : (
                    <span className="flex items-center gap-1 text-sky-600 dark:text-sky-400">
                      <ArrowDownLeft className="size-3.5 inline" />
                      {m.sender_name || m.sender}
                    </span>
                  )}
                </span>

                <span className="text-xs text-muted-foreground">no chat</span>
                <span className="text-xs font-medium text-foreground bg-muted px-1.5 py-0.5 rounded truncate max-w-[180px]">
                  {chatDisplay}
                </span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-2 shrink-0">
            <span className="text-xs font-mono text-muted-foreground whitespace-nowrap">
              {whenLabel(m.timestamp)}
            </span>

            {m.content && (
              <Button
                variant="ghost"
                size="icon"
                className="size-7 opacity-70 group-hover:opacity-100 transition-opacity"
                onClick={copyContent}
                title="Copiar mensagem"
              >
                {copied ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
              </Button>
            )}
          </div>
        </div>

        {/* Message body */}
        <div className="text-sm leading-relaxed whitespace-pre-wrap break-words text-foreground selection:bg-primary/20">
          {m.content || (
            <span className="italic text-muted-foreground">
              {m.media_type ? `[Arquivo de mídia: ${m.media_type}]` : "[Mensagem sem conteúdo textual]"}
            </span>
          )}
        </div>

        {/* Media tag if any */}
        {m.media_type && (
          <div className="flex items-center gap-1.5">
            <Badge variant="outline" className="gap-1.5 text-xs font-normal">
              {getMediaIcon(m.media_type)}
              <span>Mídia: {m.media_type}</span>
            </Badge>
          </div>
        )}

        {/* Metadata badges & Audit indicators */}
        <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
          {/* Instance & Attribution tags */}
          <div className="flex flex-wrap items-center gap-1.5">
            {m.instance_alias && (
              <Badge variant="outline" className="gap-1 text-[11px] font-normal py-0.5">
                <Smartphone className="size-3 text-muted-foreground" />
                <span>{m.instance_alias}</span>
              </Badge>
            )}

            {m.department_name && (
              <Badge variant="secondary" className="gap-1 text-[11px] font-normal py-0.5">
                <Building2 className="size-3 text-muted-foreground" />
                <span>{m.department_name}</span>
              </Badge>
            )}

            {m.employee_name && (
              <Badge variant="secondary" className="gap-1 text-[11px] font-normal py-0.5">
                <User className="size-3 text-muted-foreground" />
                <span>{m.employee_name}</span>
              </Badge>
            )}
          </div>

          {/* Audit flags */}
          <div className="flex flex-wrap items-center gap-2">
            {m.is_deleted_remote && (
              <Badge variant="destructive" className="gap-1 text-[11px] py-0.5">
                <Trash2 className="size-3" />
                <span>Apagada pelo remetente</span>
                {m.deleted_at && <span className="opacity-90">({whenLabel(m.deleted_at)})</span>}
              </Badge>
            )}

            {m.is_edited && (
              <Badge variant="warning" className="gap-1 text-[11px] py-0.5">
                <Pencil className="size-3" />
                <span>Mensagem editada</span>
              </Badge>
            )}

            {(m.is_deleted_remote || m.is_edited) && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 gap-1 text-xs px-2 text-muted-foreground hover:text-foreground"
                onClick={() => onShowVersions(m)}
              >
                <History className="size-3" />
                Histórico de versões
              </Button>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
