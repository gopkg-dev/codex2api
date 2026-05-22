import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Copy,
  CopyCheck,
  Cpu,
  ExternalLink,
  HardDrive,
  Info,
  Laptop,
  Monitor,
  Radio,
  RefreshCw,
  SquareTerminal,
  Terminal,
  Users,
  Wrench,
} from "lucide-react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { api } from "../api";
import { useBranding } from "../branding";
import {
  getBucketConfig,
  getTimeRangeISO,
  type TimeRangeKey,
} from "../lib/timeRange";
import type {
  ChartAggregation,
  IPStatsWindow,
  IPUsageStat,
  PublicAccountPoolStats,
  PublicHomeResponse,
  PublicMaintenanceRoute,
  UsageModelStat,
} from "../types";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import Modal from "../components/Modal";
import ToastNotice from "../components/ToastNotice";
import { useToast } from "../hooks/useToast";

const PUBLIC_REFRESH_INTERVAL_SECONDS = 5;
const PUBLIC_TIME_RANGES: TimeRangeKey[] = ["1h", "6h", "24h", "7d"];
const IP_STATS_WINDOWS: Array<{ key: IPStatsWindow; label: string }> = [
  { key: "1m", label: "1分钟" },
  { key: "5m", label: "5分钟" },
  { key: "15m", label: "15分钟" },
  { key: "1h", label: "1小时" },
  { key: "today", label: "今日" },
];
const QQ_GROUP_NUMBER = "1054851130";
const QQ_GROUP_URL = "https://qm.qq.com/q/PphhfxKPee";
const CC_SWITCH_PROTOCOL_ERROR =
  "CC-Switch 未安装或协议处理程序未注册。请先安装 CC-Switch 或手动复制 API 密钥。";
const PUBLIC_MAINTENANCE_ROUTE_META: Record<
  string,
  { label: string; order: number }
> = {
  "/v1/chat/completions": { label: "OpenAI Chat", order: 10 },
  "/v1/images/edits": { label: "GPT生图", order: 20 },
  "/v1/images/generations": { label: "GPT生图", order: 20 },
  "/v1/messages": { label: "Claude", order: 40 },
  "/v1/responses": { label: "Codex", order: 50 },
  "/v1/responses/compact": { label: "Codex", order: 50 },
};

