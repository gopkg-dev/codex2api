import {
  type ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import { ShieldCheck, Upload, X } from "lucide-react";
import PageHeader from "../components/PageHeader";
import StateShell from "../components/StateShell";
import ToastNotice from "../components/ToastNotice";
import { useToast } from "../hooks/useToast";
import { api } from "../api";
import type { SystemSettings } from "../types";
import { getErrorMessage } from "../utils/error";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
  | "disable_fast_service_tier"
  | "image_generation_tool_mode"
  | "downstream_usage_multiplier"
  | "protocol_message_usage_blast_enabled"
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
  disable_fast_service_tier: false,
  image_generation_tool_mode: "auto",
  downstream_usage_multiplier: 1,
  protocol_message_usage_blast_enabled: false,
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

export default function APISecurity() {
  const { t } = useTranslation();
  const { toast, showToast } = useToast();
  const [form, setForm] = useState<SecurityForm>(defaultForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
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
      const settings = await api.getSettings();
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
        disable_fast_service_tier: settings.disable_fast_service_tier,
        image_generation_tool_mode: settings.image_generation_tool_mode || "auto",
        downstream_usage_multiplier: settings.downstream_usage_multiplier,
        protocol_message_usage_blast_enabled:
          settings.protocol_message_usage_blast_enabled,
        api_maintenance_message: settings.api_maintenance_message,
        api_maintenance_sse_randomize: settings.api_maintenance_sse_randomize,
        api_maintenance_image_b64_json: settings.api_maintenance_image_b64_json,
        api_maintenance_routes_json: settings.api_maintenance_routes_json,
      });
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
        disable_fast_service_tier: updated.disable_fast_service_tier,
        image_generation_tool_mode: updated.image_generation_tool_mode || "auto",
        downstream_usage_multiplier: updated.downstream_usage_multiplier,
        protocol_message_usage_blast_enabled:
          updated.protocol_message_usage_blast_enabled,
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

  if (loading) {
    return (
      <StateShell
        variant="page"
        loading
        loadingTitle="加载 API 防护"
        loadingDescription="正在同步防护配置。"
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
        description="管理 /v1 流量限制、IP 智能封禁策略和端点维护响应。"
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
          title="API 安全与维护"
          description="管理本地回退过滤、用量响应和维护提示内容。"
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
                  <div className="text-sm font-semibold">关闭 OpenAI Fast</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    拦截 service_tier=fast/priority，按普通请求透传给 OpenAI
                    网关。
                  </p>
                </div>
                <ToggleSwitch
                  checked={form.disable_fast_service_tier}
                  onChange={(checked) =>
                    setForm((f) => ({
                      ...f,
                      disable_fast_service_tier: checked,
                    }))
                  }
                  label="关闭 OpenAI Fast"
                />
              </div>
              <div className="rounded-lg border border-border p-4">
                <div>
                  <div className="text-sm font-semibold">图片工具</div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    控制 Codex /responses 的 image_generation 工具注入。
                  </p>
                </div>
                <div className="mt-3 grid gap-2 sm:grid-cols-3">
                  {[
                    {
                      value: "auto",
                      label: "自动",
                      desc: "按请求特征注入",
                    },
                    {
                      value: "force_on",
                      label: "强制开启",
                      desc: "始终注入图片工具",
                    },
                    {
                      value: "force_off",
                      label: "强制关闭",
                      desc: "移除图片工具",
                    },
                  ].map((option) => {
                    const active =
                      form.image_generation_tool_mode === option.value;
                    return (
                      <button
                        key={option.value}
                        type="button"
                        onClick={() =>
                          setForm((f) => ({
                            ...f,
                            image_generation_tool_mode:
                              option.value as SecurityForm["image_generation_tool_mode"],
                          }))
                        }
                        className={cn(
                          "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                          active
                            ? "border-primary bg-primary/10 text-primary"
                            : "border-border bg-background hover:bg-muted",
                        )}
                      >
                        <span className="block font-semibold">
                          {option.label}
                        </span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {option.desc}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
              <div className="rounded-lg border border-border p-4">
                <div>
                  <div className="text-sm font-semibold">
                    下游 cached_tokens 倍数
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    调整真实上游响应返回给下游的缓存命中用量，默认 1 倍。
                  </p>
                </div>
                <div className="mt-3 max-w-xs">
                  <Field label="倍数">
                    <input
                      type="number"
                      min="0.01"
                      max="1000"
                      step="0.01"
                      className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      value={form.downstream_usage_multiplier}
                      onChange={(e) =>
                        setForm((f) => ({
                          ...f,
                          downstream_usage_multiplier:
                            Number(e.target.value) || 1,
                        }))
                      }
                    />
                  </Field>
                </div>
                <div className="mt-4 flex items-start justify-between gap-4 border-t border-border pt-4">
                  <div>
                    <div className="text-sm font-semibold">搞爆下游上下文</div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      将维护、密钥和风控提示响应的 usage 放大 99999 倍。
                    </p>
                  </div>
                  <ToggleSwitch
                    checked={form.protocol_message_usage_blast_enabled}
                    onChange={(checked) =>
                      setForm((f) => ({
                        ...f,
                        protocol_message_usage_blast_enabled: checked,
                      }))
                    }
                    label="搞爆下游上下文"
                  />
                </div>
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
              <Field
                label="默认维护文案"
                description="追加到维护、密钥不可用、限流与自动封禁的协议提示中。"
              >
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
                const enabled = route.enabled === true;
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
                            enabled: checked,
                          })
                        }
                        label={group.label}
                      />
                    </div>
                    <p className="text-xs leading-relaxed text-muted-foreground">
                      返回文案：{group.messagePrefix}，
                      {form.api_maintenance_message.trim() ||
                        "系统维护中，请稍后重试。"}
                    </p>
                  </div>
                );
              })}
            </div>
          </div>
        </SecurityCard>
      </div>
    </div>
  );
}
