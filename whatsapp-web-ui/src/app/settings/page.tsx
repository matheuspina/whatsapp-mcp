"use client";

import { LogOut } from "lucide-react";
import { toast } from "sonner";
import { ActiveSessions } from "@/components/settings/active-sessions";
import { MediaStorage } from "@/components/settings/media-storage";
import { PageContainer, PageHeader } from "@/components/layout/page";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { WhatsAppAPI, getErrorMessage } from "@/lib/api";
import { useAuth, useSettings } from "@/lib/store";

export default function SettingsPage() {
  const { darkMode, setDarkMode } = useSettings();
  const { username, setAnon } = useAuth();

  const handleLogout = async () => {
    try {
      await new WhatsAppAPI().logout();
    } catch (error) {
      toast.error(getErrorMessage(error).title, { description: getErrorMessage(error).description });
    }
    setAnon(); // the gate sends us back to /login
  };

  return (
    <PageContainer size="narrow">
        <PageHeader title="Configurações" description="Conta, acessos, arquivos e aparência" />

        <Card>
          <CardHeader>
            <CardTitle>Conta</CardTitle>
            <CardDescription>Gerencie seu acesso ao painel.</CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between">
            <div className="text-sm">
              Acesso atual: <span className="font-medium">{username}</span>
            </div>
            <Button variant="outline" onClick={handleLogout}>
              <LogOut className="mr-2 h-4 w-4" />
              Sair
            </Button>
          </CardContent>
        </Card>

        <ActiveSessions />

        <MediaStorage />

        <Card>
          <CardHeader>
            <CardTitle>Aparência</CardTitle>
            <CardDescription>Escolha como o painel será exibido</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label htmlFor="darkMode">Modo escuro</Label>
                <p className="text-sm text-muted-foreground">Usar o tema escuro no painel</p>
              </div>
              <Switch id="darkMode" checked={darkMode} onCheckedChange={setDarkMode} />
            </div>
          </CardContent>
        </Card>
    </PageContainer>
  );
}