export default function PublicHome() {
  const { siteName } = useBranding();
  const { toast, showToast } = useToast();
  const [overview, setOverview] = useState<PublicHomeResponse | null>(null);
  const [chartData, setChartData] = useState<ChartAggregation | null>(null);
  const [timeRange, setTimeRange] = useState<TimeRangeKey>("24h");
  const [loading, setLoading] = useState(true);
  const [chartLoading, setChartLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshCountdown, setRefreshCountdown] = useState(
    PUBLIC_REFRESH_INTERVAL_SECONDS,
  );
  const [error, setError] = useState("");
  const [copiedTarget, setCopiedTarget] = useState("");
  const [docsOpen, setDocsOpen] = useState(false);
  const [ipStatsOpen, setIPStatsOpen] = useState(false);
  const [ipStatsWindow, setIPStatsWindow] = useState<IPStatsWindow>("5m");
  const [ipStatsLoading, setIPStatsLoading] = useState(false);
  const [docsInitialTab, setDocsInitialTab] =
    useState<PublicDocsTopTab>("codex");
  const chartAbort = useRef<AbortController | null>(null);
  const copyResetTimer = useRef<number | null>(null);
  const ccSwitchFallbackTimer = useRef<number | null>(null);
  const refreshInFlight = useRef(false);
  const ipStatsWindowRef = useRef<IPStatsWindow>("5m");
  const overviewRequestSeq = useRef(0);
  const ipStatsLoadSeq = useRef(0);

  const loadOverview = useCallback(async (ipWindow?: IPStatsWindow) => {
    const requestSeq = overviewRequestSeq.current + 1;
    overviewRequestSeq.current = requestSeq;
    const res = await api.getPublicHome({
      ipWindow: ipWindow ?? ipStatsWindowRef.current,
    });
    if (requestSeq === overviewRequestSeq.current) {
      setOverview(res);
    }
  }, []);

  const loadChart = useCallback(
    async (options?: { background?: boolean }) => {
      chartAbort.current?.abort();
      const controller = new AbortController();
      chartAbort.current = controller;
      if (!options?.background) {
        setChartLoading(true);
      }
      try {
        const { start, end } = getTimeRangeISO(timeRange);
        const { bucketMinutes } = getBucketConfig(timeRange);
        const res = await api.getPublicChartData({ start, end, bucketMinutes });
        if (!controller.signal.aborted) {
          setChartData(res);
        }
      } finally {
        if (!controller.signal.aborted) {
          setChartLoading(false);
        }
      }
    },
    [timeRange],
  );

  const reload = useCallback(
    async (options?: { background?: boolean }) => {
      if (refreshInFlight.current) return;
      refreshInFlight.current = true;
      setError("");
      setRefreshing(true);
      try {
        await Promise.all([loadOverview(), loadChart(options)]);
      } catch (err) {
        setError(err instanceof Error ? err.message : "加载失败");
      } finally {
        setLoading(false);
        setRefreshing(false);
        setRefreshCountdown(PUBLIC_REFRESH_INTERVAL_SECONDS);
        refreshInFlight.current = false;
      }
    },
    [loadChart, loadOverview],
  );

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    return () => {
      if (copyResetTimer.current !== null) {
        window.clearTimeout(copyResetTimer.current);
      }
      if (ccSwitchFallbackTimer.current !== null) {
        window.clearTimeout(ccSwitchFallbackTimer.current);
      }
    };
  }, []);

  const copyText = useCallback(async (text: string, target: string) => {
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopiedTarget(target);
      if (copyResetTimer.current !== null) {
        window.clearTimeout(copyResetTimer.current);
      }
      copyResetTimer.current = window.setTimeout(() => {
        setCopiedTarget((current) => (current === target ? "" : current));
      }, 1200);
    } catch {
      setError("复制失败");
    }
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState !== "visible") return;
      setRefreshCountdown((current) => {
        if (current <= 1) {
          void reload({ background: true });
          return PUBLIC_REFRESH_INTERVAL_SECONDS;
        }
        return current - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [reload]);

  const chartPoints = useMemo(() => {
    if (!chartData) return [];
    const { bucketMinutes } = getBucketConfig(timeRange);
    return chartData.timeline.map((point) => {
      const tokenTotal =
        point.input_tokens +
        point.output_tokens +
        point.reasoning_tokens +
        point.cached_tokens;
      const date = new Date(point.bucket);
      return {
        label: formatChartLabel(date, timeRange),
        fullLabel: date.toLocaleString(),
        rpm: round(point.requests / bucketMinutes, 2),
        tpm: round(tokenTotal / bucketMinutes, 2),
      };
    });
  }, [chartData, timeRange]);

  const usage = overview?.usage;
  const ops = overview?.ops;
  const accountPool = overview?.account_pool;
  const maintenance = overview?.maintenance;
  const latestKey = overview?.latest_key?.trim() ?? "";
  const modelStats = usage?.model_stats ?? [];
  const ipStats = ops?.ip_stats ?? [];
  const ipStatsTotal = ops?.ip_stats_total ?? ipStats.length;
  const publicApiBase = window.location.origin;
  const serviceAvailable = hasPublicAvailableRoutes(maintenance);
  const codexCcSwitchUrl = buildCcSwitchImportUrl({
    app: "codex",
    baseUrl: publicApiBase,
    apiKey: latestKey,
    name: siteName,
    model: "gpt-5.4",
  });
  const claudeCcSwitchUrl = buildCcSwitchImportUrl({
    app: "claude",
    baseUrl: publicApiBase,
    apiKey: latestKey,
    name: siteName,
  });
  const openDocs = useCallback((tab: PublicDocsTopTab) => {
    setDocsInitialTab(tab);
    setDocsOpen(true);
  }, []);

  const changeIPStatsWindow = useCallback(
    (nextWindow: IPStatsWindow) => {
      if (nextWindow === ipStatsWindowRef.current) return;
      ipStatsWindowRef.current = nextWindow;
      const loadSeq = ipStatsLoadSeq.current + 1;
      ipStatsLoadSeq.current = loadSeq;
      setIPStatsWindow(nextWindow);
      setIPStatsLoading(true);
      setError("");
      void loadOverview(nextWindow)
        .catch((err) => {
          setError(err instanceof Error ? err.message : "加载失败");
        })
        .finally(() => {
          if (loadSeq === ipStatsLoadSeq.current) {
            setIPStatsLoading(false);
            setRefreshCountdown(PUBLIC_REFRESH_INTERVAL_SECONDS);
          }
        });
    },
    [loadOverview],
  );
  const showCcSwitchFallback = useCallback(() => {
    if (ccSwitchFallbackTimer.current !== null) {
      window.clearTimeout(ccSwitchFallbackTimer.current);
    }
    ccSwitchFallbackTimer.current = window.setTimeout(() => {
      if (document.visibilityState === "visible" && document.hasFocus()) {
        showToast(CC_SWITCH_PROTOCOL_ERROR, "error");
      }
      ccSwitchFallbackTimer.current = null;
    }, 1200);
  }, [showToast]);

  return (
    <main className="min-h-screen bg-background px-4 py-5 text-foreground sm:px-6 lg:px-8">
      <ToastNotice toast={toast} />
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5">
        <header className="flex flex-col gap-4 rounded-lg border border-border bg-card px-4 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="mb-2 inline-flex items-center gap-2 rounded-md border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-semibold text-primary">
              <Radio className="size-3.5" />
              Public Status
            </div>
            <h1 className="text-2xl font-bold tracking-normal text-foreground sm:text-3xl">
              {siteName}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              公益站实时状态，永久免费, 定时轮换Key，旧Key自动失效。
            </p>
          </div>
          <div className="flex min-w-0 flex-col items-start gap-2 sm:items-end">
            <div className="flex flex-wrap items-center gap-2 sm:justify-end">
              <a
                href={QQ_GROUP_URL}
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-8 items-center gap-2 rounded-md border border-primary/20 bg-primary/10 px-3 text-sm font-semibold text-primary transition-colors hover:bg-primary/15"
              >
                <span
                  className="inline-flex size-4 items-center justify-center text-base leading-none"
                  aria-hidden="true"
                >
                  🐧
                </span>
                QQ 群 {QQ_GROUP_NUMBER}
              </a>
              <CopyIconButton
                label="复制 QQ 群号"
                copied={copiedTarget === "qq"}
                onClick={() => void copyText(QQ_GROUP_NUMBER, "qq")}
              />
              <StatusPill serviceAvailable={serviceAvailable} />
              <div className="inline-flex h-8 items-center gap-2 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 text-sm font-semibold text-emerald-700 dark:text-emerald-300">
                <RefreshCw
                  className={`size-3.5 ${refreshing ? "animate-spin" : ""}`}
                />
                5s 自动刷新
                <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 font-mono text-xs tabular-nums">
                  {refreshCountdown}s
                </span>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void reload()}
                disabled={loading || refreshing}
              >
                <RefreshCw
                  className={`size-4 ${loading || refreshing ? "animate-spin" : ""}`}
                />
                刷新
              </Button>
            </div>
            <div className="flex max-w-full flex-wrap items-center gap-2 sm:justify-end">
              <div className="flex max-w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-sm">
                <span className="shrink-0 font-semibold text-muted-foreground">
                  API 地址
                </span>
                <span
                  className="min-w-0 max-w-[min(50vw,260px)] truncate font-mono text-xs font-semibold tabular-nums text-foreground"
                  title={publicApiBase}
                >
                  {publicApiBase}
                </span>
                <CopyIconButton
                  label="复制 API 地址"
                  copied={copiedTarget === "api_base"}
                  onClick={() => void copyText(publicApiBase, "api_base")}
                />
              </div>
              <div className="flex max-w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-sm">
                <span className="shrink-0 font-semibold text-muted-foreground">
                  最新密钥
                </span>
                <span
                  className="min-w-0 max-w-[min(50vw,320px)] truncate font-mono text-xs font-semibold tabular-nums text-foreground"
                  title={latestKey}
                >
                  {latestKey || "-"}
                </span>
                <CopyIconButton
                  label="复制最新密钥"
                  copied={copiedTarget === "latest_key"}
                  disabled={!latestKey}
                  onClick={() => void copyText(latestKey, "latest_key")}
                />
              </div>
            </div>
          </div>
        </header>

        {error && (
          <div className="rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <MaintenanceStatusCard
          maintenance={maintenance}
          loading={loading}
          onOpenDocs={openDocs}
          codexCcSwitchUrl={codexCcSwitchUrl}
          claudeCcSwitchUrl={claudeCcSwitchUrl}
          ccSwitchDisabled={!latestKey}
          onCcSwitchImport={showCcSwitchFallback}
        />
        <PublicUsageDocsModal
          show={docsOpen}
          initialTab={docsInitialTab}
          baseUrl={publicApiBase}
          apiKey={latestKey}
          copiedTarget={copiedTarget}
          onCopy={(text, target) => void copyText(text, target)}
          onClose={() => setDocsOpen(false)}
        />
        <PublicIPStatsModal
          show={ipStatsOpen}
          stats={ipStats}
          total={ipStatsTotal}
          windowKey={ipStatsWindow}
          onWindowChange={changeIPStatsWindow}
          loading={loading || ipStatsLoading}
          onClose={() => setIPStatsOpen(false)}
        />

        <section className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <SummaryDashboardCard
            icon={<Activity className="size-5" />}
            title="使用统计"
            description="请求、词元与费用"
            loading={loading}
            headerRight={
              <div
                className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-emerald-500/25 bg-emerald-500/10 px-2.5 text-emerald-700 dark:text-emerald-300"
                title={`实时请求 ${formatInteger(ops?.requests.active)}`}
              >
                <span className="size-1.5 rounded-full bg-emerald-500" />
                <span className="text-xs font-semibold leading-none">
                  实时请求
                </span>
                <span className="font-mono text-sm font-bold leading-none tabular-nums">
                  {loading ? "-" : formatInteger(ops?.requests.active)}
                </span>
              </div>
            }
            metrics={[
              {
                label: "总请求数",
                value: formatInteger(usage?.total_requests),
              },
              {
                label: "今日请求",
                value: formatInteger(usage?.today_requests),
              },
              {
                label: "总计词元",
                value: formatCompactNumber(usage?.total_tokens),
              },
              {
                label: "今日词元",
                value: formatCompactNumber(usage?.today_tokens),
              },
              {
                label: "总计费用",
                value: formatMoneyFixed2(usage?.total_user_billed),
              },
              {
                label: "今日费用",
                value: formatMoneyFixed2(usage?.today_user_billed),
              },
            ]}
          />
          <AccountPoolCard accountPool={accountPool} loading={loading} />
          <SummaryDashboardCard
            icon={<HardDrive className="size-5" />}
            title="缓存 + 健康"
            description="命中率、延迟和错误率"
            loading={loading}
            metrics={[
              {
                label: "今日命中率",
                value: formatPercent(usage?.today_cache_rate),
              },
              {
                label: "总命中率",
                value: formatPercent(usage?.total_cache_rate),
              },
              {
                label: "首 Token",
                value: formatLatency(usage?.avg_first_token_ms),
              },
              {
                label: "完成延迟",
                value: formatLatency(usage?.avg_duration_ms),
              },
              {
                label: "今日缓存 Token",
                value: formatCompactNumber(usage?.today_cached_tokens),
              },
              { label: "错误率", value: formatPercent(usage?.error_rate) },
            ]}
          />
          <SummaryDashboardCard
            icon={<Cpu className="size-5" />}
            title="系统负载"
            description="进程与主机资源"
            loading={loading}
            compactMetrics
            headerRight={
              <div
                className="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-full border border-emerald-500/25 bg-emerald-500/10 px-2.5 text-emerald-700 dark:text-emerald-300"
                title={`全局限制 ${
                  (ops?.traffic.rpm_limit ?? 0) > 0
                    ? `${formatInteger(ops?.traffic.rpm_limit)} RPM`
                    : "不限"
                }`}
              >
                <span className="size-1.5 rounded-full bg-emerald-500" />
                <span className="text-xs font-semibold leading-none">
                  全局限制
                </span>
                <span className="font-mono text-sm font-bold leading-none tabular-nums">
                  {(ops?.traffic.rpm_limit ?? 0) > 0
                    ? `${formatInteger(ops?.traffic.rpm_limit)} RPM`
                    : "不限"}
                </span>
              </div>
            }
            metrics={[
              {
                label: "RPM",
                value: formatDecimal(ops?.traffic.rpm, 0),
              },
              {
                label: "TPM",
                value: formatDecimal(ops?.traffic.tpm, 0),
              },
              {
                label: "CPU",
                value: `${formatDecimal(ops?.cpu.percent, 1)}%`,
                sub: `${ops?.cpu.cores ?? 0} cores`,
              },
              {
                label: "内存",
                value: `${formatDecimal(ops?.memory.percent, 1)}%`,
                sub: `${formatBytes(ops?.memory.used_bytes)} / ${formatBytes(ops?.memory.total_bytes)}`,
              },
              {
                label: "协程",
                value: formatInteger(ops?.runtime.goroutines),
                sub: `${formatInteger(ops?.requests.active)} active`,
              },
              {
                label: "网络",
                value: formatBytes(ops?.network.total_bytes),
                sub: `RX ${formatBytesCompact(ops?.network.rx_bytes)} TX ${formatBytesCompact(ops?.network.tx_bytes)}`,
              },
            ]}
          />
        </section>

        <section className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)]">
          <Card className="h-[420px] py-0">
            <CardContent className="flex h-full min-h-0 flex-col p-4 sm:p-5">
              <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-foreground">
                    RPM / TPM 趋势
                  </h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    按时间窗口动态聚合
                  </p>
                </div>
                <div className="inline-flex w-fit rounded-lg border border-border bg-muted/50 p-0.5">
                  {PUBLIC_TIME_RANGES.map((key) => (
                    <button
                      key={key}
                      type="button"
                      onClick={() => setTimeRange(key)}
                      className={`rounded-md px-3 py-1.5 text-xs font-semibold transition-colors ${
                        timeRange === key
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground"
                      }`}
                    >
                      {key.toUpperCase()}
                    </button>
                  ))}
                </div>
              </div>
              <div className="min-h-0 flex-1">
                {chartLoading ? (
                  <ChartSkeleton />
                ) : chartPoints.length > 0 ? (
                  <ResponsiveContainer width="100%" height="100%">
                    <LineChart
                      data={chartPoints}
                      margin={{ top: 8, right: 18, left: 0, bottom: 0 }}
                    >
                      <CartesianGrid
                        vertical={false}
                        stroke="var(--color-border)"
                        strokeDasharray="4 4"
                      />
                      <XAxis
                        dataKey="label"
                        tick={{
                          fill: "var(--color-muted-foreground)",
                          fontSize: 12,
                        }}
                        tickLine={false}
                        axisLine={{ stroke: "var(--color-border)" }}
                        minTickGap={18}
                      />
                      <YAxis
                        tick={{
                          fill: "var(--color-muted-foreground)",
                          fontSize: 12,
                        }}
                        tickLine={false}
                        axisLine={{ stroke: "var(--color-border)" }}
                        tickFormatter={formatChartAxisValue}
                        width={56}
                      />
                      <Tooltip
                        labelFormatter={(_, payload) =>
                          payload?.[0]?.payload?.fullLabel ?? ""
                        }
                        formatter={(value, name) => [
                          formatChartTooltipValue(value, name),
                          String(name).toUpperCase(),
                        ]}
                        contentStyle={{
                          backgroundColor: "var(--color-card)",
                          border: "1px solid var(--color-border)",
                          borderRadius: 8,
                        }}
                        labelStyle={{
                          color: "var(--color-foreground)",
                          fontWeight: 600,
                        }}
                      />
                      <Legend wrapperStyle={{ fontSize: 12 }} />
                      <Line
                        type="monotone"
                        dataKey="rpm"
                        name="RPM"
                        stroke="#059669"
                        strokeWidth={2.2}
                        dot={false}
                      />
                      <Line
                        type="monotone"
                        dataKey="tpm"
                        name="TPM"
                        stroke="#7c3aed"
                        strokeWidth={2.2}
                        dot={false}
                      />
                    </LineChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="flex h-full items-center justify-center rounded-lg border border-dashed border-border bg-muted/20 text-sm text-muted-foreground">
                    暂无图表数据
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          <ModelStatsCard
            stats={modelStats}
            loading={loading}
            ipStatsTotal={ipStatsTotal}
            onOpenIPStats={() => setIPStatsOpen(true)}
          />
        </section>
      </div>
    </main>
  );
}

function StatusPill({ serviceAvailable }: { serviceAvailable: boolean }) {
  return (
    <div
      className={`inline-flex max-w-full items-center gap-2 rounded-md border px-3 py-1.5 text-sm font-semibold ${
        serviceAvailable
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
          : "border-destructive/30 bg-destructive/10 text-destructive"
      }`}
    >
      <span className="size-2 rounded-full bg-current" />
      <span className="truncate">
        {serviceAvailable ? "服务正常" : "维护中"}
      </span>
    </div>
  );
}

function CopyIconButton({
  label,
  copied,
  disabled,
  onClick,
}: {
  label: string;
  copied: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  const Icon = copied ? CopyCheck : Copy;
  return (
    <Button
      type="button"
      variant="outline"
      size="icon-sm"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className={
        copied
          ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-300"
          : ""
      }
    >
      <Icon className="size-4" />
    </Button>
  );
}

function RankingIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="560 30 920 980"
      className={className}
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M1012.6577777 717.01333333c-82.13333333 0-159.25333333-32.74666667-217.28-92.26666666-58.02666667-59.52-90.02666667-138.66666667-90.02666667-222.82666667V34.34666667h617.38666667v364.69333333c0 84.90666667-32.21333333 164.8-90.77333333 224.85333333s-136.42666667 93.12-219.30666667 93.12zM780.0177777 110.93333333v290.98666667c0 131.52 104.32 238.50666667 232.64 238.50666667 129.81333333 0 235.41333333-108.26666667 235.41333333-241.38666667V110.93333333H780.0177777z"
        fill="#ffcd00"
      />
      <path
        d="M976.66844437 955.85066667l1.06453333-280.85333334 74.66666667 0.28373334-1.0656 280.85333333z"
        fill="#ffcd00"
      />
      <path
        d="M810.2321777 986.5728l0.91733333-106.66666667 408.85333334 3.5136-0.91733334 106.66666667zM929.4705777 325.44746667l133.96906667-136.8032 38.10666666 37.31733333-133.96906666 136.8032zM927.7009777 495.73866667l136.52693333-140.86293334 38.29866667 37.12-136.52693333 140.86293334zM1285.2977777 445.44v-49.28c51.52 0 86.72-26.13333333 107.84-79.68 16.32-41.6 17.06666667-83.41333333 17.06666667-85.12v-38.4h-117.33333334v-49.28H1455.64444437v87.89333333c0 2.13333333-0.53333333 52.8-20.69333334 104.10666667-12.26666667 31.25333333-29.33333333 56.42666667-50.66666666 74.77333333-26.98666667 23.14666667-60.26666667 34.98666667-98.98666667 34.98666667zM743.00444437 443.84v-51.09333333c-51.30666667 0-86.4-27.09333333-107.41333334-82.77333334-16.32-43.2-16.96-86.61333333-16.96-88.32v-39.89333333h116.69333334v-51.09333333h-162.13333334V221.86666667c0 2.24 0.53333333 54.82666667 20.58666667 108.05333333 12.26666667 32.53333333 29.22666667 58.56 50.56 77.65333333 26.88 24 60.05333333 36.26666667 98.66666667 36.26666667z"
        fill="#ffcd00"
      />
    </svg>
  );
}

