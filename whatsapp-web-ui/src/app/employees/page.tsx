"use client";

import { useEffect, useState, useMemo, useCallback } from "react";
import { Users, Plus, Pencil, Trash2, Loader2, Search, Building2, Mail } from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { WhatsAppAPI, Employee, Department } from "@/lib/api";

export default function EmployeesPage() {
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedDeptFilter, setSelectedDeptFilter] = useState<string>("all");

  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);
  const [editingEmp, setEditingEmp] = useState<Employee | null>(null);

  // Form states
  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [deptId, setDeptId] = useState<string>("none");
  const [email, setEmail] = useState("");
  const [active, setActive] = useState(true);

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadData = useCallback(async () => {
    try {
      setLoading(true);
      const [empData, deptData] = await Promise.all([
        api.getEmployees(),
        api.getDepartments(),
      ]);
      setEmployees(empData);
      setDepartments(deptData);
    } catch (err: unknown) {
      toast.error("Erro ao carregar colaboradores", {
        description: err instanceof Error ? err.message : "Falha na comunicação com o bridge",
      });
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    let ignore = false;
    Promise.all([api.getEmployees(), api.getDepartments()])
      .then(([empData, deptData]) => {
        if (!ignore) {
          setEmployees(empData);
          setDepartments(deptData);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar colaboradores", {
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
    setEditingEmp(null);
    setName("");
    setRole("");
    setDeptId("none");
    setEmail("");
    setActive(true);
    setDialogOpen(true);
  };

  const openEditDialog = (emp: Employee) => {
    setEditingEmp(emp);
    setName(emp.name);
    setRole(emp.role || "");
    setDeptId(emp.department_id ? emp.department_id.toString() : "none");
    setEmail(emp.email || "");
    setActive(emp.active);
    setDialogOpen(true);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      toast.error("O nome do colaborador é obrigatório.");
      return;
    }

    try {
      setSaving(true);
      const parsedDeptId = deptId !== "none" ? parseInt(deptId, 10) : null;
      const payload = {
        name: name.trim(),
        role: role.trim(),
        department_id: parsedDeptId,
        email: email.trim(),
      };

      if (editingEmp) {
        await api.updateEmployee(editingEmp.id, { ...payload, active });
        toast.success("Colaborador atualizado com sucesso.");
      } else {
        await api.createEmployee(payload);
        toast.success("Colaborador cadastrado com sucesso.");
      }
      setDialogOpen(false);
      loadData();
    } catch (err: unknown) {
      toast.error("Erro ao salvar colaborador", {
        description: err instanceof Error ? err.message : "Ocorreu um erro ao salvar",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteId) return;
    try {
      await api.deleteEmployee(deleteId);
      toast.success("Colaborador excluído com sucesso.");
      setDeleteId(null);
      loadData();
    } catch (err: unknown) {
      toast.error("Erro ao excluir colaborador", {
        description: err instanceof Error ? err.message : "Não foi possível excluir",
      });
    }
  };

  // Filtered employees
  const filteredEmployees = useMemo(() => {
    return employees.filter((emp) => {
      const matchesSearch =
        emp.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
        (emp.role && emp.role.toLowerCase().includes(searchQuery.toLowerCase())) ||
        (emp.email && emp.email.toLowerCase().includes(searchQuery.toLowerCase()));

      const matchesDept =
        selectedDeptFilter === "all" ||
        (selectedDeptFilter === "none" && !emp.department_id) ||
        (emp.department_id && emp.department_id.toString() === selectedDeptFilter);

      return matchesSearch && matchesDept;
    });
  }, [employees, searchQuery, selectedDeptFilter]);

  return (
    <PageContainer>
      <PageHeader
        title="Colaboradores"
        description="Gerencie os membros da equipe para desambiguação e resolução de contexto da IA."
        actions={
          <Button onClick={openCreateDialog} className="gap-2">
            <Plus className="size-4" />
            Novo Colaborador
          </Button>
        }
      />

      {/* Filters bar */}
      <div className="flex flex-col sm:flex-row items-center gap-3">
        <div className="relative flex-1 w-full">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Buscar por nome, cargo ou telefone..."
            className="pl-9"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
        </div>
        <div className="w-full sm:w-56">
          <Select value={selectedDeptFilter} onValueChange={setSelectedDeptFilter}>
            <SelectTrigger>
              <SelectValue placeholder="Filtrar por setor" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Todos os Setores</SelectItem>
              <SelectItem value="none">Sem Setor Definido</SelectItem>
              {departments.map((dept) => (
                <SelectItem key={dept.id} value={dept.id.toString()}>
                  {dept.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="size-8 animate-spin text-muted-foreground" />
        </div>
      ) : filteredEmployees.length === 0 ? (
        <Card className="flex flex-col items-center justify-center py-12 text-center">
          <div className="flex size-14 items-center justify-center rounded-full bg-muted">
            <Users className="size-7 text-muted-foreground" />
          </div>
          <CardTitle className="mt-4 text-lg">Nenhum colaborador encontrado</CardTitle>
          <CardDescription className="max-w-sm mt-1">
            {searchQuery || selectedDeptFilter !== "all"
              ? "Nenhum colaborador corresponde aos filtros selecionados."
              : "Cadastre os colaboradores da equipe para mapear aparelhos e histórico de atendimento."}
          </CardDescription>
          {!searchQuery && selectedDeptFilter === "all" && (
            <Button onClick={openCreateDialog} className="mt-6 gap-2">
              <Plus className="size-4" />
              Cadastrar Primeiro Colaborador
            </Button>
          )}
        </Card>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filteredEmployees.map((emp) => (
            <Card key={emp.id} className="relative overflow-hidden transition-all hover:border-primary/50">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div>
                    <CardTitle className="text-base font-semibold">{emp.name}</CardTitle>
                    {emp.role && (
                      <span className="text-xs text-muted-foreground font-normal block mt-0.5">
                        {emp.role}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-foreground"
                      onClick={() => openEditDialog(emp)}
                    >
                      <Pencil className="size-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 text-muted-foreground hover:text-destructive"
                      onClick={() => setDeleteId(emp.id)}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </div>
              </CardHeader>

              <CardContent className="space-y-3 pt-0">
                <div className="flex flex-wrap items-center gap-1.5">
                  {!emp.active && (
                    <Badge variant="outline" className="text-xs text-muted-foreground">
                      Inativo
                    </Badge>
                  )}
                  {emp.department_name ? (
                    <Badge variant="secondary" className="gap-1 text-xs">
                      <Building2 className="size-3" />
                      {emp.department_name}
                    </Badge>
                  ) : (
                    <Badge variant="outline" className="text-xs text-muted-foreground">
                      Sem setor
                    </Badge>
                  )}
                </div>

                {emp.email && (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Mail className="size-3.5" />
                    <span>{emp.email}</span>
                  </div>
                )}
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
              <DialogTitle>{editingEmp ? "Editar Colaborador" : "Novo Colaborador"}</DialogTitle>
              <DialogDescription>
                Informações para identificar o colaborador nas análises e buscas de conversas.
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="emp-name">Nome Completo *</Label>
                <Input
                  id="emp-name"
                  placeholder="Ex: João da Silva"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="emp-role">Cargo / Função</Label>
                <Input
                  id="emp-role"
                  placeholder="Ex: Vendedor Sênior, Suporte Técnico N1"
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                />
              </div>

              <div className="grid gap-2">
                <Label htmlFor="emp-dept">Setor / Departamento</Label>
                <Select value={deptId} onValueChange={setDeptId}>
                  <SelectTrigger id="emp-dept">
                    <SelectValue placeholder="Selecione um setor" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">Nenhum / Não Atribuído</SelectItem>
                    {departments.map((dept) => (
                      <SelectItem key={dept.id} value={dept.id.toString()}>
                        {dept.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-2">
                <Label htmlFor="emp-email">E-mail</Label>
                <Input
                  id="emp-email"
                  type="email"
                  placeholder="nome@empresa.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>

              {editingEmp && (
                <div className="flex items-center justify-between gap-3 rounded-md border p-3">
                  <div className="text-xs">
                    <p className="font-medium">Colaborador ativo</p>
                    <p className="text-muted-foreground">Inativos deixam de aparecer para a IA ao buscar pessoas.</p>
                  </div>
                  <Switch checked={active} onCheckedChange={setActive} aria-label="Colaborador ativo" />
                </div>
              )}
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                Cancelar
              </Button>
              <Button type="submit" disabled={saving}>
                {saving && <Loader2 className="mr-2 size-4 animate-spin" />}
                {editingEmp ? "Salvar Alterações" : "Cadastrar Colaborador"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Confirmação de Exclusão */}
      <AlertDialog open={deleteId !== null} onOpenChange={(open) => !open && setDeleteId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Excluir Colaborador</AlertDialogTitle>
            <AlertDialogDescription>
              Tem certeza que deseja remover este colaborador? As mensagens históricas continuarão salvas no sistema.
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
