import { Building2, History, Pencil, Smartphone, Trash2, User } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { FeedMessage } from "@/lib/api";

export function whenLabel(value: string): string {
  return new Date(value).toLocaleString("pt-BR");
}

interface FeedMessageCardProps {
  message: FeedMessage;
  onShowVersions: (message: FeedMessage) => void;
}

/** One captured message: who wrote it, which number and person held it, and its audit state (edited, revoked). */
export function FeedMessageCard({ message: m, onShowVersions }: FeedMessageCardProps) {
  return (
    <Card className={m.is_deleted_remote ? "border-destructive/40" : ""}>
      <CardContent className="space-y-2 py-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{m.is_from_me ? "Enviada" : m.sender_name || m.sender}</span>
          <span>em {m.chat_name || m.chat_jid.split("@")[0]}</span>
          <span>{whenLabel(m.timestamp)}</span>
          {m.instance_alias && (
            <Badge variant="outline" className="gap-1">
              <Smartphone className="size-3" />
              {m.instance_alias}
            </Badge>
          )}
          {m.employee_name && (
            <Badge variant="secondary" className="gap-1">
              <User className="size-3" />
              {m.employee_name}
            </Badge>
          )}
          {m.department_name && (
            <Badge variant="secondary" className="gap-1">
              <Building2 className="size-3" />
              {m.department_name}
            </Badge>
          )}
        </div>

        <p className="whitespace-pre-wrap break-words text-sm">{m.content || (m.media_type ? `[${m.media_type}]` : "")}</p>

        {(m.is_deleted_remote || m.is_edited) && (
          <div className="flex flex-wrap items-center gap-2">
            {m.is_deleted_remote && (
              <Badge variant="destructive" className="gap-1">
                <Trash2 className="size-3" />
                Apagada pelo remetente
                {m.deleted_at ? ` em ${whenLabel(m.deleted_at)}` : ""}
              </Badge>
            )}
            {m.is_edited && (
              <Badge variant="warning" className="gap-1">
                <Pencil className="size-3" />
                Editada
              </Badge>
            )}
            <Button variant="ghost" size="sm" className="h-7 gap-1.5 text-xs" onClick={() => onShowVersions(m)}>
              <History className="size-3.5" />
              Ver versões anteriores
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
