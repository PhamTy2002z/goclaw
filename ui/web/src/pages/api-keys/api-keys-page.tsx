import { useState, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Plus, RefreshCw, Key, Ban, Copy, Check, Shield, Building2, Clock } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { SearchInput } from "@/components/shared/search-input";
import { Pagination } from "@/components/shared/pagination";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { ConfirmDeleteDialog } from "@/components/shared/confirm-delete-dialog";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { usePagination } from "@/hooks/use-pagination";
import { useApiKeys } from "./hooks/use-api-keys";
import { ApiKeyCreateDialog } from "./api-key-create-dialog";
import { useTenants } from "@/hooks/use-tenants";
import { formatRelativeTime } from "@/lib/format";
import type { ApiKeyData } from "@/types/api-key";

function fullDateTime(iso: string | null): string {
  if (!iso) return "";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric", month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
}

function keyStatus(key: ApiKeyData, t: (k: string) => string): { label: string; variant: "default" | "secondary" | "destructive" } {
  if (key.revoked) return { label: t("status.revoked"), variant: "destructive" };
  if (key.expires_at && new Date(key.expires_at) < new Date()) return { label: t("status.expired"), variant: "secondary" };
  return { label: t("status.active"), variant: "default" };
}

function ApiKeyCard({
  apiKey,
  isCrossTenant,
  tenants,
  t,
  onRevoke,
}: {
  apiKey: ApiKeyData;
  isCrossTenant: boolean;
  tenants: { id: string; name: string }[];
  t: (k: string, opts?: Record<string, string>) => string;
  onRevoke: () => void;
}) {
  const status = keyStatus(apiKey, t);
  const tenantName = apiKey.tenant_id
    ? tenants.find((tn) => tn.id === apiKey.tenant_id)?.name ?? t("tenantBadgeUnknown")
    : t("tenantBadgeSystem");

  return (
    <Card className="py-0 gap-0">
      <CardContent className="px-3 py-2">
        <div className="flex items-center justify-between gap-3">
          {/* Left: name + meta */}
          <div className="min-w-0 flex-1 space-y-0.5">
            {/* Row 1: name + prefix + status + tenant */}
            <div className="flex items-center gap-2 flex-wrap">
              <span className="font-medium text-sm truncate">{apiKey.name}</span>
              <code className="text-[10px] text-muted-foreground font-mono">{apiKey.prefix}...***</code>
              <Badge variant={status.variant} className="text-[10px] shrink-0">{status.label}</Badge>
              {isCrossTenant && (
                <Badge variant="outline" className="text-[10px] shrink-0 gap-0.5">
                  <Building2 className="h-2.5 w-2.5" />
                  {tenantName}
                </Badge>
              )}
            </div>

            {/* Row 2: scopes + dates */}
            <div className="flex items-center gap-2 flex-wrap text-xs text-muted-foreground">
              <div className="flex items-center gap-1">
                <Shield className="h-3 w-3 shrink-0" />
                {apiKey.scopes.map((s) => (
                  <Badge key={s} variant="secondary" className="text-[10px] font-mono px-1.5 py-0">
                    {s.replace("operator.", "")}
                  </Badge>
                ))}
              </div>
              <span className="text-muted-foreground/50">·</span>
              <span className="flex items-center gap-1" title={fullDateTime(apiKey.created_at)}>
                <Clock className="h-3 w-3" />
                {formatRelativeTime(apiKey.created_at)}
              </span>
              <span title={apiKey.last_used_at ? fullDateTime(apiKey.last_used_at) : undefined}>
                {t("columns.lastUsed")}: {apiKey.last_used_at ? formatRelativeTime(apiKey.last_used_at) : t("never")}
              </span>
            </div>
          </div>

          {/* Right: revoke button */}
          {!apiKey.revoked && (
            <Button
              variant="ghost"
              size="icon"
              onClick={onRevoke}
              className="text-muted-foreground hover:text-destructive shrink-0 h-7 w-7"
              title={t("revoke.confirmLabel")}
            >
              <Ban className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

export function ApiKeysPage() {
  const { t } = useTranslation("api-keys");
  const { t: tc } = useTranslation("common");
  const { apiKeys, loading, refresh, createApiKey, revokeApiKey } = useApiKeys();
  const { isCrossTenant, tenants } = useTenants();

  const spinning = useMinLoading(loading);
  const showSkeleton = useDeferredLoading(loading && apiKeys.length === 0);
  const [search, setSearch] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<ApiKeyData | null>(null);
  const [revokeLoading, setRevokeLoading] = useState(false);
  const [newKeyRaw, setNewKeyRaw] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const filtered = apiKeys.filter(
    (k) => k.name.toLowerCase().includes(search.toLowerCase()) || k.prefix.includes(search),
  );

  const { pageItems, pagination, setPage, setPageSize, resetPage } = usePagination(filtered);

  useEffect(() => { resetPage(); }, [search, resetPage]);

  const handleRevoke = async () => {
    if (!revokeTarget) return;
    setRevokeLoading(true);
    try {
      await revokeApiKey(revokeTarget.id);
      setRevokeTarget(null);
    } finally {
      setRevokeLoading(false);
    }
  };

  const handleCopy = async () => {
    if (!newKeyRaw) return;
    await navigator.clipboard.writeText(newKeyRaw);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="p-4 sm:p-6 pb-16">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <div className="flex gap-2">
            <Button size="sm" onClick={() => setCreateOpen(true)} className="gap-1">
              <Plus className="h-3.5 w-3.5" /> {t("addKey")}
            </Button>
            <Button variant="outline" size="sm" onClick={refresh} disabled={spinning} className="gap-1">
              <RefreshCw className={spinning ? "animate-spin h-3.5 w-3.5" : "h-3.5 w-3.5"} /> {tc("refresh")}
            </Button>
          </div>
        }
      />

      <div className="mt-4">
        <SearchInput value={search} onChange={setSearch} placeholder={t("searchPlaceholder")} className="max-w-sm" />
      </div>

      <div className="mt-4">
        {showSkeleton ? (
          <TableSkeleton rows={5} />
        ) : filtered.length === 0 ? (
          <EmptyState icon={Key} title={t("emptyTitle")} description={t("emptyDescription")} />
        ) : (
          <>
            <div className="space-y-2">
              {pageItems.map((key) => (
                <ApiKeyCard
                  key={key.id}
                  apiKey={key}
                  isCrossTenant={isCrossTenant}
                  tenants={tenants}
                  t={t}
                  onRevoke={() => setRevokeTarget(key)}
                />
              ))}
            </div>
            <Pagination {...pagination} onPageChange={setPage} onPageSizeChange={setPageSize} />
          </>
        )}
      </div>

      <ApiKeyCreateDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreate={async (input) => {
          const res = await createApiKey(input);
          setCreateOpen(false);
          setNewKeyRaw(res.key);
        }}
      />

      {/* Show-once key dialog */}
      <Dialog open={!!newKeyRaw} onOpenChange={(open) => !open && setNewKeyRaw(null)}>
        <DialogContent className="max-sm:inset-0 max-sm:translate-x-0 max-sm:translate-y-0 sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("created.title")}</DialogTitle>
            <DialogDescription>{t("created.description")}</DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <code className="flex-1 overflow-x-auto rounded bg-muted px-3 py-2 text-base md:text-sm font-mono break-all">
              {newKeyRaw}
            </code>
            <Button variant="outline" size="sm" onClick={handleCopy} className="gap-1 shrink-0">
              {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
              {copied ? t("created.copied") : t("created.copy")}
            </Button>
          </div>
          <DialogFooter>
            <Button onClick={() => setNewKeyRaw(null)}>{t("created.done")}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDeleteDialog
        open={!!revokeTarget}
        onOpenChange={(v) => !v && setRevokeTarget(null)}
        title={t("revoke.title")}
        description={t("revoke.description", { name: revokeTarget?.name })}
        confirmValue={revokeTarget?.name || ""}
        confirmLabel={t("revoke.confirmLabel")}
        onConfirm={handleRevoke}
        loading={revokeLoading}
      />
    </div>
  );
}
