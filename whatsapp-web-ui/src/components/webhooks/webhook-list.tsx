"use client";

import { useState, useEffect } from "react";
import { Webhook, WhatsAppAPI, getErrorMessage } from "@/lib/api";

import { PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { WebhookCard } from "./webhook-card";
import { WebhookForm, WebhookFormData } from "./webhook-form";
import { WebhookLogs } from "./webhook-logs";
import { Plus, RefreshCw, Loader2, Webhook as WebhookIcon } from "lucide-react";
import { toast } from "sonner";

export function WebhookList() {
  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logsWebhook, setLogsWebhook] = useState<{ id: string; name: string } | null>(null);
  const [deleteDialog, setDeleteDialog] = useState<{ id: string; name: string } | null>(null);
  const [submitting, setSubmitting] = useState(false);

  // Bumped every time the form dialog opens, so the form remounts with fresh state.
  const [formKey, setFormKey] = useState(0);

  const loadWebhooks = async () => {
    try {
      const api = new WhatsAppAPI();
      setWebhooks(await api.getWebhooks());
    } catch (error) {
      const { title, description } = getErrorMessage(error);
      toast.error(title, { description });
      setWebhooks([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    let ignore = false;
    new WhatsAppAPI()
      .getWebhooks()
      .then((data) => {
        if (!ignore) setWebhooks(data);
      })
      .catch((error: unknown) => {
        if (ignore) return;
        const { title, description } = getErrorMessage(error);
        toast.error(title, { description });
        setWebhooks([]);
      })
      .finally(() => {
        if (!ignore) setLoading(false);
      });
    return () => {
      ignore = true;
    };
  }, []);

  const handleCreate = () => {
    setEditingWebhook(null);
    setFormKey((k) => k + 1);
    setFormOpen(true);
  };

  const handleEdit = (webhook: Webhook) => {
    setEditingWebhook(webhook);
    setFormKey((k) => k + 1);
    setFormOpen(true);
  };

  const handleFormSubmit = async (data: WebhookFormData) => {
    setSubmitting(true);
    try {
      const api = new WhatsAppAPI();
      if (editingWebhook) {
        await api.updateWebhook(editingWebhook.id, data);
        toast.success("Webhook atualizado");
      } else {
        await api.createWebhook(data);
        toast.success("Webhook criado");
      }
      setFormOpen(false);
      loadWebhooks();
    } catch (error) {
      const { title, description } = getErrorMessage(error);
      toast.error(title, { description });
    } finally {
      setSubmitting(false);
    }
  };

  const handleToggle = async (id: string, enabled: boolean) => {
    try {
      const api = new WhatsAppAPI();
      await api.toggleWebhook(id, enabled);
      toast.success(enabled ? "Webhook ativado" : "Webhook desativado");
      loadWebhooks();
    } catch (error) {
      const { title, description } = getErrorMessage(error);
      toast.error(title, { description });
    }
  };

  const handleTest = async (id: string) => {
    try {
      const api = new WhatsAppAPI();
      await api.testWebhook(id);
      toast.success("Teste enviado com sucesso");
    } catch (error) {
      const { title, description } = getErrorMessage(error);
      toast.error(title, { description });
    }
  };

  const handleLogs = (id: string) => {
    const webhook = webhooks.find((w) => w.id === id);
    setLogsWebhook({ id, name: webhook?.name || "Webhook" });
    setLogsOpen(true);
  };

  const handleDelete = async () => {
    if (!deleteDialog) return;

    try {
      const api = new WhatsAppAPI();
      await api.deleteWebhook(deleteDialog.id);
      toast.success("Webhook excluído");
      setDeleteDialog(null);
      loadWebhooks();
    } catch (error) {
      const { title, description } = getErrorMessage(error);
      toast.error(title, { description });
    }
  };

  return (
    <>
      <PageHeader
        title="Webhooks"
        description="Envie novas mensagens para outros sistemas automaticamente"
        actions={
          <>
            <Button variant="outline" onClick={loadWebhooks} disabled={loading}>
              <RefreshCw className={"h-4 w-4 mr-2" + (loading ? " animate-spin" : "")} />
              Atualizar
            </Button>
            <Button onClick={handleCreate}>
              <Plus className="h-4 w-4 mr-2" />
              Novo webhook
            </Button>
          </>
        }
      />

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      ) : webhooks.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-muted-foreground border rounded-lg bg-muted/30">
          <WebhookIcon className="h-10 w-10 mb-4" />
          <h3 className="text-base font-medium mb-2">Nenhum webhook cadastrado</h3>
          <p className="text-sm mb-4">Crie um webhook para começar a receber mensagens em outro sistema.</p>
          <Button onClick={handleCreate}>
            <Plus className="h-4 w-4 mr-2" />
            Criar webhook
          </Button>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
          {webhooks.map((webhook) => (
            <WebhookCard
              key={webhook.id}
              webhook={webhook}
              onTest={handleTest}
              onLogs={handleLogs}
              onEdit={handleEdit}
              onToggle={handleToggle}
              onDelete={(id, name) => setDeleteDialog({ id, name })}
            />
          ))}
        </div>
      )}

      <WebhookForm
        key={formKey}
        open={formOpen}
        onOpenChange={setFormOpen}
        webhook={editingWebhook}
        onSubmit={handleFormSubmit}
        isLoading={submitting}
      />

      <WebhookLogs
        key={logsWebhook?.id ?? "none"}
        open={logsOpen}
        onOpenChange={setLogsOpen}
        webhookId={logsWebhook?.id || null}
        webhookName={logsWebhook?.name || ""}
      />

      <AlertDialog open={!!deleteDialog} onOpenChange={() => setDeleteDialog(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
          <AlertDialogTitle>Excluir webhook</AlertDialogTitle>
            <AlertDialogDescription>
              Deseja excluir &quot;{deleteDialog?.name}&quot;? O histórico de entregas também será excluído e essa ação não pode ser desfeita.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={handleDelete} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
              Excluir
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