function PublicIPStatsModal({
  show,
  stats,
  total,
  windowKey,
  onWindowChange,
  loading,
  onClose,
}: {
  show: boolean;
  stats: IPUsageStat[];
  total: number;
  windowKey: IPStatsWindow;
  onWindowChange: (window: IPStatsWindow) => void;
  loading: boolean;
  onClose: () => void;
}) {
  const visibleStats = stats.slice(0, 20);
  return (
    <Modal
      show={show}
      title="IP 使用排行榜"
      onClose={onClose}
      contentClassName="sm:max-w-4xl"
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-3 rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-700 dark:text-emerald-300">
          <div className="flex items-center gap-2 font-semibold">
            <RankingIcon className="size-4" />
            {getIPStatsWindowLabel(windowKey)}
          </div>
          <div className="font-mono text-xs tabular-nums">
            {formatInteger(total)} IP
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          {IP_STATS_WINDOWS.map((option) => (
            <Button
              key={option.key}
              type="button"
              size="sm"
              variant={windowKey === option.key ? "default" : "outline"}
              className="h-8 px-3 text-xs"
              onClick={() => onWindowChange(option.key)}
              disabled={loading && windowKey === option.key}
            >
              {loading && windowKey === option.key ? (
                <RefreshCw className="mr-1 size-3 animate-spin" />
              ) : null}
              {option.label}
            </Button>
          ))}
        </div>

        {loading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, index) => (
              <div
                key={index}
                className="h-10 animate-pulse rounded-md bg-muted"
              />
            ))}
          </div>
        ) : visibleStats.length > 0 ? (
          <div className="overflow-x-auto rounded-lg border border-border">
            <div className="min-w-[680px] sm:min-w-full">
              <div className="grid grid-cols-[minmax(140px,1.4fr)_minmax(72px,.7fr)_minmax(72px,.7fr)_minmax(64px,.55fr)_minmax(72px,.6fr)_minmax(80px,.7fr)_minmax(96px,.85fr)] bg-muted/60 px-3 py-2 text-xs font-semibold text-muted-foreground">
                <div>IP</div>
                <div>状态</div>
                <div className="text-right">请求数</div>
                <div className="text-right">RPM</div>
                <div className="text-right">TPM</div>
                <div className="text-right">Token</div>
                <div className="text-right">费用</div>
              </div>
              <div className="max-h-[52vh] divide-y divide-border overflow-y-auto">
                {visibleStats.map((item) => (
                  <div
                    key={item.ip}
                    className="grid grid-cols-[minmax(140px,1.4fr)_minmax(72px,.7fr)_minmax(72px,.7fr)_minmax(64px,.55fr)_minmax(72px,.6fr)_minmax(80px,.7fr)_minmax(96px,.85fr)] items-center px-3 py-2 text-sm"
                  >
                    <div
                      className="truncate font-mono text-foreground"
                      title={item.ip}
                    >
                      {item.ip}
                    </div>
                    <div>
                      <IPStatusBadge status={item.status} />
                    </div>
                    <div className="text-right tabular-nums text-foreground">
                      {formatInteger(item.requests)}
                    </div>
                    <div className="text-right tabular-nums text-foreground">
                      {formatInteger(Math.round(item.rpm))}
                    </div>
                    <div className="text-right tabular-nums text-foreground">
                      {formatCompactNumber(Math.round(item.tpm))}
                    </div>
                    <div className="text-right tabular-nums text-foreground">
                      {formatCompactNumber(item.tokens)}
                    </div>
                    <div className="text-right tabular-nums text-foreground">
                      {formatMoneyFixed2(item.cost)}
                    </div>
                  </div>
                ))}
              </div>
            </div>
            {total > visibleStats.length ? (
              <div className="border-t border-border bg-muted/30 px-3 py-2 text-right text-xs text-muted-foreground">
                已显示前 {formatInteger(visibleStats.length)} 个，共{" "}
                {formatInteger(total)} 个 IP
              </div>
            ) : null}
          </div>
        ) : (
          <div className="flex min-h-32 items-center justify-center rounded-lg border border-dashed border-border bg-muted/20 text-sm text-muted-foreground">
            暂无已完成的 API 代理请求
          </div>
        )}
      </div>
    </Modal>
  );
}

