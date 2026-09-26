"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import {
  Building2,
  Plus,
  Pencil,
  Trash2,
  Loader2,
  Users,
  Search,
  LayoutGrid,
  Table as TableIcon,
  MoreHorizontal,
  Calendar,
  X,
  Briefcase,
} from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { WhatsAppAPI, Department, Employee } from "@/lib/api";

export default function DepartmentsPage() {
  const [departments, setDepartments] = useState<Department[]>([]);
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [viewMode, setViewMode] = useState<"grid" | "table">("grid");

  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [saving, setSaving] = useState(false);
  const [editingDept, setEditingDept] = useState<Department | null>(null);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const api = useMemo(() => new WhatsAppAPI(), []);

  const loadData = useCallback(async () => {
    try {
      setLoading(true);
      const [deptData, empData] = await Promise.all([
        api.getDepartments(),
        api.getEmployees().catch(() => [] as Employee[]),
      ]);
      setDepartments(deptData);
      setEmployees(empData);
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
    Promise.all([api.getDepartments(), api.getEmployees().catch(() => [] as Employee[])])
      .then(([deptData, empData]) => {
        if (!ignore) {
          setDepartments(deptData);
          setEmployees(empData);
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
      loadData();
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
      loadData();
    } catch (err: unknown) {
      toast.error("Erro ao excluir setor", {
        description: err instanceof Error ? err.message : "Não foi possível excluir o setor",
      });
    }
  };

  // Map employee count by department
  const employeesByDept = useMemo(() => {
    const map = new Map<number, Employee[]>();
    for (const emp of employees) {
      if (emp.department_id) {
        const list = map.get(emp.department_id) || [];
        list.push(emp);
        map.set(emp.department_id, list);
      }
    }
    return map;
  }, [employees]);

  // Filtered departments
  const filteredDepartments = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return departments;
    return departments.filter(
      (d) =>
        d.name.toLowerCase().includes(q) ||
        (d.description && d.description.toLowerCase().includes(q))
    );
  }, [departments, search]);

  // Metrics
  const totalEmployeesWithDept = useMemo(() => {
    return employees.filter((e) => e.department_id !== null && e.department_id !== undefined).length;
  }, [employees]);

  const targetDeptForDelete = departments.find((d) => d.id === deleteId);
  const deleteDeptEmpCount = deleteId ? employeesByDept.get(deleteId)?.length || 0 : 0;

  return (
    <PageContainer>
      <PageHeader
        title="Setores & Departamentos"
        description="Estruture a organização em departamentos para contexto semântico da IA, relatórios e governança de atendimento."
        actions={
          <Button onClick={openCreateDialog} className="gap-2 shadow-sm">
            <Plus className="size-4" />
            Novo Setor
          </Button>
        }
      />

      <StatGrid columns={3}>
        <StatCard
          icon={<Building2 />}
          label="Total de setores"
          value={loading ? "..." : departments.length}
          description="cadastrados no sistema"
        />
        <StatCard
          icon={<Users className="text-emerald-600 dark:text-emerald-400" />}
          label="Colaboradores alocados"
          value={loading ? "..." : totalEmployeesWithDept}
          description="vinculados a algum setor"
        />
        <StatCard
          icon={<Briefcase className="text-sky-600 dark:text-sky-400" />}
          label="Média por setor"
          value={loading || departments.length === 0 ? "0" : (totalEmployeesWithDept / departments.length).toFixed(1)}
          description="membros por departamento"
        />
      </StatGrid>

      {/* Search and View Controls */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3">
        <div className="relative flex-1 w-full max-w-md">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Buscar setor por nome ou atribuição..."
            className="pl-9 pr-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {search && (
            <button
              type="button"
              onClick={() => setSearch("")}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            >
              <X className="size-4" />
            </button>
          )}
        </div>

        <div className="flex items-center gap-2 self-end sm:self-auto">
          <span className="text-xs text-muted-foreground mr-1">
            {filteredDepartments.length} {filteredDepartments.length === 1 ? "setor" : "setores"}
          </span>

          <div className="flex items-center rounded-lg border bg-muted/30 p-0.5">
            <Button
              variant={viewMode === "grid" ? "secondary" : "ghost"}
              size="icon"
              className="size-8"
              onClick={() => setViewMode("grid")}
              title="Visualização em Grade"
            >
              <LayoutGrid className="size-4" />
            </Button>
            <Button
              variant={viewMode === "table" ? "secondary" : "ghost"}
              size="icon"
              className="size-8"
              onClick={() => setViewMode("table")}
              title="Visualização em Tabela"
            >
              <TableIcon className="size-4" />
            </Button>
          </div>
        </div>
      </div>

      {/* Content */}
      {loading ? (
        <Card className="p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <Loader2 className="size-8 animate-spin text-primary" />
            <p className="text-sm text-muted-foreground">Carregando setores e membros...</p>
          </div>
        </Card>
      ) : departments.length === 0 ? (
        <Card className="border-dashed p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <div className="flex size-14 items-center justify-center rounded-full bg-muted">
              <Building2 className="size-7 text-muted-foreground" />
            </div>
            <CardTitle className="text-base font-semibold">Nenhum setor cadastrado</CardTitle>
            <CardDescription className="max-w-sm text-xs">
              Cadastre departamentos como Comercial, Suporte, Financeiro ou Diretoria para organizar seus colaboradores e conversas.
            </CardDescription>
            <Button onClick={openCreateDialog} className="mt-2 gap-2">
              <Plus className="size-4" />
              Cadastrar Primeiro Setor
            </Button>
          </div>
        </Card>
      ) : filteredDepartments.length === 0 ? (
        <Card className="border-dashed p-8">
          <div className="flex flex-col items-center justify-center gap-2 text-center">
            <Search className="size-6 text-muted-foreground" />
            <p className="text-sm font-medium">Nenhum setor encontrado para &ldquo;{search}&rdquo;</p>
            <Button variant="ghost" size="sm" onClick={() => setSearch("")} className="mt-1 text-xs">
              Limpar busca
            </Button>
          </div>
        </Card>
      ) : viewMode === "grid" ? (
        /* Grid Cards View */
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filteredDepartments.map((dept) => {
            const deptEmps = employeesByDept.get(dept.id) || [];
            return (
              <Card
                key={dept.id}
                className="group relative flex flex-col justify-between overflow-hidden transition-all duration-200 hover:border-primary/50 hover:shadow-md"
              >
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0 group-hover:bg-primary group-hover:text-primary-foreground transition-colors">
                        <Building2 className="size-5" />
                      </div>
                      <div className="min-w-0">
                        <CardTitle className="text-base font-semibold truncate" title={dept.name}>
                          {dept.name}
                        </CardTitle>
                        <span className="text-xs text-muted-foreground flex items-center gap-1 mt-0.5">
                          <Users className="size-3" />
                          {deptEmps.length} {deptEmps.length === 1 ? "colaborador" : "colaboradores"}
                        </span>
                      </div>
                    </div>

                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-8 text-muted-foreground hover:text-foreground shrink-0"
                        >
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => openEditDialog(dept)} className="gap-2 cursor-pointer">
                          <Pencil className="size-4" />
                          Editar Setor
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onClick={() => setDeleteId(dept.id)}
                          className="gap-2 text-destructive focus:text-destructive cursor-pointer"
                        >
                          <Trash2 className="size-4" />
                          Excluir Setor
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>

                  <CardDescription className="mt-3 text-xs leading-relaxed line-clamp-2 min-h-[2.5rem]">
                    {dept.description || "Nenhuma descrição informada para este setor."}
                  </CardDescription>
                </CardHeader>

                <CardContent className="space-y-3 pt-0 mt-auto border-t border-border/40 pt-3">
                  {deptEmps.length > 0 ? (
                    <div className="flex items-center justify-between">
                      <div className="flex -space-x-1.5 overflow-hidden">
                        {deptEmps.slice(0, 4).map((emp) => (
                          <Avatar key={emp.id} className="size-6 border-2 border-background text-[9px] font-semibold">
                            <AvatarFallback>{emp.name.slice(0, 2).toUpperCase()}</AvatarFallback>
                          </Avatar>
                        ))}
                        {deptEmps.length > 4 && (
                          <div className="flex size-6 items-center justify-center rounded-full border-2 border-background bg-muted text-[9px] font-medium text-muted-foreground">
                            +{deptEmps.length - 4}
                          </div>
                        )}
                      </div>
                      <Badge variant="secondary" className="text-[11px] font-normal">
                        Ativo
                      </Badge>
                    </div>
                  ) : (
                    <div className="text-[11px] text-muted-foreground italic">
                      Nenhum colaborador vinculado
                    </div>
                  )}

                  <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <Calendar className="size-3" />
                      Criado em {new Date(dept.created_at).toLocaleDateString("pt-BR")}
                    </span>
                  </div>
                </CardContent>
              </Card>
            );
          })}
        </div>
      ) : (
        /* Table View */
        <Card className="overflow-hidden border-border/80 shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[300px]">Setor</TableHead>
                <TableHead>Atribuição / Descrição</TableHead>
                <TableHead className="w-[180px]">Membros</TableHead>
                <TableHead className="w-[140px]">Data Criação</TableHead>
                <TableHead className="w-[80px] text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredDepartments.map((dept) => {
                const deptEmps = employeesByDept.get(dept.id) || [];
                return (
                  <TableRow key={dept.id} className="group">
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2.5">
                        <div className="flex size-8 items-center justify-center rounded-md bg-primary/10 text-primary shrink-0">
                          <Building2 className="size-4" />
                        </div>
                        <span className="font-semibold text-foreground">{dept.name}</span>
                      </div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground max-w-md truncate">
                      {dept.description || "—"}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Badge variant={deptEmps.length > 0 ? "secondary" : "outline"} className="gap-1 text-xs">
                          <Users className="size-3" />
                          {deptEmps.length}
                        </Badge>
                        {deptEmps.length > 0 && (
                          <span className="text-xs text-muted-foreground truncate max-w-[120px]">
                            {deptEmps.map((e) => e.name).join(", ")}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {new Date(dept.created_at).toLocaleDateString("pt-BR")}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="size-8">
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEditDialog(dept)} className="gap-2 cursor-pointer">
                            <Pencil className="size-4" />
                            Editar
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={() => setDeleteId(dept.id)}
                            className="gap-2 text-destructive focus:text-destructive cursor-pointer"
                          >
                            <Trash2 className="size-4" />
                            Excluir
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Dialog Criação / Edição */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <form onSubmit={handleSave}>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Building2 className="size-5 text-primary" />
                {editingDept ? "Editar Setor" : "Novo Setor"}
              </DialogTitle>
              <DialogDescription>
                {editingDept
                  ? "Atualize as informações do departamento."
                  : "Defina o nome e a finalidade deste setor na governança de conversas."}
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-4 py-4">
              <div className="grid gap-1.5">
                <Label htmlFor="dept-name">Nome do Setor *</Label>
                <Input
                  id="dept-name"
                  placeholder="Ex: Comercial, Suporte, Financeiro, Diretoria"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="dept-desc">Descrição / Atribuição</Label>
                <Input
                  id="dept-desc"
                  placeholder="Ex: Responsável por prospecção, atendimento e fechamentos"
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                />
                <p className="text-[11px] text-muted-foreground">
                  Essa informação é repassada como contexto semântico para a IA compreender quem atende quem.
                </p>
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
            <AlertDialogTitle className="flex items-center gap-2 text-destructive">
              <Trash2 className="size-5" />
              Excluir Setor
            </AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <p>
                Tem certeza que deseja excluir o setor <strong>&ldquo;{targetDeptForDelete?.name}&rdquo;</strong>?
              </p>
              {deleteDeptEmpCount > 0 && (
                <p className="rounded-md border border-warning/50 bg-warning/10 p-2.5 text-xs text-warning font-medium">
                  Atenção: {deleteDeptEmpCount} {deleteDeptEmpCount === 1 ? "colaborador está vinculado" : "colaboradores estão vinculados"} a este setor e ficarão sem setor definido.
                </p>
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Excluir Setor
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </PageContainer>
  );
}
