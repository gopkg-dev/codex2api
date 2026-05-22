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
import Modal from "../components/Modal";
import Pagination from "../components/Pagination";
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

function parseDateTime(value?: string) {
  if (!value) return null;
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? time : null;
}

function formatDuration(ms: number) {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  if (totalSeconds <= 0) return "刚刚";
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (days > 0) return `${days}天${hours}小时`;
  if (hours > 0) return `${hours}小时${minutes}分钟`;
  if (minutes > 0) return `${minutes}分钟${seconds}秒`;
  return `${seconds}秒`;
}

function isPastDate(value: string | undefined, now: number) {
  if (!value) return false;
  const time = parseDateTime(value);
  return time !== null && time <= now;
}

function banStatus(ban: IPBan, now: number) {
  if (ban.unbanned_at) {
    return {
      label: "已解封",
      className:
        "border-slate-500/25 bg-slate-500/10 text-slate-600 dark:text-slate-300",
    };
  }
  if (isPastDate(ban.expires_at, now)) {
    return {
      label: "已解封",
      className:
        "border-slate-500/25 bg-slate-500/10 text-slate-600 dark:text-slate-300",
    };
  }
  return {
    label: "封禁中",
    className:
      "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  };
}

function banReleaseTimeLabel(ban: IPBan, now: number) {
  const unbannedAt = parseDateTime(ban.unbanned_at);
  if (unbannedAt !== null) return `已于 ${formatDate(ban.unbanned_at)} 解封`;
  const expiresAt = parseDateTime(ban.expires_at);
  if (expiresAt !== null) {
    if (expiresAt > now) return `剩余 ${formatDuration(expiresAt - now)}`;
    return `已于 ${formatDate(ban.expires_at)} 解封`;
  }
  return "永久";
}

function parseIPBanInput(value: string): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const line of value.split(/\r?\n/)) {
    const ip = line.trim();
    if (!ip || ip.startsWith("#") || seen.has(ip)) continue;
    seen.add(ip);
    result.push(ip);
  }
  return result;
}

