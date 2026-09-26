"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Loader2, LogIn, MessageSquare } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { APIError, WhatsAppAPI } from "@/lib/api";
import { useAuth } from "@/lib/store";

function loginErrorMessage(error: unknown): string {
  if (error instanceof APIError) {
    switch (error.status) {
      case 401:
        return "Usuário ou senha inválidos.";
      case 429:
        return "Muitas tentativas sem sucesso. Aguarde alguns minutos e tente novamente.";
      case 501:
        return "O acesso ainda não foi configurado. Fale com o administrador.";
      default:
        return error.message;
    }
  }
  return "Não foi possível conectar ao serviço. Tente novamente.";
}

export default function LoginPage() {
  const router = useRouter();
  const setAuthed = useAuth((s) => s.setAuthed);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  const submitLogin = async () => {
    if (loading) return;

    const trimmedUsername = username.trim();
    if (!trimmedUsername) {
      setError("Informe seu usuário.");
      usernameRef.current?.focus();
      return;
    }
    if (!password) {
      setError("Informe sua senha.");
      passwordRef.current?.focus();
      return;
    }

    setLoading(true);
    setError("");
    try {
      const user = await new WhatsAppAPI().login(trimmedUsername, password);
      setPassword("");
      setAuthed(user.username);
      router.replace("/");
    } catch (err) {
      setError(loginErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    submitLogin();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      if (!username.trim()) {
        usernameRef.current?.focus();
        return;
      }
      if (!password) {
        passwordRef.current?.focus();
        return;
      }
      submitLogin();
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/30 p-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <MessageSquare className="mx-auto mb-2 h-10 w-10 text-success" />
          <CardTitle>WhatsApp MCP</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} onKeyDown={handleKeyDown} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="username">Usuário</Label>
              <Input
                ref={usernameRef}
                id="username"
                autoComplete="username"
                autoFocus
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Senha</Label>
              <Input
                ref={passwordRef}
                id="password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full" disabled={loading}>
              {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <LogIn className="mr-2 h-4 w-4" />}
              Entrar
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
