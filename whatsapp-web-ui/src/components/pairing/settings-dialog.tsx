"use client";

import Link from "next/link";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { useSettings } from "@/lib/store";

interface SettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsDialog({ open, onOpenChange }: SettingsDialogProps) {
  const { darkMode, setDarkMode } = useSettings();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Configurações</DialogTitle>
          <DialogDescription>Ajustes rápidos do painel</DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Aparência</h4>
            <div className="flex items-center justify-between">
              <div>
                <Label htmlFor="dark-mode">Modo escuro</Label>
                <p className="text-xs text-muted-foreground">Usar o tema escuro</p>
              </div>
              <Button id="dark-mode" variant={darkMode ? "default" : "outline"} size="sm" onClick={() => setDarkMode(!darkMode)}>
                {darkMode ? "Ativado" : "Desativado"}
              </Button>
            </div>
          </div>

          <Separator />

          <div className="space-y-2">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">Conta</h4>
            <p className="text-sm text-muted-foreground">
              Gerencie os acessos e saia da conta pela página de configurações.
            </p>
            <Button variant="outline" asChild onClick={() => onOpenChange(false)}>
              <Link href="/settings">Abrir configurações</Link>
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