function IPStatusBadge({ status }: { status?: string }) {
  const config = getIPStatusConfig(status);
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ${config.className}`}
    >
      <span className="size-1.5 rounded-full bg-current" />
      {config.label}
    </span>
  );
}

function getIPStatusConfig(status?: string): {
  label: string;
  className: string;
} {
  switch (status) {
    case "banned":
      return {
        label: "已封禁",
        className: "bg-red-500/10 text-red-600 dark:text-red-300",
      };
    case "abnormal":
      return {
        label: "异常",
        className: "bg-orange-500/10 text-orange-600 dark:text-orange-300",
      };
    case "watch":
      return {
        label: "关注",
        className: "bg-amber-500/10 text-amber-600 dark:text-amber-300",
      };
    default:
      return {
        label: "正常",
        className: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
      };
  }
}

function CodexIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 1024 1024"
      className={className}
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
    >
      <rect width="1024" height="1024" rx="190" fill="#FEFEFF" />
      <path
        d="M590.7 176.6c57.2-8.4 116.4 2.7 163.7 37.4 47.2 34.6 78.4 89.6 86.8 147.5 3.1 21.4 2.2 43.4-2 64.6 41.7 42.5 58.4 95.5 56.9 153.4l-1.9-2.4c-3.9-3.3-6.2-3-11.2-3H393.2c4.2-8.5 8.4-17.2 13.1-25.4 5.5-9.4 12.4-17.5 18.1-26.7 6.4-10.4 9.2-17.8 4.1-29.1-9.1-17.6-20.6-34.2-30.7-51.2-8.2-13.8-16.8-27.5-24-41.9-5.2-10.4-11.4-17.8-23.1-20.8-14.9-3.8-31.5 6.7-35.6 21.5-3.2 11.6 2.6 22.1 8.2 31.8 13.3 23 27.5 45.5 40.4 68.8 3.3 5.9 6.3 10.1 3.9 17-5.7 13.4-14.9 25.4-22.6 37.7-9.4 15-19.1 29.9-27.6 45.4-5.6 10.2-9.5 20-4.1 31.3 4.8 10.1 14.8 17.5 26.2 17.9 15.3.6 25-8.5 31.9-21.2 3.1-5.7 6.6-7.8 12.9-8.3 40.8-.7 81.5.2 122.2.6 3.5 8.3 5.4 17.4 12.6 23.5 8.3 7 17.5 7.5 27.8 7.4h137.6c15.4 0 24.1-7.3 31.2-20.7 2.2-4.2 3.9-8.7 5.9-13.1h166.8c-5.3 35.1-27.7 67.3-56.3 88-16.7 12.1-35.2 20.7-55.2 25.7-14.7 3.7-20.4 9-24.7 23.8-15.3 52.4-58.2 96.3-110.1 112.7-60 19-127.6 5.6-174.8-36.5-8.7-7.8-12.4-9.7-24.1-7.3-49.7 10.2-104.1-1.1-145.8-30.5-52.6-37.1-84.5-96.2-85.9-160.6-.4-17.4 1.8-34.6 6.5-51.4-44.1-43.5-63.6-101.5-52.7-162.7 11.7-65.7 58.8-122 120.8-145.9 10.7-4.1 18.1-5.7 23.5-17.1 6.1-12.8 7.5-27.1 13.6-39.9 17.1-36 47.4-65.7 83.9-81.8 57.3-25.2 127.6-16.3 181.6 15.9z"
        fill="#859FFF"
      />
      <path
        d="M186.2 705.8h642.7c-12.1 24.7-33.7 45.7-57 59.9-16.1 9.8-34.6 17.8-53.3 20.9-8.1 1.3-15.6 2-23.8 2.1H267.7c18.2 30.5 52.9 50.9 86.9 59.5 27.3 6.9 55.1 6 82.2-.9 20.6 18.5 44.6 32 71 39.6 48.4 13.9 102.4 8.6 147.4-14.2 30.6-15.5 57.1-39 73.3-69.7H267.7c-31.7-19.9-57.6-50.9-72.3-85.4-1.8-4-3.1-7.9-4.3-11.8z"
        fill="#434FFF"
      />
      <path
        d="M147.8 378h697.1c3.6 40.8-8.8 82.5-32.9 115.5-3.1 4.3-6.4 8.4-9.9 12.4h-179c-18.8 0-37.6.1-56.4.1H424.4c2.6 6.9 4 13.3 3.2 20.4-1.8 15.7-13.7 30.9-21.8 43.8-1.9 3-3.8 6.1-5.5 9.1h493.9c-1 14.2-3.5 28.4-7.7 41.8H721.8c-3.7 8-6.6 17.3-13 23.5-7.5 7.3-16.1 9-26.2 9H546.8c-9.4 0-18.2-1.1-25.8-7.4-6.4-5.3-9.1-13.3-12-20.8-39.6-.4-79.2-1.5-118.8-1-9.1.1-16 1-22.5 8.1-5.5 8.8-11.2 17-21.6 20.5-9.2 3.1-19.7 1.5-27.3-4.5-7.9-6.2-11.7-15.8-10.5-25.8 1.1-9.1 6.6-17.5 11.2-25.2 11.3-18.7 23.6-36.8 35.2-55.3 3.8-6.1 7.9-12.1 10.9-18.7 2.8-6.2-.7-11.2-3.7-16.7-13.7-24.8-29.6-48.6-42.9-73.5-4.4-8.2-8.7-15.7-7.4-25.4 1.8-13.3 13.4-24 26.8-25.5 12.8-1.5 23.4 4.8 29.5 15.8 8.8 15.7 17 31.7 26 47.2 9.9 17.2 20.9 33.7 30.1 51.3 1 2 1.8 4.1 2.4 6.3h467.1c-3.4-13.6-8.6-26.8-15.4-39.1-8.8-15.9-20.8-29.6-33.6-42.3 3.1-15.5 3.8-31.4 2.3-47.1H147.8z"
        fill="#6C89FF"
      />
      <path
        d="M346.1 381.2c13.7 4.9 19.3 16.7 25.6 28.9 2.5 4.9 5.3 9.7 8 14.4l3.6 6.4c13.6 24.2 27.8 48.1 41.2 72.4 5.8 10.5 4.2 20.4-1.2 30.6-7.8 14.6-17.2 28.5-26 42.5-8.1 12.9-15.6 26.2-23.3 39.3-6.5 11.1-13.3 25.7-27 30.3-9.3 3.1-19.5 1.5-27.2-4.8-7.3-5.9-10.8-14.6-9.5-24 1.3-9.6 8.1-19.7 13.1-27.8 13.3-21.7 27.6-42.8 40.9-64.5 2.4-3.9 4.6-7.4 2.9-12.1-11.4-23-25.9-44.4-38.7-66.6-6.2-10.7-14.9-22.5-17.3-34.7-2.9-14.9 10.3-29.9 24.8-31.3 3.6-.4 6.9 0 10.1 1zM536.9 593.7h149.8c14.7.1 25.6 7.1 29.6 21.9 3.3 12.2-1.8 25.2-12.6 31.3-6.4 3.6-13.1 3.4-20.2 3.4H547.3c-10.2-.1-17.2-2.4-25-9.1-6.5-7.5-7.4-14.9-7-24.6.3-8 5.3-13.4 11.1-18.2 3.6-3 6.4-4.1 10.5-4.7z"
        fill="#FDFEFF"
      />
    </svg>
  );
}

function ClaudeIcon({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 1024 1024"
      className={className}
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M252.8 652.8l167.893333-94.293333 2.773334-8.106667-2.773334-4.48h-8.106666l-28.16-1.706667-96-2.56-83.2-3.413333-80.64-4.266667-20.266667-4.266666L85.333333 504.746667l1.92-12.586667 17.066667-11.52 24.32 2.133333 53.973333 3.626667 81.066667 5.546667 58.666667 3.413333 87.04 9.173333h13.866666l1.92-5.546666-4.693333-3.413334-3.626667-3.413333-83.84-56.746667-90.666666-60.16-47.573334-34.56-25.813333-17.493333-13.013333-16.426667-5.546667-35.84 23.253333-25.813333 31.36 2.133333 7.893334 2.133334 31.786666 24.32 67.84 52.48L401.066667 391.466667l13.013333 10.88 5.12-3.626667 0.64-2.56-5.76-9.813333-48.213333-87.04L314.453333 210.773333l-22.826666-36.693333-5.973334-21.973333a107.861333 107.861333 0 0 1-3.626666-26.026667l26.666666-36.053333L323.413333 85.333333l35.413334 4.693334 14.933333 13.013333 21.973333 50.346667 35.626667 79.36 55.253333 107.733333 16.213334 32 8.746666 29.653333 3.2 9.173334h5.546667v-5.12l4.48-60.8 8.32-74.453334 8.106667-96 2.773333-27.093333 13.44-32.426667 26.666667-17.493333 20.693333 10.026667 17.066667 24.32-2.346667 15.786666-10.24 65.92-19.84 103.253334-13.013333 69.12h7.466666l8.746667-8.746667 34.986667-46.506667 58.666666-73.386666 26.026667-29.226667 30.293333-32.213333 19.413334-15.36h36.693333l27.093333 40.106666-12.16 41.386667-37.76 48-31.36 40.533333-45.013333 60.586667-28.16 48.426667 2.56 3.84 6.613333-0.64 101.546667-21.546667 54.826667-10.026667 65.493333-11.306666 29.653333 13.866666 3.2 14.08-11.733333 28.8-69.973333 17.28-82.133334 16.426667-122.24 29.013333-1.493333 1.066667 1.706667 2.133333 55.04 5.12 23.466666 1.28h57.6l107.306667 7.893334 28.16 18.56 16.853333 22.613333-2.773333 17.28-43.306667 21.973333-58.24-13.866666-136.106666-32.426667-46.72-11.733333h-6.4v3.84l38.826666 37.973333 71.253334 64.426667 89.173333 82.986666 4.48 20.48-11.52 16.213334-12.16-1.706667-78.506667-58.88-30.293333-26.666667-68.48-57.6h-4.48v5.973334l15.786667 23.04 83.413333 125.226666 4.266667 38.4-5.973334 12.586667-21.546666 7.466667-23.68-4.266667-48.853334-68.48-50.346666-77.226667-40.533334-69.12-4.906666 2.773334-23.893334 258.133333-11.306666 13.226667-26.026667 10.026666-21.546667-16.426666-11.52-26.666667 11.52-52.48 13.866667-68.48 11.306667-54.4 10.24-67.626667 5.973333-22.4-0.426667-1.493333-4.906666 0.64-50.986667 69.973333-77.653333 104.746667-61.44 65.706667-14.72 5.76-25.386667-13.226667 2.346667-23.466667 14.293333-20.906666 84.906667-107.946667 51.2-66.986667 33.066666-38.613333v-5.546667h-2.133333l-225.493333 146.56-40.106667 5.12-17.28-16.213333 2.133333-26.666667 8.106667-8.746666 67.84-46.72h-0.213333l0.853333 0.853333z"
        fill="#D97757"
      />
    </svg>
  );
}

interface SummaryMetric {
  label: string;
  value: string;
  sub?: string;
}

function SummaryDashboardCard({
  icon,
  title,
  description,
  metrics,
  loading,
  headerRight,
  compactMetrics = false,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  metrics: SummaryMetric[];
  loading: boolean;
  headerRight?: ReactNode;
  compactMetrics?: boolean;
}) {
  return (
    <Card className="py-0">
      <CardContent className="flex min-h-[236px] flex-col p-4">
        <div className="mb-4 flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              {icon}
            </div>
            <div className="min-w-0">
              <h2
                className="truncate text-base font-semibold text-foreground"
                title={title}
              >
                {title}
              </h2>
              <p
                className="mt-1 truncate text-sm text-muted-foreground"
                title={description}
              >
                {description}
              </p>
            </div>
          </div>
          {headerRight ? (
            <div className="flex min-h-10 shrink-0 items-center">
              {headerRight}
            </div>
          ) : null}
        </div>

        <div className="grid flex-1 grid-cols-2 gap-2">
          {metrics.map((metric) => (
            <SummaryMetricCell
              key={metric.label}
              metric={metric}
              loading={loading}
              compact={compactMetrics}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function SummaryMetricCell({
  metric,
  loading,
  compact,
}: {
  metric: SummaryMetric;
  loading: boolean;
  compact?: boolean;
}) {
  return (
    <div
      className={`min-w-0 rounded-lg border border-border/70 bg-muted/20 ${
        compact ? "p-2.5" : "p-3"
      }`}
    >
      <div
        className="truncate text-xs font-semibold text-muted-foreground"
        title={metric.label}
      >
        {metric.label}
      </div>
      {loading ? (
        <div
          className={`w-20 animate-pulse rounded-md bg-muted ${
            compact ? "mt-1.5 h-5" : "mt-2 h-6"
          }`}
        />
      ) : (
        <div
          className={`mt-1 truncate font-bold leading-none tabular-nums text-foreground ${
            compact ? "text-lg" : "text-xl"
          }`}
          title={metric.value}
        >
          {metric.value}
        </div>
      )}
      {metric.sub && (
        <div
          className={`truncate text-[11px] text-muted-foreground ${
            compact ? "mt-1" : "mt-2"
          }`}
          title={metric.sub}
        >
          {metric.sub}
        </div>
      )}
    </div>
  );
}

function AccountPoolCard({
  accountPool,
  loading,
}: {
  accountPool?: PublicAccountPoolStats;
  loading: boolean;
}) {
  const plans = accountPool?.plans ?? [];
  return (
    <Card className="py-0">
      <CardContent className="flex min-h-[236px] flex-col p-4">
        <div className="mb-4 flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <Users className="size-5" />
          </div>
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold text-foreground">
              号池状态
            </h2>
            <p className="mt-1 truncate text-sm text-muted-foreground">
              账号类型、可用和限流
            </p>
          </div>
        </div>

        {loading ? (
          <div className="grid flex-1 grid-cols-2 gap-2">
            {[0, 1, 2, 3, 4, 5].map((item) => (
              <div
                key={item}
                className="h-10 animate-pulse rounded-lg bg-muted"
              />
            ))}
          </div>
        ) : (
          <div className="flex flex-1 flex-col gap-3">
            <div className="grid grid-cols-4 gap-1.5">
              <AccountPoolMiniStat
                label="总数"
                value={formatInteger(accountPool?.total)}
              />
              <AccountPoolMiniStat
                label="可用"
                value={formatInteger(accountPool?.available)}
                tone="success"
              />
              <AccountPoolMiniStat
                label="限流"
                value={formatInteger(accountPool?.rate_limited)}
                tone={accountPool?.rate_limited ? "warning" : "normal"}
              />
              <AccountPoolMiniStat
                label="封禁"
                value={formatInteger(accountPool?.unauthorized)}
                tone={accountPool?.unauthorized ? "danger" : "normal"}
              />
            </div>
            <div className="grid grid-cols-3 gap-1.5">
              {plans.map((plan) => (
                <div
                  key={plan.type}
                  className="min-w-0 rounded-lg border border-border/70 bg-muted/20 px-2.5 py-2 text-center"
                  title={`${plan.label}: ${plan.available}/${plan.total} 可用`}
                >
                  <div className="truncate text-xs font-semibold text-muted-foreground">
                    {plan.label}
                  </div>
                  <div className="mt-1 text-base font-bold leading-none tabular-nums text-foreground">
                    {formatInteger(plan.total)}
                  </div>
                </div>
              ))}
            </div>
            <div className="grid grid-cols-2 gap-1.5">
              <AccountPoolBadge
                label="5H"
                value={accountPool?.rate_limited_5h ?? 0}
                tone="warning"
              />
              <AccountPoolBadge
                label="7D"
                value={accountPool?.rate_limited_7d ?? 0}
                tone="warning"
              />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function AccountPoolMiniStat({
  label,
  value,
  tone = "normal",
}: {
  label: string;
  value: string;
  tone?: "normal" | "success" | "warning" | "danger";
}) {
  const toneClass = {
    normal: "text-foreground",
    success: "text-emerald-700 dark:text-emerald-300",
    warning: "text-amber-700 dark:text-amber-300",
    danger: "text-destructive",
  }[tone];
  return (
    <div className="min-w-0 rounded-lg border border-border/70 bg-muted/20 px-2 py-2">
      <div className="truncate text-[11px] font-semibold text-muted-foreground">
        {label}
      </div>
      <div
        className={`mt-1 truncate text-lg font-bold leading-none tabular-nums ${toneClass}`}
      >
        {value}
      </div>
    </div>
  );
}

function AccountPoolBadge({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "warning" | "danger" | "muted";
}) {
  const toneClass = {
    warning:
      value > 0
        ? "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
        : "border-border bg-muted/20 text-muted-foreground",
    danger:
      value > 0
        ? "border-destructive/30 bg-destructive/10 text-destructive"
        : "border-border bg-muted/20 text-muted-foreground",
    muted:
      value > 0
        ? "border-slate-500/30 bg-slate-500/10 text-slate-700 dark:text-slate-300"
        : "border-border bg-muted/20 text-muted-foreground",
  }[tone];
  return (
    <div
      className={`min-w-0 rounded-md border px-2 py-1 text-center text-[11px] font-semibold ${toneClass}`}
    >
      <span className="truncate">{label}</span>
      <span className="ml-1 font-mono tabular-nums">
        {formatInteger(value)}
      </span>
    </div>
  );
}

function MaintenanceStatusCard({
  maintenance,
  loading,
  onOpenDocs,
  codexCcSwitchUrl,
  claudeCcSwitchUrl,
  ccSwitchDisabled,
  onCcSwitchImport,
}: {
  maintenance: PublicHomeResponse["maintenance"] | undefined;
  loading: boolean;
  onOpenDocs: (tab: PublicDocsTopTab) => void;
  codexCcSwitchUrl: string;
  claudeCcSwitchUrl: string;
  ccSwitchDisabled: boolean;
  onCcSwitchImport: () => void;
}) {
  const routes = maintenance?.routes ?? [];
  const enabled = Boolean(maintenance?.enabled);
  const maintenanceRoutes = sortPublicMaintenanceRoutes(
    routes.filter((route) => route.maintenance),
  );
  const availableRoutes = sortPublicMaintenanceRoutes(
    routes.filter((route) => !route.maintenance),
  );
  const endpointRoutes = mergePublicMaintenanceRoutes([
    ...availableRoutes,
    ...maintenanceRoutes,
  ]);
  const allMaintenance =
    enabled && routes.length > 0 && availableRoutes.length === 0;
  const partialMaintenance =
    enabled && maintenanceRoutes.length > 0 && availableRoutes.length > 0;
  const statusText = allMaintenance
    ? maintenance?.message || "系统维护中，请稍后重试。"
    : partialMaintenance
      ? "服务正常，部分端点维护中"
      : "所有端点正常可用";
  const tone = allMaintenance ? "danger" : "normal";
  const statusLabel = allMaintenance ? "关闭" : "正常";
  const toneClass = {
    danger: "border-destructive/30 bg-destructive/10 text-destructive",
    normal:
      "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  }[tone];
  const iconClass = {
    danger: "bg-destructive/10 text-destructive",
    normal: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300",
  }[tone];

  return (
    <Card className="py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="grid gap-5 lg:grid-cols-[minmax(240px,360px)_minmax(0,1fr)] lg:items-start">
          <div className="flex min-w-0 items-start gap-3">
            <div
              className={`flex size-11 shrink-0 items-center justify-center rounded-lg ${iconClass}`}
            >
              <Wrench className="size-5" />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-lg font-semibold text-foreground">
                  服务状态
                </h2>
                <span
                  className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-semibold ${toneClass}`}
                >
                  <span className="size-1.5 rounded-full bg-current" />
                  {statusLabel}
                </span>
              </div>
              <p
                className={`mt-1 text-sm ${allMaintenance ? "font-semibold text-destructive" : "text-muted-foreground"}`}
              >
                {statusText}
              </p>
            </div>
          </div>

          <div className="flex min-w-0 flex-col gap-3">
            <div className="flex flex-wrap gap-2 lg:justify-end">
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1.5"
                onClick={() => onOpenDocs("codex")}
              >
                <CodexIcon className="size-4" />
                Codex 配置教程
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="gap-1.5"
                onClick={() => onOpenDocs("claude")}
              >
                <ClaudeIcon className="size-4" />
                Claude 配置教程
              </Button>
              <PublicDocsCcSwitchButton
                href={codexCcSwitchUrl}
                disabled={ccSwitchDisabled}
                label="导入 CC Switch"
                icon={<CodexIcon className="size-4" />}
                onImportClick={onCcSwitchImport}
              />
              <PublicDocsCcSwitchButton
                href={claudeCcSwitchUrl}
                disabled={ccSwitchDisabled}
                label="导入 CC Switch"
                icon={<ClaudeIcon className="size-4" />}
                onImportClick={onCcSwitchImport}
              />
            </div>
            {allMaintenance && !loading ? (
              <div className="flex min-h-20 items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-destructive">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <div className="min-w-0">
                  <div className="text-sm font-semibold">全部端点维护中</div>
                  <div className="mt-1 text-sm leading-6">{statusText}</div>
                </div>
              </div>
            ) : loading ? (
              <div className="flex flex-wrap gap-2">
                {[0, 1, 2].map((i) => (
                  <div
                    key={i}
                    className="h-7 w-36 animate-pulse rounded-md bg-muted"
                  />
                ))}
              </div>
            ) : routes.length > 0 ? (
              <div className="flex min-w-0 flex-wrap items-center gap-2 lg:justify-end">
                {endpointRoutes.map((route) => (
                  <MaintenanceRouteChip key={route.path} route={route} />
                ))}
              </div>
            ) : (
              <div className="text-sm text-muted-foreground">
                所有 API 端点正常可用
              </div>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

type PublicDocsTopTab = "codex" | "claude";
type PublicDocsCodexPlatform = "unix" | "windows";
type PublicDocsClaudePlatform = "unix" | "cmd" | "powershell";
type PublicDocsTabOption<T extends string> = {
  value: T;
  label: string;
  icon: ReactNode;
};

function PublicUsageDocsModal({
  show,
  initialTab,
  baseUrl,
  apiKey,
  copiedTarget,
  onCopy,
  onClose,
}: {
  show: boolean;
  initialTab: PublicDocsTopTab;
  baseUrl: string;
  apiKey: string;
  copiedTarget: string;
  onCopy: (text: string, target: string) => void;
  onClose: () => void;
}) {
  const [codexPlatform, setCodexPlatform] =
    useState<PublicDocsCodexPlatform>("unix");
  const [claudePlatform, setClaudePlatform] =
    useState<PublicDocsClaudePlatform>("unix");
  const key = apiKey || "YOUR_API_KEY";
  const isCodex = initialTab === "codex";

  const codexConfigPath =
    codexPlatform === "windows"
      ? "%userprofile%\\.codex\\config.toml"
      : "~/.codex/config.toml";
  const codexAuthPath =
    codexPlatform === "windows"
      ? "%userprofile%\\.codex\\auth.json"
      : "~/.codex/auth.json";
  const claudeSettingsPath =
    claudePlatform === "unix"
      ? "~/.claude/settings.json"
      : "%userprofile%\\.claude\\settings.json";
  const claudeEnvLabel =
    claudePlatform === "cmd"
      ? "Windows CMD 环境变量"
      : claudePlatform === "powershell"
        ? "PowerShell 环境变量"
        : "Terminal 环境变量";
  const codexConfig = `model_provider = "OpenAI"
model = "gpt-5.5"
model_reasoning_effort = "high"
plan_mode_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
show_raw_agent_reasoning = true
approval_policy = "never"
sandbox_mode = "danger-full-access"
personality = "friendly"
web_search = "live"

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
requires_openai_auth = true
supports_websockets = false`;

  const codexAuth = `{
  "OPENAI_API_KEY": "${key}"
}`;

  const claudeEnv =
    claudePlatform === "cmd"
      ? `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${key}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`
      : claudePlatform === "powershell"
        ? `$env:ANTHROPIC_BASE_URL="${baseUrl}"
$env:ANTHROPIC_AUTH_TOKEN="${key}"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"`
        : `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="${key}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`;

  const claudeSettings = `{
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "${key}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`;
  return (
    <Modal
      show={show}
      title={isCodex ? "Codex 配置教程" : "Claude 配置教程"}
      onClose={onClose}
      contentClassName="sm:max-w-[860px]"
      bodyClassName="space-y-5"
    >
      {isCodex ? (
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground">Codex CLI</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              写入本机 Codex 配置目录，使用 Responses 接口。
            </p>
          </div>
          <PublicDocsSegmentedTabs
            value={codexPlatform}
            onChange={setCodexPlatform}
            options={[
              {
                value: "unix",
                label: "macOS / Linux",
                icon: <Laptop className="size-3.5" />,
              },
              {
                value: "windows",
                label: "Windows",
                icon: <Monitor className="size-3.5" />,
              },
            ]}
          />
          <PublicDocsCodeBlock
            label={codexConfigPath}
            content={codexConfig}
            copied={copiedTarget === "docs_codex_config"}
            onCopy={() => onCopy(codexConfig, "docs_codex_config")}
          />
          <PublicDocsCodeBlock
            label={codexAuthPath}
            content={codexAuth}
            copied={copiedTarget === "docs_codex_auth"}
            onCopy={() => onCopy(codexAuth, "docs_codex_auth")}
          />
          <PublicDocsHint>
            {codexPlatform === "windows" ? (
              <div>
                按 <code className="font-mono text-foreground">Win+R</code>
                ，输入{" "}
                <code className="font-mono text-foreground">
                  %userprofile%\.codex
                </code>{" "}
                打开配置目录。如目录不存在，请先手动创建。
              </div>
            ) : (
              <div>
                请确保配置目录存在。macOS/Linux 用户可运行{" "}
                <code className="font-mono text-foreground">
                  mkdir -p ~/.codex
                </code>{" "}
                创建目录。
              </div>
            )}
          </PublicDocsHint>
        </section>
      ) : (
        <section className="space-y-3">
          <div>
            <h3 className="text-sm font-semibold text-foreground">
              Claude Code
            </h3>
            <p className="mt-1 text-sm text-muted-foreground">
              可使用环境变量，或写入 Claude settings.json。
            </p>
          </div>
          <PublicDocsSegmentedTabs
            value={claudePlatform}
            onChange={setClaudePlatform}
            options={[
              {
                value: "unix",
                label: "macOS / Linux",
                icon: <Laptop className="size-3.5" />,
              },
              {
                value: "cmd",
                label: "Windows CMD",
                icon: <Terminal className="size-3.5" />,
              },
              {
                value: "powershell",
                label: "PowerShell",
                icon: <SquareTerminal className="size-3.5" />,
              },
            ]}
          />
          <PublicDocsCodeBlock
            label={claudeEnvLabel}
            content={claudeEnv}
            copied={copiedTarget === "docs_claude_env"}
            onCopy={() => onCopy(claudeEnv, "docs_claude_env")}
          />
          <PublicDocsCodeBlock
            label={claudeSettingsPath}
            content={claudeSettings}
            copied={copiedTarget === "docs_claude_settings"}
            onCopy={() => onCopy(claudeSettings, "docs_claude_settings")}
          />
          <PublicDocsHint>
            这些环境变量将在当前终端会话中生效。如需永久配置，请将其添加到{" "}
            <code className="font-mono text-foreground">~/.bashrc</code>、
            <code className="font-mono text-foreground">~/.zshrc</code>{" "}
            或相应的配置文件中。
          </PublicDocsHint>
        </section>
      )}
    </Modal>
  );
}

function PublicDocsSegmentedTabs<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (value: T) => void;
  options: PublicDocsTabOption<T>[];
}) {
  return (
    <div className="inline-flex w-fit max-w-full flex-wrap rounded-lg border border-border bg-muted/50 p-0.5">
      {options.map((option) => (
        <button
          key={option.value}
          type="button"
          onClick={() => onChange(option.value)}
          className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-semibold transition-colors ${
            value === option.value
              ? "bg-background text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground"
          }`}
        >
          {option.icon}
          {option.label}
        </button>
      ))}
    </div>
  );
}

function PublicDocsCcSwitchButton({
  href,
  disabled,
  label,
  icon,
  onImportClick,
}: {
  href: string;
  disabled: boolean;
  label: string;
  icon: ReactNode;
  onImportClick: () => void;
}) {
  if (disabled) {
    return (
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-fit gap-2"
        disabled
      >
        {icon}
        {label}
      </Button>
    );
  }

  return (
    <Button
      asChild
      type="button"
      variant="outline"
      size="sm"
      className="w-fit gap-2"
    >
      <a href={href} onClick={onImportClick}>
        {icon}
        {label}
        <ExternalLink className="size-3.5" />
      </a>
    </Button>
  );
}

function PublicDocsHint({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2 text-xs leading-6 text-muted-foreground">
      <Info className="mt-1 size-3.5 shrink-0 text-primary" />
      <div className="min-w-0 space-y-1">{children}</div>
    </div>
  );
}

function buildCcSwitchImportUrl({
  app,
  baseUrl,
  apiKey,
  name,
  model,
}: {
  app: "codex" | "claude";
  baseUrl: string;
  apiKey: string;
  name: string;
  model?: string;
}) {
  const params = new URLSearchParams({
    resource: "provider",
    app,
    name: name || "Codex2API",
    homepage: baseUrl,
    endpoint: baseUrl,
    apiKey,
    configFormat: "json",
  });

  if (model) {
    params.set("model", model);
  }

  return `ccswitch://v1/import?${params.toString()}`;
}

function PublicDocsCodeBlock({
  label,
  content,
  copied,
  onCopy,
}: {
  label: string;
  content: string;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-zinc-950">
      <div className="flex items-center justify-between gap-3 border-b border-zinc-800 bg-zinc-900 px-3 py-2">
        <span className="truncate font-mono text-xs text-zinc-400">
          {label}
        </span>
        <button
          type="button"
          onClick={onCopy}
          className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-semibold transition-colors ${
            copied
              ? "bg-emerald-500/20 text-emerald-300"
              : "bg-zinc-800 text-zinc-300 hover:bg-zinc-700 hover:text-white"
          }`}
        >
          {copied ? (
            <CopyCheck className="size-3.5" />
          ) : (
            <Copy className="size-3.5" />
          )}
          {copied ? "已复制" : "复制"}
        </button>
      </div>
      <pre className="max-h-64 overflow-auto p-3 font-mono text-xs leading-6 text-zinc-100">
        <code>{content}</code>
      </pre>
    </div>
  );
}

function MaintenanceRouteChip({ route }: { route: PublicMaintenanceRoute }) {
  const meta = getPublicMaintenanceRouteMeta(route.path);
  const tone = route.maintenance ? "maintenance" : "available";
  const className =
    tone === "available"
      ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
      : "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300";
  const Icon = tone === "available" ? CheckCircle2 : Wrench;
  return (
    <span
      className={`group relative inline-flex max-w-full items-center gap-2 rounded-md border px-3 py-1.5 text-sm outline-none ring-offset-background transition-shadow focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${className}`}
      aria-label={`${meta.label} ${route.path}`}
      tabIndex={0}
    >
      <Icon className="size-3.5 shrink-0" />
      <span className="shrink-0 font-semibold">{meta.label}</span>
      <span className="pointer-events-none absolute bottom-[calc(100%+0.5rem)] left-1/2 z-40 max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-md border border-border bg-popover px-2.5 py-1.5 font-mono text-xs leading-5 text-popover-foreground opacity-0 shadow-lg transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
        {route.path}
      </span>
    </span>
  );
}

function ModelStatsCard({
  stats,
  loading,
  ipStatsTotal,
  onOpenIPStats,
}: {
  stats: UsageModelStat[];
  loading: boolean;
  ipStatsTotal: number;
  onOpenIPStats: () => void;
}) {
  const maxRequests = Math.max(1, ...stats.map((item) => item.requests));
  return (
    <Card className="h-[420px] py-0">
      <CardContent className="flex h-full min-h-0 flex-col p-4 sm:p-5">
        <div className="mb-4 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-foreground">
              模型统计
            </h2>
            <p className="mt-1 text-sm text-muted-foreground">
              按请求量和计费聚合
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 shrink-0 px-2 text-xs"
            onClick={onOpenIPStats}
          >
            <RankingIcon className="size-3.5" />
            IP 排行榜
            <span className="font-mono tabular-nums">
              {formatInteger(ipStatsTotal)}
            </span>
          </Button>
        </div>
        {loading ? (
          <div className="min-h-0 flex-1 space-y-3 overflow-hidden">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="h-12 animate-pulse rounded-lg bg-muted" />
            ))}
          </div>
        ) : stats.length > 0 ? (
          <div className="min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
            {stats.map((item) => (
              <div
                key={item.model}
                className="rounded-lg border border-border/70 bg-muted/20 p-3"
              >
                <div className="mb-2 flex min-w-0 items-center justify-between gap-3">
                  <span
                    className="truncate text-sm font-semibold text-foreground"
                    title={item.model}
                  >
                    {item.model}
                  </span>
                  <span className="shrink-0 text-xs font-semibold tabular-nums text-muted-foreground">
                    {formatMoney(item.user_billed)}
                  </span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{
                      width: `${Math.max(4, (item.requests / maxRequests) * 100)}%`,
                    }}
                  />
                </div>
                <div className="mt-2 flex items-center justify-between text-xs text-muted-foreground">
                  <span>{formatInteger(item.requests)} requests</span>
                  <span>{formatCompactNumber(item.tokens)} tokens</span>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 items-center justify-center rounded-lg border border-dashed border-border bg-muted/20 text-sm text-muted-foreground">
            暂无模型统计
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ChartSkeleton() {
  return (
    <div className="flex h-full items-end gap-2 px-2 pb-6">
      {[42, 64, 38, 72, 55, 80, 46, 62, 50, 76, 58, 68].map((height, index) => (
        <div
          key={index}
          className="flex-1 animate-pulse rounded-t-md bg-muted"
          style={{ height: `${height}%`, animationDelay: `${index * 70}ms` }}
        />
      ))}
    </div>
  );
}

function hasPublicAvailableRoutes(
  maintenance: PublicHomeResponse["maintenance"] | undefined,
): boolean {
  if (!maintenance?.enabled) return true;
  const routes = maintenance.routes ?? [];
  if (routes.length === 0) return false;
  return routes.some((route) => !route.maintenance);
}

function sortPublicMaintenanceRoutes(
  routes: PublicMaintenanceRoute[],
): PublicMaintenanceRoute[] {
  return [...routes].sort((a, b) => {
    const aMeta = getPublicMaintenanceRouteMeta(a.path);
    const bMeta = getPublicMaintenanceRouteMeta(b.path);
    if (aMeta.order !== bMeta.order) return aMeta.order - bMeta.order;
    return a.path.localeCompare(b.path);
  });
}

function mergePublicMaintenanceRoutes(
  routes: PublicMaintenanceRoute[],
): PublicMaintenanceRoute[] {
  const merged = new Map<string, PublicMaintenanceRoute>();
  for (const route of routes) {
    const meta = getPublicMaintenanceRouteMeta(route.path);
    const key =
      meta.label === "Codex" || meta.label === "GPT生图"
        ? meta.label
        : route.path;
    const current = merged.get(key);
    if (!current) {
      merged.set(key, route);
      continue;
    }
    merged.set(key, {
      ...current,
      maintenance: current.maintenance || route.maintenance,
      path:
        current.path === route.path
          ? current.path
          : `${current.path}, ${route.path}`,
    });
  }
  return [...merged.values()];
}

function getPublicMaintenanceRouteMeta(path: string): {
  label: string;
  order: number;
} {
  if (path.includes("/v1/responses")) {
    return { label: "Codex", order: 50 };
  }
  if (
    path.includes("/v1/images/edits") ||
    path.includes("/v1/images/generations")
  ) {
    return { label: "GPT生图", order: 20 };
  }
  return PUBLIC_MAINTENANCE_ROUTE_META[path] ?? { label: "API", order: 999 };
}

function formatChartLabel(date: Date, range: TimeRangeKey): string {
  if (range === "7d") {
    return `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, "0")}:00`;
  }
  return `${String(date.getHours()).padStart(2, "0")}:${String(date.getMinutes()).padStart(2, "0")}`;
}

function formatInteger(value?: number | null): string {
  return Math.round(value ?? 0).toLocaleString();
}

function getIPStatsWindowLabel(windowKey: IPStatsWindow): string {
  switch (windowKey) {
    case "1m":
      return "最近 1 分钟";
    case "15m":
      return "最近 15 分钟";
    case "1h":
      return "最近 1 小时";
    case "today":
      return "今日";
    default:
      return "最近 5 分钟";
  }
}

function formatCompactNumber(value?: number | null): string {
  const amount = Math.round(value ?? 0);
  const abs = Math.abs(amount);
  const units = [
    { value: 1_000_000_000, suffix: "B" },
    { value: 1_000_000, suffix: "M" },
    { value: 1_000, suffix: "K" },
  ];
  const unit = units.find((item) => abs >= item.value);
  if (!unit) return amount.toLocaleString();
  const compact = amount / unit.value;
  const digits = Math.abs(compact) >= 100 ? 0 : 1;
  return `${compact.toFixed(digits).replace(/\.0$/, "")}${unit.suffix}`;
}

function formatDecimal(value?: number | null, digits = 2): string {
  return (value ?? 0).toFixed(digits);
}

function formatPercent(value?: number | null, digits = 2): string {
  return `${formatDecimal(value, digits)}%`;
}

function formatLatency(value?: number | null): string {
  const ms = value ?? 0;
  if (ms <= 0) return "-";
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms)}ms`;
}

function formatMoney(value?: number | null): string {
  return formatMoneyFixed2(value);
}

function formatMoneyFixed2(value?: number | null): string {
  return `$${(value ?? 0).toFixed(2)}`;
}

function formatBytes(bytes?: number | null): string {
  const value = bytes ?? 0;
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let size = value / 1024;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[index]}`;
}

function formatBytesCompact(bytes?: number | null): string {
  const value = bytes ?? 0;
  if (value < 1024) return `${value}B`;
  const units = ["K", "M", "G", "T"];
  let size = value / 1024;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  const digits = size >= 100 ? 0 : size >= 10 ? 1 : 2;
  return `${size.toFixed(digits).replace(/\.0+$/, "")}${units[index]}`;
}

function round(value: number, digits: number): number {
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

function formatChartAxisValue(value: number): string {
  const abs = Math.abs(value);
  if (abs === 0) return "0";
  if (abs < 0.01) return "<0.01";
  if (abs < 1) return value.toFixed(2);
  if (abs < 1000) return Math.round(value).toLocaleString();
  return new Intl.NumberFormat(undefined, {
    notation: "compact",
    maximumFractionDigits: 0,
  }).format(value);
}

function formatChartTooltipValue(value: unknown, name: unknown): string {
  if (typeof value !== "number") return String(value);
  return Math.round(value).toLocaleString();
}
