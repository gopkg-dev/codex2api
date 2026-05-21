import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Activity,
  CheckCircle2,
  Copy,
  CopyCheck,
  Cpu,
  Gauge,
  HardDrive,
  Radio,
  RefreshCw,
  Wrench,
} from 'lucide-react'
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { api } from '../api'
import { useBranding } from '../branding'
import { getBucketConfig, getTimeRangeISO, type TimeRangeKey } from '../lib/timeRange'
import type { ChartAggregation, PublicHomeResponse, UsageModelStat } from '../types'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

const PUBLIC_REFRESH_INTERVAL_SECONDS = 5
const PUBLIC_TIME_RANGES: TimeRangeKey[] = ['1h', '6h', '24h', '7d']
const QQ_GROUP_NUMBER = '1054851130'
const QQ_GROUP_URL = 'https://qm.qq.com/q/PphhfxKPee'

export default function PublicHome() {
  const { siteName } = useBranding()
  const [overview, setOverview] = useState<PublicHomeResponse | null>(null)
  const [chartData, setChartData] = useState<ChartAggregation | null>(null)
  const [timeRange, setTimeRange] = useState<TimeRangeKey>('1h')
  const [loading, setLoading] = useState(true)
  const [chartLoading, setChartLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshCountdown, setRefreshCountdown] = useState(PUBLIC_REFRESH_INTERVAL_SECONDS)
  const [error, setError] = useState('')
  const [copiedTarget, setCopiedTarget] = useState('')
  const chartAbort = useRef<AbortController | null>(null)
  const copyResetTimer = useRef<number | null>(null)
  const refreshInFlight = useRef(false)

  const loadOverview = useCallback(async () => {
    const res = await api.getPublicHome()
    setOverview(res)
  }, [])

  const loadChart = useCallback(async (options?: { background?: boolean }) => {
    chartAbort.current?.abort()
    const controller = new AbortController()
    chartAbort.current = controller
    if (!options?.background) {
      setChartLoading(true)
    }
    try {
      const { start, end } = getTimeRangeISO(timeRange)
      const { bucketMinutes } = getBucketConfig(timeRange)
      const res = await api.getPublicChartData({ start, end, bucketMinutes })
      if (!controller.signal.aborted) {
        setChartData(res)
      }
    } finally {
      if (!controller.signal.aborted) {
        setChartLoading(false)
      }
    }
  }, [timeRange])

  const reload = useCallback(async (options?: { background?: boolean }) => {
    if (refreshInFlight.current) return
    refreshInFlight.current = true
    setError('')
    setRefreshing(true)
    try {
      await Promise.all([loadOverview(), loadChart(options)])
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
      setRefreshCountdown(PUBLIC_REFRESH_INTERVAL_SECONDS)
      refreshInFlight.current = false
    }
  }, [loadChart, loadOverview])

  useEffect(() => {
    void reload()
  }, [reload])

  useEffect(() => {
    return () => {
      if (copyResetTimer.current !== null) {
        window.clearTimeout(copyResetTimer.current)
      }
    }
  }, [])

  const copyText = useCallback(async (text: string, target: string) => {
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      setCopiedTarget(target)
      if (copyResetTimer.current !== null) {
        window.clearTimeout(copyResetTimer.current)
      }
      copyResetTimer.current = window.setTimeout(() => {
        setCopiedTarget((current) => (current === target ? '' : current))
      }, 1200)
    } catch {
      setError('复制失败')
    }
  }, [])

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return
      setRefreshCountdown((current) => {
        if (current <= 1) {
          void reload({ background: true })
          return PUBLIC_REFRESH_INTERVAL_SECONDS
        }
        return current - 1
      })
    }, 1000)
    return () => window.clearInterval(timer)
  }, [reload])

  const chartPoints = useMemo(() => {
    if (!chartData) return []
    const { bucketMinutes } = getBucketConfig(timeRange)
    return chartData.timeline.map((point) => {
      const tokenTotal = point.input_tokens + point.output_tokens + point.reasoning_tokens + point.cached_tokens
      const date = new Date(point.bucket)
      return {
        label: formatChartLabel(date, timeRange),
        fullLabel: date.toLocaleString(),
        qps: round(point.requests / (bucketMinutes * 60), 3),
        rpm: round(point.requests / bucketMinutes, 2),
        tpm: round(tokenTotal / bucketMinutes, 2),
      }
    })
  }, [chartData, timeRange])

  const usage = overview?.usage
  const ops = overview?.ops
  const maintenance = overview?.maintenance
  const latestKey = overview?.latest_key?.trim() ?? ''
  const modelStats = usage?.model_stats ?? []
  const publicApiBase = window.location.origin

  return (
    <main className="min-h-screen bg-background px-4 py-5 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5">
        <header className="flex flex-col gap-4 rounded-lg border border-border bg-card px-4 py-4 shadow-sm sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="mb-2 inline-flex items-center gap-2 rounded-md border border-primary/20 bg-primary/10 px-2.5 py-1 text-xs font-semibold text-primary">
              <Radio className="size-3.5" />
              Public Status
            </div>
            <h1 className="text-2xl font-bold tracking-normal text-foreground sm:text-3xl">{siteName}</h1>
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
                <span className="inline-flex size-4 items-center justify-center text-base leading-none" aria-hidden="true">🐧</span>
                QQ 群 {QQ_GROUP_NUMBER}
              </a>
              <CopyIconButton
                label="复制 QQ 群号"
                copied={copiedTarget === 'qq'}
                onClick={() => void copyText(QQ_GROUP_NUMBER, 'qq')}
              />
              <StatusPill enabled={Boolean(maintenance?.enabled)} />
              <div className="inline-flex h-8 items-center gap-2 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 text-sm font-semibold text-emerald-700 dark:text-emerald-300">
                <RefreshCw className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`} />
                5s 自动刷新
                <span className="rounded bg-emerald-500/15 px-1.5 py-0.5 font-mono text-xs tabular-nums">
                  {refreshCountdown}s
                </span>
              </div>
              <Button type="button" variant="outline" size="sm" onClick={() => void reload()} disabled={loading || refreshing}>
                <RefreshCw className={`size-4 ${loading || refreshing ? 'animate-spin' : ''}`} />
                刷新
              </Button>
            </div>
            <div className="flex max-w-full flex-wrap items-center gap-2 sm:justify-end">
              <div className="flex max-w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-sm">
                <span className="shrink-0 font-semibold text-muted-foreground">API 地址</span>
                <span className="min-w-0 max-w-[min(50vw,260px)] truncate font-mono text-xs font-semibold tabular-nums text-foreground" title={publicApiBase}>
                  {publicApiBase}
                </span>
                <CopyIconButton
                  label="复制 API 地址"
                  copied={copiedTarget === 'api_base'}
                  onClick={() => void copyText(publicApiBase, 'api_base')}
                />
              </div>
              <div className="flex max-w-full items-center gap-2 rounded-md border border-border bg-muted/30 px-2.5 py-1.5 text-sm">
                <span className="shrink-0 font-semibold text-muted-foreground">最新密钥</span>
                <span className="min-w-0 max-w-[min(50vw,320px)] truncate font-mono text-xs font-semibold tabular-nums text-foreground" title={latestKey}>
                  {latestKey || '-'}
                </span>
                <CopyIconButton
                  label="复制最新密钥"
                  copied={copiedTarget === 'latest_key'}
                  disabled={!latestKey}
                  onClick={() => void copyText(latestKey, 'latest_key')}
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

        <MaintenanceStatusCard maintenance={maintenance} loading={loading} />

        <section className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <SummaryDashboardCard
            icon={<Activity className="size-5" />}
            title="使用统计"
            description="请求、Token 与计费"
            loading={loading}
            metrics={[
              { label: '总请求数', value: formatInteger(usage?.total_requests) },
              { label: '总 Token 数', value: formatCompactNumber(usage?.total_tokens) },
              { label: '总计费', value: formatMoneyFixed2(usage?.total_user_billed) },
              { label: '今日请求数', value: formatInteger(usage?.today_requests) },
            ]}
          />
          <SummaryDashboardCard
            icon={<Gauge className="size-5" />}
            title="系统运维"
            description="实时吞吐和限流"
            loading={loading}
            metrics={[
              { label: 'QPS', value: formatDecimal(ops?.traffic.qps, 2), sub: `Peak ${formatDecimal(ops?.traffic.qps_peak, 2)}` },
              { label: 'TPS', value: formatDecimal(ops?.traffic.tps, 1), sub: `Peak ${formatDecimal(ops?.traffic.tps_peak, 1)}` },
              { label: 'RPM', value: formatDecimal(ops?.traffic.rpm, 0), sub: (ops?.traffic.rpm_limit ?? 0) > 0 ? `Limit ${formatInteger(ops?.traffic.rpm_limit)}` : '无限制' },
              { label: 'TPM', value: formatDecimal(ops?.traffic.tpm, 0), sub: `${formatDecimal(ops?.traffic.error_rate, 2)}% errors` },
            ]}
          />
          <SummaryDashboardCard
            icon={<Cpu className="size-5" />}
            title="系统负载"
            description="进程与主机资源"
            loading={loading}
            metrics={[
              { label: 'CPU', value: `${formatDecimal(ops?.cpu.percent, 1)}%`, sub: `${ops?.cpu.cores ?? 0} cores` },
              { label: '内存', value: `${formatDecimal(ops?.memory.percent, 1)}%`, sub: `${formatBytes(ops?.memory.used_bytes)} / ${formatBytes(ops?.memory.total_bytes)}` },
              { label: '协程', value: formatInteger(ops?.runtime.goroutines), sub: `${formatInteger(ops?.requests.active)} active` },
              { label: '网络', value: formatBytes(ops?.network.total_bytes), sub: `RX ${formatBytesCompact(ops?.network.rx_bytes)} TX ${formatBytesCompact(ops?.network.tx_bytes)}` },
            ]}
          />
          <SummaryDashboardCard
            icon={<HardDrive className="size-5" />}
            title="缓存 + 健康"
            description="命中率、延迟和错误率"
            loading={loading}
            metrics={[
              { label: '今日命中率', value: formatPercent(usage?.today_cache_rate) },
              { label: '总命中率', value: formatPercent(usage?.total_cache_rate) },
              { label: '首 Token', value: formatLatency(usage?.avg_first_token_ms) },
              { label: '完成延迟', value: formatLatency(usage?.avg_duration_ms) },
              { label: '今日缓存 Token', value: formatCompactNumber(usage?.today_cached_tokens) },
              { label: '错误率', value: formatPercent(usage?.error_rate) },
            ]}
          />
        </section>

        <section className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(320px,1fr)]">
          <Card className="h-[420px] py-0">
            <CardContent className="flex h-full min-h-0 flex-col p-4 sm:p-5">
              <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 className="text-base font-semibold text-foreground">QPS / RPM / TPM 趋势</h2>
                  <p className="mt-1 text-sm text-muted-foreground">按时间窗口动态聚合</p>
                </div>
                <div className="inline-flex w-fit rounded-lg border border-border bg-muted/50 p-0.5">
                  {PUBLIC_TIME_RANGES.map((key) => (
                    <button
                      key={key}
                      type="button"
                      onClick={() => setTimeRange(key)}
                      className={`rounded-md px-3 py-1.5 text-xs font-semibold transition-colors ${
                        timeRange === key ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'
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
                    <LineChart data={chartPoints} margin={{ top: 8, right: 18, left: 0, bottom: 0 }}>
                      <CartesianGrid vertical={false} stroke="var(--color-border)" strokeDasharray="4 4" />
                      <XAxis dataKey="label" tick={{ fill: 'var(--color-muted-foreground)', fontSize: 12 }} tickLine={false} axisLine={{ stroke: 'var(--color-border)' }} minTickGap={18} />
                      <YAxis tick={{ fill: 'var(--color-muted-foreground)', fontSize: 12 }} tickLine={false} axisLine={{ stroke: 'var(--color-border)' }} tickFormatter={formatChartAxisValue} width={56} />
                      <Tooltip
                        labelFormatter={(_, payload) => payload?.[0]?.payload?.fullLabel ?? ''}
                        formatter={(value, name) => [formatChartTooltipValue(value, name), String(name).toUpperCase()]}
                        contentStyle={{ backgroundColor: 'var(--color-card)', border: '1px solid var(--color-border)', borderRadius: 8 }}
                        labelStyle={{ color: 'var(--color-foreground)', fontWeight: 600 }}
                      />
                      <Legend wrapperStyle={{ fontSize: 12 }} />
                      <Line type="monotone" dataKey="qps" name="QPS" stroke="#2563eb" strokeWidth={2.2} dot={false} />
                      <Line type="monotone" dataKey="rpm" name="RPM" stroke="#059669" strokeWidth={2.2} dot={false} />
                      <Line type="monotone" dataKey="tpm" name="TPM" stroke="#7c3aed" strokeWidth={2.2} dot={false} />
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

          <ModelStatsCard stats={modelStats} loading={loading} />
        </section>
      </div>
    </main>
  )
}

