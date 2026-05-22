import {
  type ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import {
  AlertTriangle,
  Ban,
  Clock,
  ShieldCheck,
  Trash2,
  Upload,
  X,
} from "lucide-react";
import PageHeader from "../components/PageHeader";
import StateShell from "../components/StateShell";
import ToastNotice from "../components/ToastNotice";
import { useToast } from "../hooks/useToast";
import { api } from "../api";
import type { IPBan, SystemSettings } from "../types";
import { getErrorMessage } from "../utils/error";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  MAINTENANCE_ROUTE_GROUPS,
  extractBase64FromDataURL,
  getMaintenanceGroupConfig,
  getMaintenanceImagePreview,
  parseMaintenanceRoutesJSON,
  stringifyMaintenanceRoutes,
  updateMaintenanceGroup,
} from "../lib/maintenanceRoutes";
import { cn } from "@/lib/utils";

type SecurityForm = Pick<
  SystemSettings,
  | "global_rpm"
  | "ip_qps_limit"
  | "ip_rpm_limit"
  | "ip_auto_ban_enabled"
  | "ip_auto_ban_duration_minutes"
  | "ip_auto_ban_on_qps"
  | "ip_auto_ban_on_rpm"
  | "filter_local_fallback_response"
  | "api_key_disabled_message"
  | "api_maintenance_enabled"
  | "api_maintenance_message"
  | "api_maintenance_sse_randomize"
  | "api_maintenance_image_b64_json"
  | "api_maintenance_routes_json"
>;

const defaultForm: SecurityForm = {
  global_rpm: 0,
  ip_qps_limit: 0,
  ip_rpm_limit: 0,
  ip_auto_ban_enabled: false,
  ip_auto_ban_duration_minutes: 30,
  ip_auto_ban_on_qps: true,
  ip_auto_ban_on_rpm: true,
  filter_local_fallback_response: true,
  api_key_disabled_message: "API Key 已被禁用，请联系管理员。",
  api_maintenance_enabled: false,
  api_maintenance_message: "系统维护中，请稍后重试。",
  api_maintenance_sse_randomize: false,
  api_maintenance_image_b64_json: "",
  api_maintenance_routes_json: "{}",
};

function SecurityCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="py-0">
      <CardContent className="p-5">
        <div className="mb-4">
          <h3 className="text-base font-semibold leading-tight text-foreground">
            {title}
          </h3>
          {description ? (
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
        {children}
      </CardContent>
    </Card>
  );
}

function Field({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-2">
      <label className="block text-sm font-semibold leading-none text-foreground">
        {label}
      </label>
      {children}
      {description ? (
        <p className="text-xs leading-relaxed text-muted-foreground">
          {description}
        </p>
      ) : null}
    </div>
  );
}

function ToggleSwitch({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={cn(
        "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
        checked
          ? "border-slate-950 bg-slate-950 dark:border-white dark:bg-white"
          : "border-border bg-muted",
      )}
    >
      <span
        className={cn(
          "inline-block size-5 rounded-full bg-background shadow-sm transition-transform",
          checked ? "translate-x-5 dark:bg-slate-950" : "translate-x-0.5",
        )}
      />
    </button>
  );
}

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function banReasonLabel(reason: string) {
  if (reason === "qps_limit") return "QPS 触发";
  if (reason === "rpm_limit") return "RPM 触发";
  return "手动封禁";
}

function banSourceLabel(source: string) {
  return source === "auto" ? "自动" : "手动";
}

