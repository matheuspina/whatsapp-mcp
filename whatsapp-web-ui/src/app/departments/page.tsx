"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { Building2, Plus, Pencil, Trash2, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { WhatsAppAPI, Department } from "@/lib/api";

export default function DepartmentsPage() {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);
  const [editingDept, setEditingDept] = useState<Department | null>(null);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadDepartments = useCallback(async () => {
    try {
      setLoading(true);
      const data = await api.getDepartments();
      setDepartments(data);
    } catch (err: unknown) {
      toast.error("Erro ao carregar setores", {
        description: err instanceof Error ? err.message : "Falha na comunicação com o bridge",
      });
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    let ignore = false;
    api
      .getDepartments()
      .then((data) => {
        if (!ignore) {
          setDepartments(data);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar setores", {
            description: err instanceof Error ? err.message : "Falha na comunicação com o bridge",
          });
          setLoading(false);
        }
      });
    return () => {
      ignore = true;
    };
  }, [api]);

  const openCreateDialog = () => {
    setEditingDept(null);
    setName("");
    setDescription("");
    setDialogOpen(true);
  };

  const openEditDialog = (dept: Department) => {
    setEditingDept(dept);
    setName(dept.name);
    setDescription(dept.description || "");
    setDialogOpen(true);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      toast.error("O nome do setor é obrigatório.");
      return;
    }

    try {
      setSaving(true);
      if (editingDept) {
        await api.updateDepartment(editingDept.id, { name: name.trim(), description: description.trim() });
        toast.success("Setor atualizado com sucesso.");
      } else {
        await api.createDepartment({ name: name.trim(), description: description.trim() });
        toast.success("Setor criado com sucesso.");
      }
      setDialogOpen(false);
      loadDepartments();
    } catch (err: unknown) {
      toast.error("Erro ao salvar setor", {
        description: err instanceof Error ? err.message : "Ocorreu um erro ao salvar",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteId) return;
    try {
      await api.deleteDepartment(deleteId);
      toast.success("Setor excluído com sucesso.");
      setDeleteId(null);
      loadDepartments();
    } catch (err: unknown) {
      toast.error("Erro ao excluir setor", {
        description: err instanceof Error ? err.message : "Não foi possível excluir o setor",
      });
    }
  };

  return (
    <PageContainer>
      <PageHeader
        title="Setores & Departamentos"
        description="Organize a empresa em departamentos para contexto semântico da IA e governança de conversas."
        actions={
          <Button onClick={openCreateDialog} className="gap-2">
            <Plus className="size-4" />
            Novo Setor
          </Button>
        }
      />

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : departments.length === 0 ? (
        <Card className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex size-14 items-center justify-center rounded-full bg-muted">
            <Building2 className="size-7 text-muted-foreground" />
          </div>
          <CardTitle className="mt-4 text-lg">Nenhum setor cadastrado</CardTitle>
          <CardDescription className="max-w-sm mt-1">
            Cadastre setores como Comercial, Suporte, Financeiro ou Diretoria para associar colaboradores e conversas.
          </CardDescription>
          <Button onClick={openCreateDialog} className="mt-6 gap-2">
            <Plus className="size-4" />
            Cadastrar Primeiro Setor
          </Button>
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {departments.map((dept) => (
            <Card key={dept.id} className="relative overflow-hidden transition-all hover:border-primary/50">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2">
                    <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                      <Building2 className="size-4" />
                    </div>
                    <CardTitle className="text-base font-semibold">{dept.name}</CardTitle>
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-foreground"
                      onClick={() => openEditDialog(dept)}
                    >
                      <Pencil className="size-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-destructive"
                      onClick={() => setDeleteId(dept.id)}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </div>
                <CardDescription className="mt-2 min-h-10 text-xs">
                  {dept.description || "Nenhuma descrição informada."}
                </CardDescription>
              </CardHeader>
              <CardContent className="pt-0">
                <div className="text-[11px] text-muted-foreground">
                  Criado em: {new Date(dept.created_at).toLocaleDateString("pt-BR")}
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Dialog Criação / Edição */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <form onSubmit={handleSave}>
            <DialogHeader>
              <DialogTitle>{editingDept ? "Editar Setor" : "Novo Setor"}</DialogTitle>
              <DialogDescription>
                {editingDept
                  ? "Atualize as informações do departamento."
                  : "Defina o nome e a finalidade deste setor na empresa."}
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="dept-name">Nome do Setor *</Label>
                <Input
                  id="dept-name"
                  placeholder="Ex: Comercial, Atendimento, Diretoria"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="dept-desc">Descrição / Atribuição</Label>
                <Input
                  id="dept-desc"
                  placeholder="Ex: Responsável por prospecção, negociações e fechamentos"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                {editingDept ? "Salvar Alterações" : "Criar Setor"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmação de Exclusão */}
      <AlertDialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Excluir Setor</AlertDialogTitle>
            <AlertDialogDescription>
              Tem certeza que deseja excluir este setor? Os colaboradores vinculados a ele ficarão sem setor atribuído.
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
    </PageContainer>
  );
}