export default function APISecurity() {
  const { t } = useTranslation();
  const { toast, showToast } = useToast();
  const [form, setForm] = useState<SecurityForm>(defaultForm);
  const [bans, setBans] = useState<IPBan[]>([]);
  const [bansTotal, setBansTotal] = useState(0);
  const [banPage, setBanPage] = useState(1);
  const [banPageSize, setBanPageSize] = useState(20);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [newIPs, setNewIPs] = useState("");
  const [newExpiresIn, setNewExpiresIn] = useState(0);
  const [batchBanOpen, setBatchBanOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const imageInputRef = useRef<HTMLInputElement>(null);
  const pendingIPs = useMemo(() => parseIPBanInput(newIPs), [newIPs]);

  const routes = useMemo(
    () => parseMaintenanceRoutesJSON(form.api_maintenance_routes_json),
    [form.api_maintenance_routes_json],
  );
  const maintenanceImagePreview = getMaintenanceImagePreview(
    form.api_maintenance_image_b64_json,
  );

  const reload = useCallback(async (options?: { page?: number; pageSize?: number }) => {
    setLoading(true);
    try {
      const nextPage = options?.page ?? banPage;
      const nextPageSize = options?.pageSize ?? banPageSize;
      const [settings, banRes] = await Promise.all([
        api.getSettings(),
        api.getIPBans(true, { page: nextPage, pageSize: nextPageSize }),
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
      setBansTotal(banRes.total);
    } catch (error) {
      showToast(`加载失败：${getErrorMessage(error)}`, "error");
    } finally {
      setLoading(false);
    }
  }, [banPage, banPageSize, showToast]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

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
    if (pendingIPs.length === 0) return;
    if (pendingIPs.some((ip) => ip.includes("/"))) {
      showToast("请输入完整 IP，IP 黑名单已不支持 CIDR", "error");
      return;
    }
    try {
      const result = await api.createIPBansBatch({
        ips: pendingIPs,
        reason: "manual",
        source: "manual",
        expires_in_minutes: newExpiresIn,
      });
      setNewIPs("");
      setNewExpiresIn(0);
      setBatchBanOpen(false);
      if (banPage === 1) {
        await reload({ page: 1 });
      } else {
        setBanPage(1);
      }
      if (result.error_count > 0) {
        showToast(
          `已封禁 ${result.created} 个，失败 ${result.error_count} 个`,
        );
      } else {
        showToast(`已封禁 ${result.created} 个 IP`);
      }
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

  const banTotalPages = Math.max(1, Math.ceil(bansTotal / banPageSize));
  const currentBanPage = Math.min(banPage, banTotalPages);

  useEffect(() => {
    if (banPage > banTotalPages) {
      setBanPage(banTotalPages);
    }
  }, [banPage, banTotalPages]);

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
          <div className="mb-4 flex items-center justify-between gap-3">
            <div className="text-xs text-muted-foreground">
              当前共 {bansTotal} 条封禁记录
            </div>
            <Button onClick={() => setBatchBanOpen(true)}>
              <Ban className="size-4" />
              批量添加 IP
            </Button>
          </div>
          <div className="overflow-hidden rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>IP</TableHead>
                  <TableHead>原因</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead>次数</TableHead>
                  <TableHead>最后封禁时间</TableHead>
                  <TableHead>解封/到期倒计时</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {bans.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={8}
                      className="h-24 text-center text-muted-foreground"
                    >
                      暂无封禁记录
                    </TableCell>
                  </TableRow>
                ) : (
                  bans.map((ban) => {
                    const status = banStatus(ban, now);
                    return (
                      <TableRow key={ban.id}>
                        <TableCell className="font-mono text-sm">
                          {ban.ip}
                        </TableCell>
                        <TableCell>{banReasonLabel(ban.reason)}</TableCell>
                        <TableCell>{banSourceLabel(ban.source)}</TableCell>
                        <TableCell>
                          <span
                            className={`inline-flex rounded-md border px-2 py-0.5 text-xs font-semibold ${status.className}`}
                          >
                            {status.label}
                          </span>
                        </TableCell>
                        <TableCell>{ban.hit_count}</TableCell>
                        <TableCell title={formatDate(ban.banned_at)}>
                          {formatDate(ban.banned_at)}
                        </TableCell>
                        <TableCell
                          title={formatDate(ban.unbanned_at || ban.expires_at)}
                        >
                          {banReleaseTimeLabel(ban, now)}
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
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>
          <Pagination
            page={currentBanPage}
            totalPages={banTotalPages}
            totalItems={bansTotal}
            pageSize={banPageSize}
            onPageChange={setBanPage}
            onPageSizeChange={(next) => {
              setBanPage(1);
              setBanPageSize(next);
            }}
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </SecurityCard>
      </div>

      <Modal
        show={batchBanOpen}
        title="批量添加黑名单 IP"
        onClose={() => setBatchBanOpen(false)}
        contentClassName="sm:max-w-xl"
        footer={
          <div className="flex w-full items-center justify-between gap-3">
            <div className="text-xs text-muted-foreground">
              已识别 {pendingIPs.length} 个 IP
            </div>
            <div className="flex gap-2">
              <Button variant="outline" onClick={() => setBatchBanOpen(false)}>
                取消
              </Button>
              <Button
                onClick={() => void addBan()}
                disabled={pendingIPs.length === 0}
              >
                <Ban className="size-4" />
                添加
              </Button>
            </div>
          </div>
        }
      >
        <div className="space-y-4">
          <Field label="IP 列表" description="每行一个完整 IP，支持 IPv4 和 IPv6。">
            <textarea
              value={newIPs}
              placeholder={"203.0.113.20\n203.0.113.21\n2001:db8::1"}
              onChange={(e) => setNewIPs(e.target.value)}
              className="min-h-52 w-full resize-y rounded-md border border-input bg-background px-3 py-3 font-mono text-sm leading-relaxed outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </Field>
          <Field label="过期分钟" description="0 表示永久封禁。">
            <Input
              type="number"
              min={0}
              value={newExpiresIn}
              onChange={(e) => setNewExpiresIn(parseInt(e.target.value) || 0)}
              placeholder="过期分钟，0 永久"
            />
          </Field>
        </div>
      </Modal>

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
