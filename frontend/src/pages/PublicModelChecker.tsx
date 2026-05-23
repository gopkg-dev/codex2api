import { FormEvent, useMemo, useState } from "react";
import {
  Activity,
  ArrowLeft,
  CheckCircle2,
  ClipboardList,
  Loader2,
  Search,
  XCircle,
} from "lucide-react";
import { api } from "../api";
import type {
  PublicModelCheckResponse,
  PublicModelEndpointSupport,
} from "../types";

function defaultBaseURL(): string {
  return `${window.location.origin.replace(/\/$/, "")}/v1`;
}

export default function PublicModelChecker() {
  const [baseURL, setBaseURL] = useState(defaultBaseURL);
  const [apiKey, setAPIKey] = useState("");
  const [result, setResult] = useState<PublicModelCheckResponse | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const summary = useMemo(() => {
    const rows = result?.rows ?? [];
    return {
      total: rows.length,
      all: rows.filter((row) => row.usable_all).length,
      failed: rows.filter((row) => !row.usable_any).length,
    };
  }, [result]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoading(true);
    setError("");
    setResult(null);
    try {
      const data = await api.checkPublicModels({
        base_url: baseURL,
        api_key: apiKey,
      });
      setResult(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "检测失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="min-h-screen bg-background px-4 py-6 text-foreground sm:px-6 lg:px-8">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5">
        <header className="rounded-lg border border-border bg-card p-5 shadow-sm">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex min-w-0 items-start gap-4">
              <div className="grid size-14 shrink-0 place-items-center rounded-lg bg-primary/10 text-primary">
                <Activity className="size-7" />
              </div>
              <div className="min-w-0">
                <a
                  href="/"
                  className="mb-3 inline-flex items-center gap-1.5 rounded-md border border-border bg-muted/40 px-2.5 py-1 text-sm font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <ArrowLeft className="size-4" />
                  公开首页
                </a>
                <h1 className="text-3xl font-black tracking-normal text-foreground sm:text-4xl">
                  模型可用性检测
                </h1>
                <p className="mt-2 max-w-2xl text-base leading-7 text-muted-foreground">
                  检测当前 OpenAI 兼容服务中 GPT 与 Codex 模型对 Chat Completions 和 Responses 的支持情况。
                </p>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-2 sm:min-w-[360px]">
              <SummaryTile label="模型" value={summary.total} />
              <SummaryTile label="双端点" value={summary.all} tone="success" />
              <SummaryTile label="失败" value={summary.failed} tone="danger" />
            </div>
          </div>
        </header>

        <section className="rounded-lg border border-border bg-card p-5 shadow-sm">
          <form
            className="grid gap-4 lg:grid-cols-[minmax(260px,1fr)_minmax(260px,1fr)_auto]"
            onSubmit={(event) => void handleSubmit(event)}
          >
            <label className="grid gap-2 text-sm font-semibold text-muted-foreground">
              Base URL
              <input
                className="h-11 rounded-md border border-input bg-background px-3 font-mono text-sm text-foreground outline-none ring-ring transition focus:ring-2"
                value={baseURL}
                onChange={(event) => setBaseURL(event.target.value)}
                autoComplete="url"
                required
              />
            </label>
            <label className="grid gap-2 text-sm font-semibold text-muted-foreground">
              API Key
              <input
                className="h-11 rounded-md border border-input bg-background px-3 font-mono text-sm text-foreground outline-none ring-ring transition focus:ring-2"
                value={apiKey}
                onChange={(event) => setAPIKey(event.target.value)}
                type="password"
                autoComplete="off"
                placeholder="sk-..."
                required
              />
            </label>
            <button
              type="submit"
              disabled={loading}
              className="inline-flex h-11 items-center justify-center gap-2 self-end rounded-md bg-primary px-5 text-sm font-bold text-primary-foreground shadow-sm hover:bg-primary/90 disabled:cursor-wait disabled:opacity-70"
            >
              {loading ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Search className="size-4" />
              )}
              {loading ? "检测中" : "开始检测"}
            </button>
          </form>
          {error ? (
            <div className="mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm font-semibold text-red-700">
              {error}
            </div>
          ) : null}
        </section>

        {loading ? (
          <section className="grid min-h-[280px] place-items-center rounded-lg border border-border bg-card p-8 text-center shadow-sm">
            <div>
              <Loader2 className="mx-auto size-10 animate-spin text-primary" />
              <div className="mt-4 text-lg font-bold">正在检测模型可用性</div>
              <div className="mt-1 text-sm text-muted-foreground">
                会先读取 /models，再逐个测试两个端点。
              </div>
            </div>
          </section>
        ) : null}

        {result ? (
          <>
            <section className="grid gap-4 lg:grid-cols-2">
              <UsableModelPanel
                title="Chat Completions 可用模型"
                models={result.chat_usable}
              />
              <UsableModelPanel
                title="Responses 可用模型"
                models={result.responses_usable}
              />
            </section>
            <section className="overflow-hidden rounded-lg border border-border bg-card shadow-sm">
              <div className="flex flex-col gap-2 border-b border-border bg-muted/30 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-center gap-2 text-lg font-black">
                  <ClipboardList className="size-5 text-primary" />
                  检测明细
                </div>
                <div className="font-mono text-xs text-muted-foreground">
                  {result.base_url}
                </div>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full min-w-[760px] border-collapse">
                  <thead>
                    <tr className="border-b border-border bg-muted/35 text-left text-xs uppercase tracking-normal text-muted-foreground">
                      <th className="px-4 py-3">模型</th>
                      <th className="px-4 py-3">Chat</th>
                      <th className="px-4 py-3">Responses</th>
                      <th className="px-4 py-3">错误摘要</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.rows.map((row) => (
                      <tr
                        key={row.model}
                        className="border-b border-border last:border-0"
                      >
                        <td className="px-4 py-3 font-mono text-sm font-bold">
                          {row.model}
                        </td>
                        <td className="px-4 py-3">
                          <EndpointBadge endpoint={row.chat} />
                        </td>
                        <td className="px-4 py-3">
                          <EndpointBadge endpoint={row.responses} />
                        </td>
                        <td className="max-w-[520px] px-4 py-3 text-sm text-muted-foreground">
                          {endpointErrors(row.chat, row.responses) || "-"}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
          </>
        ) : null}
      </div>
    </main>
  );
}

function SummaryTile({
  label,
  value,
  tone = "default",
}: {
  label: string;
  value: number;
  tone?: "default" | "success" | "danger";
}) {
  const toneClass =
    tone === "success"
      ? "text-emerald-700"
      : tone === "danger"
        ? "text-red-600"
        : "text-foreground";
  return (
    <div className="rounded-lg border border-border bg-background/65 p-3">
      <div className="text-xs font-bold text-muted-foreground">{label}</div>
      <div className={`mt-1 text-2xl font-black tabular-nums ${toneClass}`}>
        {value}
      </div>
    </div>
  );
}

function UsableModelPanel({
  title,
  models,
}: {
  title: string;
  models: string[];
}) {
  return (
    <section className="rounded-lg border border-border bg-card p-4 shadow-sm">
      <h2 className="text-lg font-black">{title}</h2>
      <div className="mt-3 flex max-h-44 flex-wrap gap-2 overflow-y-auto pr-1">
        {models.length > 0 ? (
          models.map((model) => (
            <span
              key={model}
              className="rounded-md border border-emerald-200 bg-emerald-50 px-2.5 py-1 font-mono text-sm font-bold text-emerald-700"
            >
              {model}
            </span>
          ))
        ) : (
          <span className="rounded-md border border-border bg-muted/45 px-2.5 py-1 text-sm font-semibold text-muted-foreground">
            无
          </span>
        )}
      </div>
    </section>
  );
}

function EndpointBadge({ endpoint }: { endpoint: PublicModelEndpointSupport }) {
  if (endpoint.ok) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-md border border-emerald-200 bg-emerald-50 px-2 py-1 text-sm font-bold text-emerald-700">
        <CheckCircle2 className="size-4" />
        可用
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md border border-red-200 bg-red-50 px-2 py-1 text-sm font-bold text-red-700">
      <XCircle className="size-4" />
      失败{endpoint.status ? ` ${endpoint.status}` : ""}
    </span>
  );
}

function endpointErrors(
  chat: PublicModelEndpointSupport,
  responses: PublicModelEndpointSupport,
): string {
  const errors: string[] = [];
  if (!chat.ok && chat.error) errors.push(`Chat: ${chat.error}`);
  if (!responses.ok && responses.error)
    errors.push(`Responses: ${responses.error}`);
  return errors.join(" | ");
}
