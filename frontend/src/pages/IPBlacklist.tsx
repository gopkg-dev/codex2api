import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Ban,
  Clock,
  ListChecks,
  RefreshCw,
  Trash2,
  Trophy,
} from "lucide-react";
import PageHeader from "../components/PageHeader";
import Modal from "../components/Modal";
import Pagination from "../components/Pagination";
import ToastNotice from "../components/ToastNotice";
import { useToast } from "../hooks/useToast";
import { api } from "../api";
import type {
  IPBan,
  IPStatsSort,
  IPStatsWindow,
  IPUsageStat,
  SortOrder,
} from "../types";
import { getErrorMessage } from "../utils/error";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type IPBlacklistTab = "ranking" | "bans";

const IP_STATS_WINDOWS: Array<{ key: IPStatsWindow; label: string }> = [
  { key: "1m", label: "1分钟" },
  { key: "5m", label: "5分钟" },
  { key: "15m", label: "15分钟" },
  { key: "1h", label: "1小时" },
  { key: "today", label: "今日" },
];

function IPBanCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
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
  children: ReactNode;
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

function TabButton({
  active,
  icon,
  label,
  onClick,
}: {
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold transition ${
        active
          ? "bg-background text-foreground shadow-sm"
          : "text-muted-foreground hover:text-foreground"
      }`}
    >
      {icon}
      {label}
    </button>
  );
}

function IPUsageRankingCard({
  stats,
  total,
  page,
  pageSize,
  windowKey,
  sort,
  order,
  loading,
  onWindowChange,
  onSortChange,
  onPageChange,
  onPageSizeChange,
}: {
  stats: IPUsageStat[];
  total: number;
  page: number;
  pageSize: number;
  windowKey: IPStatsWindow;
  sort: IPStatsSort;
  order: SortOrder;
  loading: boolean;
  onWindowChange: (windowKey: IPStatsWindow) => void;
  onSortChange: (sort: IPStatsSort) => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
}) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, totalPages);
  const columns: Array<{
    key: IPStatsSort;
    label: string;
    align?: "right";
  }> = [
    { key: "ip", label: "IP" },
    { key: "status", label: "状态" },
    { key: "requests", label: "请求数", align: "right" },
    { key: "rpm", label: "RPM", align: "right" },
    { key: "tpm", label: "TPM", align: "right" },
    { key: "tokens", label: "Token", align: "right" },
    { key: "cost", label: "费用", align: "right" },
  ];
  return (
    <IPBanCard
      title="IP 使用排行榜"
      description={`${getIPStatsWindowLabel(windowKey)}实时状态，支持分页和字段排序。`}
    >
      <div className="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="inline-flex w-fit items-center gap-2 rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm font-semibold text-emerald-700 dark:text-emerald-300">
          <Trophy className="size-4 text-yellow-500" />
          {getIPStatsWindowLabel(windowKey)}
          <span className="ml-2 rounded-md bg-background/70 px-2 py-0.5 font-mono text-xs tabular-nums">
            {formatInteger(total)} IP
          </span>
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
      </div>

      {loading ? (
        <div className="space-y-2">
          {Array.from({ length: 8 }).map((_, index) => (
            <div
              key={index}
              className="h-10 animate-pulse rounded-md bg-muted"
            />
          ))}
        </div>
      ) : stats.length > 0 ? (
        <>
          <div className="overflow-hidden rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  {columns.map((column) => (
                    <TableHead
                      key={column.key}
                      className={column.align === "right" ? "text-right" : ""}
                    >
                      <button
                        type="button"
                        onClick={() => onSortChange(column.key)}
                        className={`inline-flex items-center gap-1 rounded px-1 py-0.5 text-xs font-semibold transition hover:text-foreground ${
                          column.align === "right" ? "ml-auto" : ""
                        }`}
                      >
                        {column.label}
                        <SortIcon active={sort === column.key} order={order} />
                      </button>
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {stats.map((item) => (
                  <TableRow key={item.ip}>
                    <TableCell
                      className="max-w-[220px] truncate font-mono text-sm"
                      title={item.ip}
                    >
                      {item.ip}
                    </TableCell>
                    <TableCell>
                      <IPStatusBadge status={item.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatInteger(item.requests)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatInteger(Math.round(item.rpm))}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatCompactNumber(Math.round(item.tpm))}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatCompactNumber(item.tokens)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatMoneyFixed2(item.cost)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <Pagination
            page={currentPage}
            totalPages={totalPages}
            totalItems={total}
            pageSize={pageSize}
            onPageChange={onPageChange}
            onPageSizeChange={onPageSizeChange}
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </>
      ) : (
        <div className="flex min-h-[120px] items-center justify-center rounded-lg border border-dashed border-border bg-muted/20 text-sm text-muted-foreground">
          {getIPStatsWindowLabel(windowKey)}暂无已完成的 API 代理请求
        </div>
      )}
    </IPBanCard>
  );
}

function SortIcon({ active, order }: { active: boolean; order: SortOrder }) {
  if (!active) return <ArrowUpDown className="size-3.5 opacity-45" />;
  return order === "asc" ? (
    <ArrowUp className="size-3.5" />
  ) : (
    <ArrowDown className="size-3.5" />
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

function formatDate(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
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

function formatInteger(value: number): string {
  if (!Number.isFinite(value)) return "0";
  return Math.round(value).toLocaleString();
}

function formatCompactNumber(value: number): string {
  if (!Number.isFinite(value)) return "0";
  const abs = Math.abs(value);
  const sign = value < 0 ? "-" : "";
  if (abs >= 1_000_000_000) {
    return `${sign}${trimFixed(abs / 1_000_000_000, 1)}B`;
  }
  if (abs >= 1_000_000) {
    return `${sign}${trimFixed(abs / 1_000_000, 1)}M`;
  }
  if (abs >= 1_000) {
    return `${sign}${trimFixed(abs / 1_000, 1)}K`;
  }
  return `${Math.round(value)}`;
}

function trimFixed(value: number, digits: number): string {
  return value.toFixed(digits).replace(/\.0$/, "");
}

function formatMoneyFixed2(value: number): string {
  if (!Number.isFinite(value)) return "$0.00";
  return `$${value.toFixed(2)}`;
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

function isBanActive(ban: IPBan, now: number) {
  if (!ban.enabled || ban.unbanned_at) return false;
  return !isPastDate(ban.expires_at, now);
}

function banStatus(ban: IPBan, now: number) {
  if (!isBanActive(ban, now)) {
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

export default function IPBlacklist() {
  const { toast, showToast } = useToast();
  const [activeTab, setActiveTab] = useState<IPBlacklistTab>("ranking");
  const [ipStats, setIPStats] = useState<IPUsageStat[]>([]);
  const [ipStatsTotal, setIPStatsTotal] = useState(0);
  const [ipStatsPage, setIPStatsPage] = useState(1);
  const [ipStatsPageSize, setIPStatsPageSize] = useState(20);
  const [ipStatsWindow, setIPStatsWindow] = useState<IPStatsWindow>("5m");
  const [ipStatsSort, setIPStatsSort] = useState<IPStatsSort>("requests");
  const [ipStatsOrder, setIPStatsOrder] = useState<SortOrder>("desc");
  const [ipStatsLoading, setIPStatsLoading] = useState(true);
  const ipStatsLoadSeq = useRef(0);
  const [bans, setBans] = useState<IPBan[]>([]);
  const [bansTotal, setBansTotal] = useState(0);
  const [banPage, setBanPage] = useState(1);
  const [banPageSize, setBanPageSize] = useState(20);
  const [bansLoading, setBansLoading] = useState(true);
  const [newIPs, setNewIPs] = useState("");
  const [newExpiresIn, setNewExpiresIn] = useState(0);
  const [batchBanOpen, setBatchBanOpen] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const pendingIPs = useMemo(() => parseIPBanInput(newIPs), [newIPs]);

  const loadIPStats = useCallback(
    async (options?: {
      windowKey?: IPStatsWindow;
      page?: number;
      pageSize?: number;
      sort?: IPStatsSort;
      order?: SortOrder;
    }) => {
      const seq = ipStatsLoadSeq.current + 1;
      ipStatsLoadSeq.current = seq;
      setIPStatsLoading(true);
      try {
        const response = await api.getIPUsageStats({
          window: options?.windowKey ?? ipStatsWindow,
          page: options?.page ?? ipStatsPage,
          pageSize: options?.pageSize ?? ipStatsPageSize,
          sort: options?.sort ?? ipStatsSort,
          order: options?.order ?? ipStatsOrder,
        });
        if (seq !== ipStatsLoadSeq.current) return;
        setIPStats(response.stats ?? []);
        setIPStatsTotal(response.total ?? 0);
      } catch (error) {
        if (seq === ipStatsLoadSeq.current) {
          showToast(`加载排行榜失败：${getErrorMessage(error)}`, "error");
        }
      } finally {
        if (seq === ipStatsLoadSeq.current) {
          setIPStatsLoading(false);
        }
      }
    },
    [
      ipStatsOrder,
      ipStatsPage,
      ipStatsPageSize,
      ipStatsSort,
      ipStatsWindow,
      showToast,
    ],
  );

  const reload = useCallback(
    async (options?: { page?: number; pageSize?: number }) => {
      setBansLoading(true);
      try {
        const nextPage = options?.page ?? banPage;
        const nextPageSize = options?.pageSize ?? banPageSize;
        const banRes = await api.getIPBans(true, {
          page: nextPage,
          pageSize: nextPageSize,
        });
        setBans(banRes.bans);
        setBansTotal(banRes.total);
      } catch (error) {
        showToast(`加载失败：${getErrorMessage(error)}`, "error");
      } finally {
        setBansLoading(false);
      }
    },
    [banPage, banPageSize, showToast],
  );

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    void loadIPStats();
  }, [loadIPStats]);

  useEffect(() => {
    if (activeTab !== "ranking") return;
    const timer = window.setInterval(() => {
      void loadIPStats();
    }, 15000);
    return () => window.clearInterval(timer);
  }, [activeTab, loadIPStats]);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

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
  const ipStatsTotalPages = Math.max(
    1,
    Math.ceil(ipStatsTotal / ipStatsPageSize),
  );

  const changeIPStatsWindow = (windowKey: IPStatsWindow) => {
    setIPStatsWindow(windowKey);
    setIPStatsPage(1);
  };

  const changeIPStatsSort = (sort: IPStatsSort) => {
    setIPStatsPage(1);
    if (sort === ipStatsSort) {
      setIPStatsOrder((current) => (current === "desc" ? "asc" : "desc"));
      return;
    }
    setIPStatsSort(sort);
    setIPStatsOrder(sort === "ip" ? "asc" : "desc");
  };

  useEffect(() => {
    if (banPage > banTotalPages) {
      setBanPage(banTotalPages);
    }
  }, [banPage, banTotalPages]);

  useEffect(() => {
    if (ipStatsPage > ipStatsTotalPages) {
      setIPStatsPage(ipStatsTotalPages);
    }
  }, [ipStatsPage, ipStatsTotalPages]);

  return (
    <div>
      <ToastNotice toast={toast} />
      <PageHeader
        title="IP 黑名单"
        description="查看 IP 使用排行榜，管理手动封禁和自动封禁记录。"
        onRefresh={() => {
          if (activeTab === "ranking") {
            void loadIPStats();
          } else {
            void reload();
          }
        }}
        actions={
          activeTab === "bans" ? (
            <Button onClick={() => setBatchBanOpen(true)}>
              <Ban className="size-4" />
              批量添加 IP
            </Button>
          ) : undefined
        }
      />

      <div className="mb-4 inline-flex rounded-lg border border-border bg-muted/40 p-1">
        <TabButton
          active={activeTab === "ranking"}
          icon={<Trophy className="size-4" />}
          label="使用排行榜"
          onClick={() => setActiveTab("ranking")}
        />
        <TabButton
          active={activeTab === "bans"}
          icon={<ListChecks className="size-4" />}
          label="黑名单管理"
          onClick={() => setActiveTab("bans")}
        />
      </div>

      {activeTab === "ranking" ? (
        <IPUsageRankingCard
          stats={ipStats}
          total={ipStatsTotal}
          page={Math.min(ipStatsPage, ipStatsTotalPages)}
          pageSize={ipStatsPageSize}
          windowKey={ipStatsWindow}
          sort={ipStatsSort}
          order={ipStatsOrder}
          loading={ipStatsLoading}
          onWindowChange={changeIPStatsWindow}
          onSortChange={changeIPStatsSort}
          onPageChange={setIPStatsPage}
          onPageSizeChange={(next) => {
            setIPStatsPage(1);
            setIPStatsPageSize(next);
          }}
        />
      ) : (
        <IPBanCard
          title="封禁列表"
          description="支持分页查询、手动解封和删除封禁记录。"
        >
          <div className="mb-4 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>当前共 {bansTotal} 条封禁记录</span>
            {bansLoading ? (
              <span className="inline-flex items-center gap-1">
                <RefreshCw className="size-3 animate-spin" />
                正在加载
              </span>
            ) : null}
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
                {bansLoading ? (
                  Array.from({ length: 6 }).map((_, index) => (
                    <TableRow key={index}>
                      <TableCell colSpan={8}>
                        <div className="h-8 animate-pulse rounded bg-muted" />
                      </TableCell>
                    </TableRow>
                  ))
                ) : bans.length === 0 ? (
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
                        <TableCell
                          title={formatDate(
                            ban.last_triggered_at || ban.banned_at,
                          )}
                        >
                          {formatDate(ban.last_triggered_at || ban.banned_at)}
                        </TableCell>
                        <TableCell
                          title={formatDate(ban.unbanned_at || ban.expires_at)}
                        >
                          {banReleaseTimeLabel(ban, now)}
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="inline-flex gap-1">
                            {isBanActive(ban, now) ? (
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
        </IPBanCard>
      )}

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
    </div>
  );
}