function StatusPill({ enabled }: { enabled: boolean }) {
  return (
    <div className={`inline-flex max-w-full items-center gap-2 rounded-md border px-3 py-1.5 text-sm font-semibold ${
      enabled
        ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
        : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    }`}>
      <span className="size-2 rounded-full bg-current" />
      <span className="truncate">{enabled ? '维护中' : '正常运行'}</span>
    </div>
  )
}

function CopyIconButton({
  label,
  copied,
  disabled,
  onClick,
}: {
  label: string
  copied: boolean
  disabled?: boolean
  onClick: () => void
}) {
  const Icon = copied ? CopyCheck : Copy
  return (
    <Button
      type="button"
      variant="outline"
      size="icon-sm"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
      className={copied ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/15 dark:text-emerald-300' : ''}
    >
      <Icon className="size-4" />
    </Button>
  )
}

interface SummaryMetric {
  label: string
  value: string
  sub?: string
}

function SummaryDashboardCard({
  icon,
  title,
  description,
  metrics,
  loading,
}: {
  icon: ReactNode
  title: string
  description: string
  metrics: SummaryMetric[]
  loading: boolean
}) {
  return (
    <Card className="py-0">
      <CardContent className="flex min-h-[236px] flex-col p-4">
        <div className="mb-4 flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            {icon}
          </div>
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold text-foreground" title={title}>{title}</h2>
            <p className="mt-1 truncate text-sm text-muted-foreground" title={description}>{description}</p>
          </div>
        </div>

        <div className="grid flex-1 grid-cols-2 gap-2">
          {metrics.map((metric) => (
            <SummaryMetricCell key={metric.label} metric={metric} loading={loading} />
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

function SummaryMetricCell({ metric, loading }: { metric: SummaryMetric; loading: boolean }) {
  return (
    <div className="min-w-0 rounded-lg border border-border/70 bg-muted/20 p-3">
      <div className="truncate text-xs font-semibold text-muted-foreground" title={metric.label}>{metric.label}</div>
      {loading ? (
        <div className="mt-2 h-6 w-20 animate-pulse rounded-md bg-muted" />
      ) : (
        <div className="mt-1 truncate text-xl font-bold leading-none tabular-nums text-foreground" title={metric.value}>
          {metric.value}
        </div>
      )}
      {metric.sub && <div className="mt-2 truncate text-[11px] text-muted-foreground" title={metric.sub}>{metric.sub}</div>}
    </div>
  )
}

function MaintenanceStatusCard({
  maintenance,
  loading,
}: {
  maintenance: PublicHomeResponse['maintenance'] | undefined
  loading: boolean
}) {
  const routes = maintenance?.routes ?? []
  const enabled = Boolean(maintenance?.enabled)
  const maintenanceRoutes = routes.filter((route) => route.maintenance)
  const availableRoutes = routes.filter((route) => !route.maintenance)
  const allMaintenance = enabled && routes.length > 0 && availableRoutes.length === 0
  const partialMaintenance = enabled && maintenanceRoutes.length > 0 && availableRoutes.length > 0
  const statusText = allMaintenance
    ? (maintenance?.message || '系统维护中，请稍后重试。')
    : partialMaintenance
      ? '系统维护中，部分端点可用'
      : '所有 API 端点正常放行'
  const tone = allMaintenance ? 'danger' : enabled ? 'warning' : 'normal'
  const toneClass = {
    danger: 'border-destructive/30 bg-destructive/10 text-destructive',
    warning: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    normal: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  }[tone]
  const iconClass = {
    danger: 'bg-destructive/10 text-destructive',
    warning: 'bg-amber-500/10 text-amber-600 dark:text-amber-300',
    normal: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-300',
  }[tone]

  return (
    <Card className="py-0">
      <CardContent className="p-4 sm:p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <div className={`flex size-10 shrink-0 items-center justify-center rounded-lg ${iconClass}`}>
              <Wrench className="size-5" />
            </div>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="text-base font-semibold text-foreground">API 维护模式</h2>
                <span className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-semibold ${toneClass}`}>
                  <span className="size-1.5 rounded-full bg-current" />
                  {enabled ? (allMaintenance ? '全部维护' : '已开启') : '已关闭'}
                </span>
              </div>
              <p className={`mt-1 text-sm ${allMaintenance ? 'font-semibold text-destructive' : 'text-muted-foreground'}`}>
                {statusText}
              </p>
            </div>
          </div>

          <div className="min-w-0 lg:max-w-[58%]">
            <div className="mb-2 text-xs font-semibold text-muted-foreground">端点状态</div>
            {loading ? (
              <div className="flex flex-wrap gap-2">
                {[0, 1, 2].map((i) => (
                  <div key={i} className="h-7 w-36 animate-pulse rounded-md bg-muted" />
                ))}
              </div>
            ) : enabled && routes.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {routes.map((route) => (
                  <span
                    key={route.path}
                    className={`inline-flex max-w-full items-center gap-1.5 rounded-md border px-2.5 py-1 font-mono text-xs ${
                      route.maintenance
                        ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                        : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                    }`}
                    title={`${route.path} - ${route.maintenance ? '维护中' : '正常放行'}`}
                  >
                    {route.maintenance ? <Wrench className="size-3.5 shrink-0" /> : <CheckCircle2 className="size-3.5 shrink-0" />}
                    <span className="truncate">{route.path}</span>
                  </span>
                ))}
              </div>
            ) : (
              <div className="text-sm text-muted-foreground">暂无维护端点</div>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function ModelStatsCard({ stats, loading }: { stats: UsageModelStat[]; loading: boolean }) {
  const maxRequests = Math.max(1, ...stats.map((item) => item.requests))
  return (
    <Card className="h-[420px] py-0">
      <CardContent className="flex h-full min-h-0 flex-col p-4 sm:p-5">
        <div className="mb-4">
          <h2 className="text-base font-semibold text-foreground">模型统计</h2>
          <p className="mt-1 text-sm text-muted-foreground">按请求量和计费聚合</p>
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
              <div key={item.model} className="rounded-lg border border-border/70 bg-muted/20 p-3">
                <div className="mb-2 flex min-w-0 items-center justify-between gap-3">
                  <span className="truncate text-sm font-semibold text-foreground" title={item.model}>{item.model}</span>
                  <span className="shrink-0 text-xs font-semibold tabular-nums text-muted-foreground">{formatMoney(item.user_billed)}</span>
                </div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  <div className="h-full rounded-full bg-primary" style={{ width: `${Math.max(4, (item.requests / maxRequests) * 100)}%` }} />
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
  )
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
  )
}

function formatChartLabel(date: Date, range: TimeRangeKey): string {
  if (range === '7d') {
    return `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, '0')}:00`
  }
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function formatInteger(value?: number | null): string {
  return Math.round(value ?? 0).toLocaleString()
}

function formatCompactNumber(value?: number | null): string {
  const amount = Math.round(value ?? 0)
  const abs = Math.abs(amount)
  const units = [
    { value: 1_000_000_000, suffix: 'B' },
    { value: 1_000_000, suffix: 'M' },
    { value: 1_000, suffix: 'K' },
  ]
  const unit = units.find((item) => abs >= item.value)
  if (!unit) return amount.toLocaleString()
  const compact = amount / unit.value
  const digits = Math.abs(compact) >= 100 ? 0 : 1
  return `${compact.toFixed(digits).replace(/\.0$/, '')}${unit.suffix}`
}

function formatDecimal(value?: number | null, digits = 2): string {
  return (value ?? 0).toFixed(digits)
}

function formatPercent(value?: number | null, digits = 2): string {
  return `${formatDecimal(value, digits)}%`
}

function formatLatency(value?: number | null): string {
  const ms = value ?? 0
  if (ms <= 0) return '-'
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.round(ms)}ms`
}

function formatMoney(value?: number | null): string {
  const amount = value ?? 0
  if (amount >= 100) return `$${amount.toLocaleString(undefined, { maximumFractionDigits: 1 })}`
  if (amount >= 1) return `$${amount.toFixed(2)}`
  if (amount >= 0.01) return `$${amount.toFixed(4)}`
  return `$${amount.toFixed(6)}`
}

function formatMoneyFixed2(value?: number | null): string {
  return `$${(value ?? 0).toFixed(2)}`
}

function formatBytes(bytes?: number | null): string {
  const value = bytes ?? 0
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = value / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[index]}`
}

function formatBytesCompact(bytes?: number | null): string {
  const value = bytes ?? 0
  if (value < 1024) return `${value}B`
  const units = ['K', 'M', 'G', 'T']
  let size = value / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  const digits = size >= 100 ? 0 : size >= 10 ? 1 : 2
  return `${size.toFixed(digits).replace(/\.0+$/, '')}${units[index]}`
}

function round(value: number, digits: number): number {
  const factor = 10 ** digits
  return Math.round(value * factor) / factor
}

function formatChartAxisValue(value: number): string {
  const abs = Math.abs(value)
  if (abs === 0) return '0'
  if (abs < 0.01) return '<0.01'
  if (abs < 1) return value.toFixed(2)
  if (abs < 1000) return Math.round(value).toLocaleString()
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 0 }).format(value)
}

function formatChartTooltipValue(value: unknown, name: unknown): string {
  if (typeof value !== 'number') return String(value)
  const metric = String(name).toLowerCase()
  if (metric === 'qps') {
    if (value > 0 && value < 0.01) return '<0.01'
    return value.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  }
  return Math.round(value).toLocaleString()
}
