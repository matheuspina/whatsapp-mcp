import { Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { MessageVersion } from "@/lib/api";
import { whenLabel } from "@/components/audit/feed-message";

interface VersionsDialogProps {
  open: boolean;
  /** null while loading */
  versions: MessageVersion[] | null;
  onClose: () => void;
}

/** The texts a message had before it was edited or revoked. */
export function VersionsDialog({ open, versions, onClose }: VersionsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Versões anteriores</DialogTitle>
          <DialogDescription>O texto que a mensagem tinha antes de ser editada ou apagada.</DialogDescription>
        </DialogHeader>
        {versions === null ? (
          <div className="flex h-24 items-center justify-center">
            <Loader2 className="size-6 animate-spin text-muted-foreground" />
          </div>
        ) : versions.length > 0 ? (
          <div className="space-y-3">
            {versions.map((v) => (
              <div key={v.id} className="rounded-md border p-3">
                <div className="mb-1 flex items-center gap-2 text-xs text-muted-foreground">
                  <Badge variant={v.reason === "delete" ? "destructive" : "warning"}>
                    {v.reason === "delete" ? "Texto apagado" : "Antes da edição"}
                  </Badge>
                  {whenLabel(v.recorded_at)}
                </div>
                <p className="whitespace-pre-wrap break-words text-sm">{v.content}</p>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">Nenhuma versão anterior foi registrada.</p>
        )}
      </DialogContent>
    </Dialog>
  );
}
