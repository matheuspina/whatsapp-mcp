"use client";

import { Webhook } from "@/lib/api";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Play, FileText, Pencil, Trash2, ExternalLink } from "lucide-react";

interface WebhookCardProps {
  webhook: Webhook;
  onTest: (id: string) => void;
  onLogs: (id: string) => void;
  onEdit: (webhook: Webhook) => void;
  onToggle: (id: string, enabled: boolean) => void;
  onDelete: (id: string, name: string) => void;
}

export function WebhookCard({ webhook, onTest, onLogs, onEdit, onToggle, onDelete }: WebhookCardProps) {
  const formatTriggers = (triggers: Webhook["triggers"]) => {
    if (!triggers || triggers.length === 0) return "Todas as mensagens";
    return triggers.map((t) => {
      if (t.trigger_type === "all") return "Todas as mensagens";
      const labels: Record<string, string> = { chat_jid: "Conversa", instance_jid: "Número", sender: "Remetente", keyword: "Palavra-chave", media_type: "Tipo de arquivo" };
      return `${labels[t.trigger_type] || "Condição"}: ${t.trigger_value}`;
    }).join(", ");
  };

  const formatDate = (dateString: string) => {
    if (!dateString) return "Não informado";
    try {
      const date = new Date(dateString);
      return date.toLocaleDateString() + " " + date.toLocaleTimeString();
    } catch {
      return "Data inválida";
    }
  };

  return (
    <Card className="relative">
      <CardHeader className="pb-3">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <h3 className="font-semibold leading-none">{webhook.name}</h3>
            <div className="flex min-w-0 items-center gap-2 text-sm text-muted-foreground">
              <ExternalLink className="h-3 w-3 shrink-0" />
              <span className="truncate">{webhook.webhook_url}</span>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Badge variant={webhook.enabled ? "default" : "secondary"}>
              {webhook.enabled ? "Ativo" : "Desativado"}
            </Badge>
            <Switch
              checked={webhook.enabled}
              onCheckedChange={(checked) => onToggle(webhook.id, checked)}
            />
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Condições:</span>
            <span className="text-right max-w-[200px] truncate">{formatTriggers(webhook.triggers)}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Criado em:</span>
            <span>{formatDate(webhook.created_at)}</span>
          </div>
        </div>
        
        <div className="flex gap-2 flex-wrap">
          <Button variant="outline" size="sm" onClick={() => onTest(webhook.id)}>
            <Play className="h-3 w-3 mr-1" />
            Testar
          </Button>
          <Button variant="outline" size="sm" onClick={() => onLogs(webhook.id)}>
            <FileText className="h-3 w-3 mr-1" />
            Histórico
          </Button>
          <Button variant="outline" size="sm" onClick={() => onEdit(webhook)}>
            <Pencil className="h-3 w-3 mr-1" />
            Editar
          </Button>
          <Button 
            variant="outline" 
            size="sm" 
            className="text-destructive hover:text-destructive"
            onClick={() => onDelete(webhook.id, webhook.name)}
          >
            <Trash2 className="h-3 w-3 mr-1" />
            Excluir
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
