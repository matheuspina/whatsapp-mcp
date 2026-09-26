"use client";

import { LogOut } from "lucide-react";
import { toast } from "sonner";
import { ActiveSessions } from "@/components/settings/active-sessions";
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
    <div className="p-4 md:p-6">
      <div className="max-w-3xl space-y-6">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
          <p className="text-sm text-muted-foreground">Account, sessions and appearance</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Account</CardTitle>
            <CardDescription>
              Sign-in credentials are set on the server (WEB_UI_USERNAME / WEB_UI_PASSWORD in .env).
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between">
            <div className="text-sm">
              Signed in as <span className="font-medium">{username}</span>
            </div>
            <Button variant="outline" onClick={handleLogout}>
              <LogOut className="mr-2 h-4 w-4" />
              Sign out
            </Button>
          </CardContent>
        </Card>

        <ActiveSessions />

        <Card>
          <CardHeader>
            <CardTitle>Appearance</CardTitle>
            <CardDescription>Customize the look and feel</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <Label htmlFor="darkMode">Dark Mode</Label>
                <p className="text-sm text-muted-foreground">Enable dark theme for the interface</p>
              </div>
              <Switch id="darkMode" checked={darkMode} onCheckedChange={setDarkMode} />
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
