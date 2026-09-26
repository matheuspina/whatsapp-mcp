"use client";

import { useEffect, useState } from "react";
import { CheckCircle2, CloudUpload, Loader2, RefreshCw, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { Notice } from "@/components/common/notice";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { MediaStorageInput, MediaStorageStatus, WhatsAppAPI, getErrorMessage } from "@/lib/api";

/** Form state: the stored settings plus the secret typed in this session (never read back from the server). */
type Form = MediaStorageInput & { secret_access_key: string };

const emptyForm: Form = {
  enabled: false,
  endpoint: "",
  region: "auto",
  bucket: "",
  access_key_id: "",
  secret_access_key: "",
  path_style: false,
  prefix: "",
  keep_local: false,
  public_base_url: "",
};

const formFrom = (status: MediaStorageStatus): Form => {
  const c = status.config;
  return {
    enabled: c.enabled,
    endpoint: c.endpoint,
    region: c.region || "auto",
    bucket: c.bucket,
    access_key_id: c.access_key_id,
    secret_access_key: "",
    path_style: c.path_style,
    prefix: c.prefix,
    keep_local: c.keep_local,
    public_base_url: c.public_base_url,
  };
};

/** Messages of the API arrive as `Error <status>` plus text; the text is what the operator needs. */
const describe = (error: unknown) => {
  const { title, description } = getErrorMessage(error);
  return description || title;
};

/**
 * Where downloaded media is stored. Files are saved locally first and uploaded in the background;
 * this card configures the bucket they go to (Cloudflare R2, Amazon S3, MinIO or any S3-compatible
 * service) and shows the upload queue.
 */
export function MediaStorage() {
  const [status, setStatus] = useState<MediaStorageStatus | null>(null);
  const [form, setForm] = useState<Form>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<"save" | "test" | "retry" | null>(null);
  const [result, setResult] = useState<{ ok: boolean; text: string } | null>(null);
  const [reloadTick, setReloadTick] = useState(0);
  // The form is filled from the server once; later refreshes only update the queue counters, so
  // they never overwrite what someone is typing.
  const [filled, setFilled] = useState(false);

  useEffect(() => {
    let cancelled = false;
    new WhatsAppAPI().getMediaStorage().then(
      (s) => {
        if (cancelled) return;
        setStatus(s);
        if (!filled) {
          setForm(formFrom(s));
          setFilled(true);
        }
        setLoading(false);
      },
      (error) => {
        if (cancelled) return;
        toast.error(getErrorMessage(error).title, { description: getErrorMessage(error).description });
        setLoading(false);
      }
    );
    return () => {
      cancelled = true;
    };
  }, [reloadTick, filled]);

  const queue = status?.queue ?? {};
  const inFlight = (queue.pending_upload ?? 0) + (queue.uploading ?? 0);

  // While files are waiting, keep the counters moving.
  useEffect(() => {
    if (!inFlight) return;
    const id = setInterval(() => setReloadTick((n) => n + 1), 10_000);
    return () => clearInterval(id);
  }, [inFlight]);

  const refresh = () => {
    setLoading(true);
    setReloadTick((n) => n + 1);
  };

  const managedByEnv = status?.source === "env";
  const locked = managedByEnv || busy !== null;
  const set = <K extends keyof Form>(key: K, value: Form[K]) => {
    setResult(null);
    setForm((f) => ({ ...f, [key]: value }));
  };

  const payload = (): MediaStorageInput => ({ ...form, secret_access_key: form.secret_access_key || undefined });

  const save = async () => {
    setBusy("save");
    setResult(null);
    try {
      const next = await new WhatsAppAPI().saveMediaStorage(payload());
      setStatus(next);
      setForm(formFrom(next)); // clears the typed secret; the server keeps it
      toast.success(next.active ? "Media storage is on" : "Settings saved");
    } catch (error) {
      setResult({ ok: false, text: describe(error) });
    } finally {
      setBusy(null);
    }
  };

  const test = async () => {
    setBusy("test");
    setResult(null);
    try {
      setResult({ ok: true, text: await new WhatsAppAPI().testMediaStorage(payload()) });
    } catch (error) {
      setResult({ ok: false, text: describe(error) });
    } finally {
      setBusy(null);
    }
  };

  const retry = async () => {
    setBusy("retry");
    try {
      const n = await new WhatsAppAPI().retryFailedMedia();
      toast.success(n > 0 ? `${n} files back in the queue` : "Nothing to retry");
      refresh();
    } catch (error) {
      toast.error(getErrorMessage(error).title, { description: getErrorMessage(error).description });
    } finally {
      setBusy(null);
    }
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0 space-y-1.5">
            <CardTitle className="flex flex-wrap items-center gap-2">
              Media storage
              {status && (
                <Badge variant={status.active ? "default" : "secondary"}>{status.active ? "Uploading" : "Off"}</Badge>
              )}
              {managedByEnv && <Badge variant="outline">Set by environment</Badge>}
            </CardTitle>
            <CardDescription>
              Photos, videos, audio and documents are saved on the server first, then uploaded in the background to
              your bucket (Cloudflare R2, Amazon S3, MinIO or any S3-compatible service), organized by department,
              employee, number and conversation.
            </CardDescription>
          </div>
          <Button variant="outline" size="sm" className="shrink-0" onClick={refresh} disabled={loading}>
            <RefreshCw className={"mr-2 h-4 w-4 " + (loading ? "animate-spin" : "")} />
            Refresh
          </Button>
        </div>
      </CardHeader>

      <CardContent className="space-y-5">
        {status === null && loading && <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />}

        {managedByEnv && (
          <Notice variant="warning" icon={TriangleAlert} title="Configured by environment variables">
            <p className="text-sm text-muted-foreground">
              The S3_* variables in the server environment (.env) take priority, so this form is read-only. Remove
              S3_BUCKET from the environment to edit the settings here.
            </p>
          </Notice>
        )}
        {status?.warning && (
          <Notice variant="warning" icon={TriangleAlert} title="Saved credentials cannot be used">
            <p className="text-sm text-muted-foreground">{status.warning}. Enter the secret access key again and save.</p>
          </Notice>
        )}
        {status?.last_error && (
          <Notice variant="destructive" icon={TriangleAlert} title="Last upload error">
            <p className="break-words text-sm text-muted-foreground">{status.last_error}</p>
          </Notice>
        )}

        {status && (
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-sm" aria-label="Upload queue">
            <Counter label="Uploaded" value={queue.uploaded} />
            <Counter label="Waiting" value={queue.pending_upload} />
            <Counter label="Uploading" value={queue.uploading} />
            <Counter label="Local only" value={queue.local} />
            <Counter label="Failed" value={queue.failed} tone={queue.failed ? "destructive" : undefined} />
            {(queue.failed ?? 0) > 0 && status.active && (
              <Button variant="link" size="sm" className="h-auto p-0" onClick={retry} disabled={busy !== null}>
                Retry failed
              </Button>
            )}
          </div>
        )}

        <div className="flex items-center justify-between gap-4">
          <div className="space-y-0.5">
            <Label htmlFor="ms-enabled">Upload media to object storage</Label>
            <p className="text-sm text-muted-foreground">
              Off, media stays in the server&apos;s local store folder. Turning it on also uploads what is already there.
            </p>
          </div>
          <Switch
            id="ms-enabled"
            checked={form.enabled}
            onCheckedChange={(v) => set("enabled", v)}
            disabled={locked}
          />
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="ms-endpoint" label="Endpoint" hint="Cloudflare R2: https://<account id>.r2.cloudflarestorage.com. Leave empty for Amazon S3.">
            <Input
              id="ms-endpoint"
              value={form.endpoint}
              onChange={(e) => set("endpoint", e.target.value)}
              placeholder="https://<account id>.r2.cloudflarestorage.com"
              disabled={locked}
              autoComplete="off"
            />
          </Field>
          <Field id="ms-region" label="Region" hint='"auto" for Cloudflare R2, e.g. "us-east-1" for Amazon S3.'>
            <Input
              id="ms-region"
              value={form.region}
              onChange={(e) => set("region", e.target.value)}
              disabled={locked}
              autoComplete="off"
            />
          </Field>
          <Field id="ms-bucket" label="Bucket">
            <Input
              id="ms-bucket"
              value={form.bucket}
              onChange={(e) => set("bucket", e.target.value)}
              placeholder="whatsapp-media"
              disabled={locked}
              autoComplete="off"
            />
          </Field>
          <Field id="ms-prefix" label="Folder inside the bucket" hint="Optional. Everything is stored under it.">
            <Input
              id="ms-prefix"
              value={form.prefix}
              onChange={(e) => set("prefix", e.target.value)}
              placeholder="whatsapp/"
              disabled={locked}
              autoComplete="off"
            />
          </Field>
          <Field id="ms-access-key" label="Access key ID">
            <Input
              id="ms-access-key"
              value={form.access_key_id}
              onChange={(e) => set("access_key_id", e.target.value)}
              disabled={locked}
              autoComplete="off"
            />
          </Field>
          <Field
            id="ms-secret"
            label="Secret access key"
            hint={status?.config.secret_set ? "Saved on the server. Leave blank to keep it." : "Stored encrypted on the server."}
          >
            <Input
              id="ms-secret"
              type="password"
              value={form.secret_access_key}
              onChange={(e) => set("secret_access_key", e.target.value)}
              placeholder={status?.config.secret_set ? "••••••••••••••••" : ""}
              disabled={locked}
              autoComplete="new-password"
            />
          </Field>
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="ms-keep-local">Keep a local copy after upload</Label>
              <p className="text-sm text-muted-foreground">
                Off frees disk space: once a file is in the bucket the local copy is deleted, and the bridge reads it
                back from the bucket when needed.
              </p>
            </div>
            <Switch
              id="ms-keep-local"
              checked={form.keep_local}
              onCheckedChange={(v) => set("keep_local", v)}
              disabled={locked}
            />
          </div>
          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="ms-path-style">Path-style addressing</Label>
              <p className="text-sm text-muted-foreground">Needed by MinIO and some self-hosted services.</p>
            </div>
            <Switch
              id="ms-path-style"
              checked={form.path_style}
              onCheckedChange={(v) => set("path_style", v)}
              disabled={locked}
            />
          </div>
          <Field
            id="ms-public-url"
            label="Public base URL"
            hint="Only if the bucket is public or has a custom domain. Leave empty for a private bucket: links are then signed and expire."
          >
            <Input
              id="ms-public-url"
              value={form.public_base_url}
              onChange={(e) => set("public_base_url", e.target.value)}
              placeholder="https://media.example.com"
              disabled={locked}
              autoComplete="off"
            />
          </Field>
        </div>

        {result && (
          <Notice variant={result.ok ? "success" : "destructive"} icon={result.ok ? CheckCircle2 : TriangleAlert}>
            <p className="break-words text-sm">{result.text}</p>
          </Notice>
        )}

        {!managedByEnv && (
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="outline" onClick={test} disabled={locked}>
              {busy === "test" ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <CloudUpload className="mr-2 h-4 w-4" />}
              Test connection
            </Button>
            <Button onClick={save} disabled={locked}>
              {busy === "save" && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function Counter({ label, value, tone }: { label: string; value?: number; tone?: "destructive" }) {
  return (
    <span className="text-muted-foreground">
      {label}{" "}
      <span className={"font-medium tabular-nums " + (tone === "destructive" ? "text-destructive" : "text-foreground")}>
        {value ?? 0}
      </span>
    </span>
  );
}
