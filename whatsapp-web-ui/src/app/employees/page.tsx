"use client";

import { useEffect, useState, useMemo, useCallback } from "react";
import {
  Users,
  Plus,
  Pencil,
  Trash2,
  Loader2,
  Search,
  Building2,
  Mail,
  Smartphone,
  CheckCircle2,
  XCircle,
  MoreHorizontal,
  LayoutGrid,
  Table as TableIcon,
  X,
  Filter,
  UserCheck,
  UserX,
  RotateCcw,
} from "lucide-react";
import { toast } from "sonner";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { StatCard, StatGrid } from "@/components/common/stat-card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { WhatsAppAPI, Employee, Department, Instance } from "@/lib/api";

export default function EmployeesPage() {
  const [employees, setEmployees] = useState<Employee[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [instances, setInstances] = useState<Instance[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedDeptFilter, setSelectedDeptFilter] = useState<string>("all");
  const [selectedStatusFilter, setSelectedStatusFilter] = useState<"all" | "active" | "inactive">("all");
  const [viewMode, setViewMode] = useState<"table" | "grid">("table");

  // Dialogs
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
      const [empData, deptData, instData] = await Promise.all([
        api.getEmployees(),
        api.getDepartments(),
        api.getInstances().catch(() => [] as Instance[]),
      ]);
      setEmployees(empData);
      setDepartments(deptData);
      setInstances(instData);
    } catch (err: unknown) {
      toast.error("Erro ao carregar colaboradores", {
        description: err instanceof Error ? err.message : "Falha na comunicação com o serviço",
      });
    } finally {
      setLoading(false);
    }
  }, [api]);

  useEffect(() => {
    let ignore = false;
    Promise.all([
      api.getEmployees(),
      api.getDepartments(),
      api.getInstances().catch(() => [] as Instance[]),
    ])
      .then(([empData, deptData, instData]) => {
        if (!ignore) {
          setEmployees(empData);
          setDepartments(deptData);
          setInstances(instData);
          setLoading(false);
        }
      })
      .catch((err: unknown) => {
        if (!ignore) {
          toast.error("Erro ao carregar colaboradores", {
            description: err instanceof Error ? err.message : "Falha na comunicação com o serviço",
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

  const handleToggleStatus = async (emp: Employee) => {
    try {
      await api.updateEmployee(emp.id, {
        name: emp.name,
        role: emp.role || "",
        department_id: emp.department_id,
        email: emp.email || "",
        active: !emp.active,
      });
      toast.success(
        emp.active
          ? `Colaborador "${emp.name}" desativado.`
          : `Colaborador "${emp.name}" reativado.`
      );
      loadData();
    } catch (err: unknown) {
      toast.error("Erro ao alterar status do colaborador", {
        description: err instanceof Error ? err.message : "Falha na comunicação",
      });
    }
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

  // Map instances assigned to employees
  const instancesByEmp = useMemo(() => {
    const map = new Map<number, Instance[]>();
    for (const inst of instances) {
      if (inst.employee_id) {
        const list = map.get(inst.employee_id) || [];
        list.push(inst);
        map.set(inst.employee_id, list);
      }
    }
    return map;
  }, [instances]);

  // Filtered employees
  const filteredEmployees = useMemo(() => {
    return employees.filter((emp) => {
      const q = searchQuery.toLowerCase();
      const matchesSearch =
        emp.name.toLowerCase().includes(q) ||
        (emp.role && emp.role.toLowerCase().includes(q)) ||
        (emp.email && emp.email.toLowerCase().includes(q));

      const matchesDept =
        selectedDeptFilter === "all" ||
        (selectedDeptFilter === "none" && !emp.department_id) ||
        (emp.department_id && emp.department_id.toString() === selectedDeptFilter);

      const matchesStatus =
        selectedStatusFilter === "all" ||
        (selectedStatusFilter === "active" && emp.active) ||
        (selectedStatusFilter === "inactive" && !emp.active);

      return matchesSearch && matchesDept && matchesStatus;
    });
  }, [employees, searchQuery, selectedDeptFilter, selectedStatusFilter]);

  const hasActiveFilters =
    searchQuery.trim() !== "" ||
    selectedDeptFilter !== "all" ||
    selectedStatusFilter !== "all";

  const resetFilters = () => {
    setSearchQuery("");
    setSelectedDeptFilter("all");
    setSelectedStatusFilter("all");
  };

  // Stats
  const activeCount = employees.filter((e) => e.active).length;
  const inactiveCount = employees.filter((e) => !e.active).length;
  const withDeptCount = employees.filter((e) => e.department_id !== null).length;

  const targetEmpForDelete = employees.find((e) => e.id === deleteId);

  return (
    <PageContainer>
      <PageHeader
        title="Colaboradores"
        description="Cadastre as pessoas responsáveis pelos números e pelo atendimento aos clientes."
        actions={
          <Button onClick={openCreateDialog} className="gap-2 shadow-sm">
            <Plus className="size-4" />
            Novo Colaborador
          </Button>
        }
      />

      <StatGrid columns={4}>
        <StatCard
          icon={<Users />}
          label="Total"
          value={loading ? "..." : employees.length}
          description="colaboradores cadastrados"
        />
        <StatCard
          icon={<UserCheck className="text-emerald-600 dark:text-emerald-400" />}
          label="Ativos"
          value={loading ? "..." : activeCount}
          description="com acesso e atribuição"
          valueClassName="text-emerald-600 dark:text-emerald-400"
        />
        <StatCard
          icon={<UserX />}
          label="Inativos"
          value={loading ? "..." : inactiveCount}
          description="afastados ou desligados"
          valueClassName="text-muted-foreground"
        />
        <StatCard
          icon={<Building2 />}
          label="Com setor"
          value={loading ? "..." : withDeptCount}
          description="vinculados a departamento"
        />
      </StatGrid>

      {/* Filter and Control Bar */}
      <Card className="border-border/80 shadow-sm p-4">
        <div className="flex flex-col gap-3">
          <div className="flex flex-col md:flex-row items-center gap-3">
            {/* Search Input */}
            <div className="relative flex-1 w-full">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Buscar por nome, cargo ou e-mail..."
                className="pl-9 pr-9"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
              />
              {searchQuery && (
                <button
                  type="button"
                  onClick={() => setSearchQuery("")}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="size-4" />
                </button>
              )}
            </div>

            {/* Department Select Filter */}
            <div className="w-full md:w-56">
              <Select value={selectedDeptFilter} onValueChange={setSelectedDeptFilter}>
                <SelectTrigger>
                  <SelectValue placeholder="Filtrar por setor" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Todos os setores</SelectItem>
                  <SelectItem value="none">Sem setor atribuído</SelectItem>
                  {departments.map((dept) => (
                    <SelectItem key={dept.id} value={dept.id.toString()}>
                      {dept.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Status Select Filter */}
            <div className="w-full md:w-44">
              <Select
                value={selectedStatusFilter}
                onValueChange={(v) => setSelectedStatusFilter(v as "all" | "active" | "inactive")}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Todos os status</SelectItem>
                  <SelectItem value="active">Apenas Ativos</SelectItem>
                  <SelectItem value="inactive">Apenas Inativos</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* View Mode Toggle */}
            <div className="flex items-center gap-2 self-end md:self-auto shrink-0">
              {hasActiveFilters && (
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={resetFilters}
                  className="h-9 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
                >
                  <RotateCcw className="size-3.5" />
                  Limpar
                </Button>
              )}

              <div className="flex items-center rounded-lg border bg-muted/30 p-0.5">
                <Button
                  variant={viewMode === "table" ? "secondary" : "ghost"}
                  size="icon"
                  className="size-8"
                  onClick={() => setViewMode("table")}
                  title="Visualização em Tabela"
                >
                  <TableIcon className="size-4" />
                </Button>
                <Button
                  variant={viewMode === "grid" ? "secondary" : "ghost"}
                  size="icon"
                  className="size-8"
                  onClick={() => setViewMode("grid")}
                  title="Visualização em Cartões"
                >
                  <LayoutGrid className="size-4" />
                </Button>
              </div>
            </div>
          </div>

          {/* Active Filter Chips */}
          {hasActiveFilters && (
            <div className="flex flex-wrap items-center gap-1.5 border-t pt-2.5">
              <span className="text-xs text-muted-foreground font-medium mr-1 flex items-center gap-1">
                <Filter className="size-3" />
                Filtros:
              </span>

              {searchQuery && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Busca: &ldquo;{searchQuery}&rdquo;
                  <button onClick={() => setSearchQuery("")} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {selectedDeptFilter !== "all" && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Setor:{" "}
                  {selectedDeptFilter === "none"
                    ? "Sem setor"
                    : departments.find((d) => d.id.toString() === selectedDeptFilter)?.name}
                  <button onClick={() => setSelectedDeptFilter("all")} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}

              {selectedStatusFilter !== "all" && (
                <Badge variant="secondary" className="gap-1 text-xs">
                  Status: {selectedStatusFilter === "active" ? "Ativos" : "Inativos"}
                  <button onClick={() => setSelectedStatusFilter("all")} className="hover:text-destructive">
                    <X className="size-3" />
                  </button>
                </Badge>
              )}
            </div>
          )}
        </div>
      </Card>

      {/* Content */}
      {loading ? (
        <Card className="p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <Loader2 className="size-8 animate-spin text-primary" />
            <p className="text-sm text-muted-foreground">Carregando lista de colaboradores...</p>
          </div>
        </Card>
      ) : employees.length === 0 ? (
        <Card className="border-dashed p-12">
          <div className="flex flex-col items-center justify-center gap-3 text-center">
            <div className="flex size-14 items-center justify-center rounded-full bg-muted">
              <Users className="size-7 text-muted-foreground" />
            </div>
            <CardTitle className="text-base font-semibold">Nenhum colaborador cadastrado</CardTitle>
            <CardDescription className="max-w-sm text-xs">
              Cadastre os membros da sua equipe para vincular aparelhos celulares e manter o histórico de atendimento auditável.
            </CardDescription>
            <Button onClick={openCreateDialog} className="mt-2 gap-2">
              <Plus className="size-4" />
              Cadastrar Primeiro Colaborador
            </Button>
          </div>
        </Card>
      ) : filteredEmployees.length === 0 ? (
        <Card className="border-dashed p-8">
          <div className="flex flex-col items-center justify-center gap-2 text-center">
            <Search className="size-6 text-muted-foreground" />
            <p className="text-sm font-medium">Nenhum colaborador encontrado para os filtros selecionados.</p>
            <Button variant="ghost" size="sm" onClick={resetFilters} className="mt-1 text-xs">
              Limpar filtros
            </Button>
          </div>
        </Card>
      ) : viewMode === "table" ? (
        /* Table View */
        <Card className="overflow-hidden border-border/80 shadow-sm">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[280px]">Colaborador</TableHead>
                <TableHead className="w-[180px]">Setor</TableHead>
                <TableHead className="w-[160px]">Aparelho(s)</TableHead>
                <TableHead>E-mail</TableHead>
                <TableHead className="w-[120px]">Status</TableHead>
                <TableHead className="w-[80px] text-right">Ações</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredEmployees.map((emp) => {
                const empInstances = instancesByEmp.get(emp.id) || [];
                return (
                  <TableRow key={emp.id} className="group">
                    <TableCell>
                      <div className="flex items-center gap-3">
                        <Avatar className="size-8 font-semibold text-xs">
                          <AvatarFallback>
                            {emp.name.slice(0, 2).toUpperCase()}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <p className="font-semibold text-foreground truncate max-w-[200px]">
                            {emp.name}
                          </p>
                          <p className="text-xs text-muted-foreground truncate">
                            {emp.role || "Sem cargo definido"}
                          </p>
                        </div>
                      </div>
                    </TableCell>

                    <TableCell>
                      {emp.department_name ? (
                        <Badge variant="secondary" className="gap-1 font-normal text-xs">
                          <Building2 className="size-3 text-muted-foreground" />
                          {emp.department_name}
                        </Badge>
                      ) : (
                        <span className="text-xs text-muted-foreground italic">Sem setor</span>
                      )}
                    </TableCell>

                    <TableCell>
                      {empInstances.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {empInstances.map((inst) => (
                            <Badge key={inst.id} variant="outline" className="gap-1 text-[11px] font-normal py-0.5">
                              <Smartphone className="size-3 text-muted-foreground" />
                              {inst.alias || inst.phone_number}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground italic">Nenhum</span>
                      )}
                    </TableCell>

                    <TableCell className="text-xs text-muted-foreground">
                      {emp.email ? (
                        <span className="flex items-center gap-1.5 truncate max-w-[220px]">
                          <Mail className="size-3.5 text-muted-foreground shrink-0" />
                          {emp.email}
                        </span>
                      ) : (
                        "—"
                      )}
                    </TableCell>

                    <TableCell>
                      {emp.active ? (
                        <Badge variant="success" className="gap-1 text-[11px]">
                          <span className="size-1.5 rounded-full bg-emerald-500 animate-pulse" />
                          Ativo
                        </Badge>
                      ) : (
                        <Badge variant="outline" className="text-[11px] text-muted-foreground">
                          Inativo
                        </Badge>
                      )}
                    </TableCell>

                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="size-8">
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => openEditDialog(emp)} className="gap-2 cursor-pointer">
                            <Pencil className="size-4" />
                            Editar
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => handleToggleStatus(emp)}
                            className="gap-2 cursor-pointer"
                          >
                            {emp.active ? (
                              <>
                                <XCircle className="size-4 text-warning" />
                                Desativar
                              </>
                            ) : (
                              <>
                                <CheckCircle2 className="size-4 text-emerald-500" />
                                Reativar
                              </>
                            )}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onClick={() => setDeleteId(emp.id)}
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
      ) : (
        /* Grid Cards View */
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filteredEmployees.map((emp) => {
            const empInstances = instancesByEmp.get(emp.id) || [];
            return (
              <Card
                key={emp.id}
                className="group relative flex flex-col justify-between overflow-hidden transition-all duration-200 hover:border-primary/50 hover:shadow-md"
              >
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="relative">
                        <Avatar className="size-11 font-semibold text-sm">
                          <AvatarFallback>{emp.name.slice(0, 2).toUpperCase()}</AvatarFallback>
                        </Avatar>
                        <span
                          className={`absolute -bottom-0.5 -right-0.5 size-3 rounded-full border-2 border-background ${
                            emp.active ? "bg-emerald-500" : "bg-muted-foreground"
                          }`}
                        />
                      </div>
                      <div className="min-w-0">
                        <CardTitle className="text-base font-semibold truncate" title={emp.name}>
                          {emp.name}
                        </CardTitle>
                        <span className="text-xs text-muted-foreground block truncate">
                          {emp.role || "Sem cargo"}
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
                        <DropdownMenuItem onClick={() => openEditDialog(emp)} className="gap-2 cursor-pointer">
                          <Pencil className="size-4" />
                          Editar
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          onClick={() => handleToggleStatus(emp)}
                          className="gap-2 cursor-pointer"
                        >
                          {emp.active ? (
                            <>
                              <XCircle className="size-4 text-warning" />
                              Desativar
                            </>
                          ) : (
                            <>
                              <CheckCircle2 className="size-4 text-emerald-500" />
                              Reativar
                            </>
                          )}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onClick={() => setDeleteId(emp.id)}
                          className="gap-2 text-destructive focus:text-destructive cursor-pointer"
                        >
                          <Trash2 className="size-4" />
                          Excluir
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </CardHeader>

                <CardContent className="space-y-3 pt-0 border-t border-border/40 pt-3">
                  <div className="flex flex-wrap items-center gap-1.5">
                    {emp.department_name ? (
                      <Badge variant="secondary" className="gap-1 text-xs font-normal">
                        <Building2 className="size-3 text-muted-foreground" />
                        {emp.department_name}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs font-normal text-muted-foreground">
                        Sem setor
                      </Badge>
                    )}

                    {emp.active ? (
                      <Badge variant="success" className="text-xs">
                        Ativo
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-xs text-muted-foreground">
                        Inativo
                      </Badge>
                    )}
                  </div>

                  {emp.email && (
                    <div className="flex items-center gap-2 text-xs text-muted-foreground truncate">
                      <Mail className="size-3.5 shrink-0" />
                      <span className="truncate">{emp.email}</span>
                    </div>
                  )}

                  {empInstances.length > 0 && (
                    <div className="flex items-center gap-1.5 text-xs text-muted-foreground pt-1">
                      <Smartphone className="size-3.5 shrink-0 text-primary" />
                      <span className="font-medium text-foreground">
                        {empInstances.map((i) => i.alias || i.phone_number).join(", ")}
                      </span>
                    </div>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {/* Dialog Criação / Edição */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <form onSubmit={handleSave}>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <Users className="size-5 text-primary" />
                {editingEmp ? "Editar Colaborador" : "Novo Colaborador"}
              </DialogTitle>
              <DialogDescription>
                {editingEmp
                  ? "Atualize o cadastro do colaborador e suas atribuições."
                  : "Cadastre um novo colaborador para vinculá-lo a aparelhos e setores."}
              </DialogDescription>
            </DialogHeader>

            <div className="grid gap-4 py-4">
              <div className="grid gap-1.5">
                <Label htmlFor="emp-name">Nome Completo *</Label>
                <Input
                  id="emp-name"
                  placeholder="Ex: Matheus Pina"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoFocus
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="emp-role">Cargo / Função</Label>
                <Input
                  id="emp-role"
                  placeholder="Ex: Gerente de Contas, Atendente, Diretor"
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="emp-dept">Departamento / Setor</Label>
                <Select value={deptId} onValueChange={setDeptId}>
                  <SelectTrigger id="emp-dept">
                    <SelectValue placeholder="Selecione um setor" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none">Nenhum / Sem setor</SelectItem>
                    {departments.map((d) => (
                      <SelectItem key={d.id} value={d.id.toString()}>
                        {d.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="emp-email">E-mail Corporativo</Label>
                <Input
                  id="emp-email"
                  type="email"
                  placeholder="Ex: matheus@empresa.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>

              {editingEmp && (
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div className="space-y-0.5">
                    <Label htmlFor="emp-active" className="text-sm font-medium">
                      Colaborador Ativo
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      Colaboradores inativos não recebem novas atribuições de aparelhos.
                    </p>
                  </div>
                  <Switch id="emp-active" checked={active} onCheckedChange={setActive} />
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
            <AlertDialogTitle className="flex items-center gap-2 text-destructive">
              <Trash2 className="size-5" />
              Excluir Colaborador
            </AlertDialogTitle>
            <AlertDialogDescription>
              Tem certeza que deseja excluir o cadastro de <strong>&ldquo;{targetEmpForDelete?.name}&rdquo;</strong>?
              O histórico de mensagens antigas atribuídas a este colaborador continuará registrado para fins de auditoria.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Excluir
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </PageContainer>
  );
}
