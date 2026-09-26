"use client";

import { useState } from "react";
import { Webhook, WebhookTrigger } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { TriggerBuilder } from "./trigger-builder";

interface WebhookFormProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  webhook?: Webhook | null;
  onSubmit: (data: WebhookFormData) => void;
  isLoading?: boolean;
}

export interface WebhookFormData {
  name: string;
  webhook_url: string;
  secret_token: string;
  enabled: boolean;
  triggers: WebhookTrigger[];
}

const defaultTrigger: WebhookTrigger = {
  trigger_type: "all",
  trigger_value: "",
  match_type: "exact",
  enabled: true,
};

function initialFormData(webhook?: Webhook | null): WebhookFormData {
  if (webhook) {
    return {
      name: webhook.name,
      webhook_url: webhook.webhook_url,
      secret_token: webhook.secret_token || "",
      enabled: webhook.enabled,
      triggers: webhook.triggers?.length ? webhook.triggers : [defaultTrigger],
    };
  }
  return { name: "", webhook_url: "", secret_token: "", enabled: true, triggers: [defaultTrigger] };
}

/**
 * The form starts from `webhook` when it mounts. The parent gives it a new `key` each time the dialog is
 * opened, so a cancelled edit never leaks into the next one.
 */
export function WebhookForm({ open, onOpenChange, webhook, onSubmit, isLoading }: WebhookFormProps) {
  const [formData, setFormData] = useState<WebhookFormData>(() => initialFormData(webhook));
  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = (): boolean => {
    const newErrors: Record<string, string> = {};

    if (!formData.name.trim()) {
      newErrors.name = "Informe um nome";
    }

    if (!formData.webhook_url.trim()) {
      newErrors.webhook_url = "Informe uma URL";
    } else {
      try {
        new URL(formData.webhook_url);
      } catch {
        newErrors.webhook_url = "Informe uma URL válida";
      }
    }

    const validTriggers = formData.triggers.filter(
      (t) => t.trigger_type === "all" || t.trigger_value.trim()
    );
    if (validTriggers.length === 0) {
      newErrors.triggers = "Adicione pelo menos uma condição válida";
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (validate()) {
      onSubmit(formData);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{webhook ? "Editar webhook" : "Criar webhook"}</DialogTitle>
          <DialogDescription>
            {webhook ? "Atualize os dados deste webhook" : "Informe onde as mensagens devem ser entregues"}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="name">Nome</Label>
            <Input
              id="name"
              placeholder="Ex.: Notificações de atendimento"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            />
            {errors.name && <p className="text-sm text-destructive">{errors.name}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="webhook_url">Endereço de destino</Label>
            <Input
              id="webhook_url"
              type="url"
              placeholder="https://example.com/webhook"
              value={formData.webhook_url}
              onChange={(e) => setFormData({ ...formData, webhook_url: e.target.value })}
            />
            {errors.webhook_url && <p className="text-sm text-destructive">{errors.webhook_url}</p>}
          </div>

          <div className="space-y-2">
            <Label htmlFor="secret_token">Chave de segurança (opcional)</Label>
            <Input
              id="secret_token"
              type="password"
              placeholder="Usada para confirmar a origem do envio"
              value={formData.secret_token}
              onChange={(e) => setFormData({ ...formData, secret_token: e.target.value })}
            />
          </div>

          <div className="flex items-center gap-2">
            <Switch
              id="enabled"
              checked={formData.enabled}
              onCheckedChange={(checked) => setFormData({ ...formData, enabled: checked })}
            />
            <Label htmlFor="enabled">Ativo</Label>
          </div>

          <TriggerBuilder
            triggers={formData.triggers}
            onChange={(triggers) => setFormData({ ...formData, triggers })}
          />
          {errors.triggers && <p className="text-sm text-destructive">{errors.triggers}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancelar
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? "Salvando..." : webhook ? "Salvar alterações" : "Criar webhook"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