export default function APISecurity() {
  const { t } = useTranslation();
  const { toast, showToast } = useToast();
  const [form, setForm] = useState<SecurityForm>(defaultForm);
  const [bans, setBans] = useState<IPBan[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [newIP, setNewIP] = useState("");
  const [newExpiresIn, setNewExpiresIn] = useState(0);
  const imageInputRef = useRef<HTMLInputElement>(null);

  const routes = useMemo(
    () => parseMaintenanceRoutesJSON(form.api_maintenance_routes_json),
    [form.api_maintenance_routes_json],
  );
  const maintenanceImagePreview = getMaintenanceImagePreview(
    form.api_maintenance_image_b64_json,
  );

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const [settings, banRes] = await Promise.all([
        api.getSettings(),
        api.getIPBans(true),
      ]);
      setForm({
        global_rpm: settings.global_rpm,
        ip_qps_limit: settings.ip_qps_limit,
        ip_rpm_limit: settings.ip_rpm_limit,
        ip_auto_ban_enabled: settings.ip_auto_ban_enabled,
        ip_auto_ban_duration_minutes:
          settings.ip_auto_ban_duration_minutes || 30,
        ip_auto_ban_on_qps: settings.ip_auto_ban_on_qps,
        ip_auto_ban_on_rpm: settings.ip_auto_ban_on_rpm,
        filter_local_fallback_response: settings.filter_local_fallback_response,
        api_key_disabled_message: settings.api_key_disabled_message,
        api_maintenance_enabled: settings.api_maintenance_enabled,
        api_maintenance_message: settings.api_maintenance_message,
        api_maintenance_sse_randomize: settings.api_maintenance_sse_randomize,
        api_maintenance_image_b64_json: settings.api_maintenance_image_b64_json,
        api_maintenance_routes_json: settings.api_maintenance_routes_json,
      });
      setBans(banRes.bans);
    } catch (error) {
      showToast(`加载失败：${getErrorMessage(error)}`, "error");
    } finally {
      setLoading(false);
    }
  }, [showToast]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const save = async () => {
    setSaving(true);
    try {
      const updated = await api.updateSettings(form);
      setForm((current) => ({
        ...current,
        global_rpm: updated.global_rpm,
        ip_qps_limit: updated.ip_qps_limit,
        ip_rpm_limit: updated.ip_rpm_limit,
        ip_auto_ban_enabled: updated.ip_auto_ban_enabled,
        ip_auto_ban_duration_minutes: updated.ip_auto_ban_duration_minutes,
        ip_auto_ban_on_qps: updated.ip_auto_ban_on_qps,
        ip_auto_ban_on_rpm: updated.ip_auto_ban_on_rpm,
      }));
      showToast("API 防护设置已保存");
    } catch (error) {
      showToast(`保存失败：${getErrorMessage(error)}`, "error");
    } finally {
      setSaving(false);
    }
  };

  const updateRouteGroup = (
    groupKey: string,
    patch: Parameters<typeof updateMaintenanceGroup>[2],
  ) => {
    const group = MAINTENANCE_ROUTE_GROUPS.find(
      (item) => item.key === groupKey,
    );
    if (!group) return;
    setForm((current) => {
      const parsed = parseMaintenanceRoutesJSON(
        current.api_maintenance_routes_json,
      );
      return {
        ...current,
        api_maintenance_routes_json: stringifyMaintenanceRoutes(
          updateMaintenanceGroup(parsed, group, patch),
        ),
      };
    });
  };

  const handleImageUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (!file.type.startsWith("image/")) {
      showToast("请选择图片文件。", "error");
      return;
    }
    try {
      const dataURL = await readFileAsDataURL(file);
      setForm((current) => ({
        ...current,
        api_maintenance_image_b64_json: extractBase64FromDataURL(dataURL),
      }));
      showToast("维护图片已转换为 b64_json");
    } catch {
      showToast("图片读取失败", "error");
    }
  };

  const addBan = async () => {
    const ip = newIP.trim();
    if (!ip) return;
    if (ip.includes("/")) {
      showToast("请输入完整 IP，IP 黑名单已不支持 CIDR", "error");
      return;
    }
    try {
      await api.createIPBan({
        ip,
        reason: "manual",
        source: "manual",
        expires_in_minutes: newExpiresIn,
      });
      setNewIP("");
      setNewExpiresIn(0);
      await reload();
      showToast("IP 已封禁");
    } catch (error) {
      showToast(`封禁失败：${getErrorMessage(error)}`, "error");
    }
  };

  const unban = async (id: number) => {
    try {
      await api.unbanIPBan(id);
      await reload();
      showToast("IP 已解封");
    } catch (error) {
      showToast(`解封失败：${getErrorMessage(error)}`, "error");
    }
  };

  const remove = async (id: number) => {
    try {
      await api.deleteIPBan(id);
      await reload();
      showToast("记录已删除");
    } catch (error) {
      showToast(`删除失败：${getErrorMessage(error)}`, "error");
    }
  };

  if (loading) {
    return (
      <StateShell
        variant="page"
        loading
        loadingTitle="加载 API 防护"
        loadingDescription="正在同步配置与封禁列表。"
      >
        {null}
      </StateShell>
    );
  }

  return (
    <div>
      <ToastNotice toast={toast} />
      <PageHeader
        title="API 防护"
        description="管理 /v1 流量限制、IP 智能封禁、API Key 禁用文案和端点维护响应。"
        onRefresh={() => void reload()}
        actions={
          <Button onClick={() => void save()} disabled={saving}>
            <ShieldCheck className="size-4" />
            {saving ? t("common.saving") : t("common.save")}
          </Button>
        }
      />

      <div className="grid gap-4 xl:grid-cols-2">
        <SecurityCard
          title="流量限制"
          description="所有限制仅作用于 /v1 接口。0 表示关闭对应限制。"
        >
          <div className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-4">
            <Field label="全局 RPM">
              <Input
                type="number"
                min={0}
                value={form.global_rpm}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    global_rpm: parseInt(e.target.value) || 0,
                  }))
                }
              />
            </Field>
            <Field label="IP QPS 限制">
              <Input
                type="number"
                min={0}
                max={10000}
                value={form.ip_qps_limit}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    ip_qps_limit: parseInt(e.target.value) || 0,
                  }))
                }
              />
            </Field>
            <Field label="IP RPM 限制">
              <Input
                type="number"
                min={0}
                value={form.ip_rpm_limit}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    ip_rpm_limit: parseInt(e.target.value) || 0,
                  }))
                }
              />
            </Field>
          </div>
        </SecurityCard>

        <SecurityCard
          title="智能封禁"
          description="触发 IP QPS 或 RPM 限制后，可自动封禁并在到期后自动恢复。"
        >
          <div className="space-y-4">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-sm font-semibold">自动封禁</div>
                <p className="mt-1 text-xs text-muted-foreground">
                  触发限制后写入封禁列表，并记录封禁原因。
                </p>
              </div>
              <ToggleSwitch
                checked={form.ip_auto_ban_enabled}
                onChange={(checked) =>
                  setForm((f) => ({ ...f, ip_auto_ban_enabled: checked }))
                }
                label="自动封禁"
              />
            </div>
            <div className="grid grid-cols-[repeat(auto-fit,minmax(160px,1fr))] gap-4">
              <Field label="封禁时长">
                <Input
                  type="number"
                  min={1}
                  max={10080}
                  value={form.ip_auto_ban_duration_minutes}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      ip_auto_ban_duration_minutes:
                        parseInt(e.target.value) || 30,
                    }))
                  }
                />
                <p className="text-xs text-muted-foreground">单位：分钟</p>
              </Field>
              <Field label="触发来源">
                <div className="flex flex-wrap gap-2">
                  <Button
                    variant={form.ip_auto_ban_on_qps ? "default" : "outline"}
                    size="sm"
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        ip_auto_ban_on_qps: !f.ip_auto_ban_on_qps,
                      }))
                    }
                  >
                    QPS
                  </Button>
                  <Button
                    variant={form.ip_auto_ban_on_rpm ? "default" : "outline"}
                    size="sm"
                    onClick={() =>
                      setForm((f) => ({
                        ...f,
                        ip_auto_ban_on_rpm: !f.ip_auto_ban_on_rpm,
                      }))
                    }
                  >
                    RPM
                  </Button>
                </div>
              </Field>
            </div>
          </div>
        </SecurityCard>
      </div>

      <div className="mt-4">
        <SecurityCard
          title="IP 黑名单"
          description="显示手动封禁和自动封禁记录，仅支持完整 IP。"
        >
          <div className="mb-4 grid gap-3 lg:grid-cols-[minmax(220px,1fr)_180px_auto]">
            <Input
              value={newIP}
              placeholder="203.0.113.20"
              onChange={(e) => setNewIP(e.target.value)}
            />
            <Input
              type="number"
              min={0}
              value={newExpiresIn}
              onChange={(e) => setNewExpiresIn(parseInt(e.target.value) || 0)}
              placeholder="过期分钟，0 永久"
            />
            <Button onClick={() => void addBan()} disabled={!newIP.trim()}>
              <Ban className="size-4" />
              添加封禁
            </Button>
          </div>
          <div className="overflow-hidden rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>IP</TableHead>
                  <TableHead>原因</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>次数</TableHead>
                  <TableHead>封禁时间</TableHead>
                  <TableHead>解封时间</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {bans.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={7}
                      className="h-24 text-center text-muted-foreground"
                    >
                      暂无封禁记录
                    </TableCell>
                  </TableRow>
                ) : (
                  bans.map((ban) => (
                    <TableRow key={ban.id}>
                      <TableCell className="font-mono text-sm">
                        {ban.ip}
                      </TableCell>
                      <TableCell>{banReasonLabel(ban.reason)}</TableCell>
                      <TableCell>{banSourceLabel(ban.source)}</TableCell>
                      <TableCell>{ban.hit_count}</TableCell>
                      <TableCell>{formatDate(ban.banned_at)}</TableCell>
                      <TableCell>
                        {ban.expires_at ? formatDate(ban.expires_at) : "永久"}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="inline-flex gap-1">
                          {ban.enabled && !ban.unbanned_at ? (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => void unban(ban.id)}
                            >
                              <Clock className="size-3.5" />
                              解封
                            </Button>
                          ) : null}
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void remove(ban.id)}
                          >
                            <Trash2 className="size-3.5" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </SecurityCard>
      </div>

      <div className="mt-4">
        <SecurityCard
          title="API 安全与维护"
          description="管理 API Key 禁用文案、本地回退过滤和维护模式响应。"
        >
          <div className="grid gap-5 xl:grid-cols-[minmax(520px,1fr)_320px]">
            <div className="space-y-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-semibold">过滤本地回退响应</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    检测上游 _local_ 响应 ID，命中后返回 502。
                  </p>
                </div>
                <ToggleSwitch
                  checked={form.filter_local_fallback_response}
                  onChange={(checked) =>
                    setForm((f) => ({
                      ...f,
                      filter_local_fallback_response: checked,
                    }))
                  }
                  label="过滤本地回退响应"
                />
              </div>
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-semibold">API 维护模式</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    生成类 POST 接口返回协议兼容的维护响应。
                  </p>
                </div>
                <ToggleSwitch
                  checked={form.api_maintenance_enabled}
                  onChange={(checked) =>
                    setForm((f) => ({ ...f, api_maintenance_enabled: checked }))
                  }
                  label="API 维护模式"
                />
              </div>
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-semibold">随机扰动 SSE 文本</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    流式分片随机拆分，并插入额外空格、标点、繁体字和 emoji
                    数字。
                  </p>
                </div>
                <ToggleSwitch
                  checked={form.api_maintenance_sse_randomize}
                  onChange={(checked) =>
                    setForm((f) => ({
                      ...f,
                      api_maintenance_sse_randomize: checked,
                    }))
                  }
                  label="随机扰动 SSE 文本"
                />
              </div>
              <Field label="API Key 禁用文案">
                <textarea
                  className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-3 text-sm leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={form.api_key_disabled_message}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      api_key_disabled_message: e.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="默认维护文案">
                <textarea
                  className="min-h-20 w-full rounded-md border border-input bg-background px-3 py-3 text-sm leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={form.api_maintenance_message}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      api_maintenance_message: e.target.value,
                    }))
                  }
                />
              </Field>
            </div>
            <div className="space-y-3">
              <div>
                <div className="text-sm font-semibold">默认图片 b64_json</div>
                <p className="mt-1 text-xs text-muted-foreground">
                  用于图片维护响应。
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => imageInputRef.current?.click()}
                >
                  <Upload className="size-4" />
                  选择图片
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    setForm((f) => ({
                      ...f,
                      api_maintenance_image_b64_json: "",
                    }))
                  }
                >
                  <X className="size-4" />
                  清除图片
                </Button>
              </div>
              <input
                ref={imageInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handleImageUpload}
              />
              <div className="flex h-[220px] w-full items-center justify-center rounded-lg border border-border bg-muted/15 p-4">
                {maintenanceImagePreview ? (
                  <img
                    src={maintenanceImagePreview}
                    alt="维护图片预览"
                    className="max-h-[180px] max-w-[260px] rounded-lg object-contain shadow-sm"
                  />
                ) : (
                  <div className="text-center text-sm text-muted-foreground">
                    未选择维护图片
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="mt-5 border-t border-border pt-4">
            <div className="mb-3">
              <div className="text-sm font-semibold text-foreground">
                按服务设置返回
              </div>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">
                Codex 会同时控制 /v1/responses 和 /v1/responses/compact。
              </p>
            </div>
            <div className="grid gap-3 xl:grid-cols-2">
              {MAINTENANCE_ROUTE_GROUPS.map((group) => {
                const route = getMaintenanceGroupConfig(routes, group);
                const enabled = route.enabled !== false;
                return (
                  <div
                    key={group.key}
                    className="space-y-3 rounded-lg border border-border bg-muted/15 p-3"
                  >
                    <div className="flex items-center justify-between gap-4">
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold text-foreground">
                          {group.label}
                        </div>
                        <p
                          className="mt-1 truncate font-mono text-[11px] text-muted-foreground"
                          title={group.paths.join(", ")}
                        >
                          {group.paths.join(" / ")}
                        </p>
                      </div>
                      <ToggleSwitch
                        checked={enabled}
                        onChange={(checked) =>
                          updateRouteGroup(group.key, {
                            enabled: checked ? undefined : false,
                          })
                        }
                        label={group.label}
                      />
                    </div>
                    <Input
                      value={route.message ?? ""}
                      placeholder="留空使用全局维护内容"
                      onChange={(e) =>
                        updateRouteGroup(group.key, { message: e.target.value })
                      }
                    />
                  </div>
                );
              })}
            </div>
          </div>
        </SecurityCard>
      </div>
      <div className="mt-4 flex items-center gap-2 rounded-lg border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">
        <AlertTriangle className="size-4 shrink-0" />
        自动封禁触发后会立即影响 /v1 请求，请根据实际流量设置合理阈值。
      </div>
    </div>
  );
}
