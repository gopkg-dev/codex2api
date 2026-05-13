import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Copy, Check, ClipboardCheck, ExternalLink, Sparkles, Terminal, KeyRound, Wand2, Server } from 'lucide-react'
import { api } from '../api'
import PageHeader from '../components/PageHeader'
import ToastNotice from '../components/ToastNotice'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Select } from '@/components/ui/select'
import { useToast } from '../hooks/useToast'
import { CodeBlock, EndpointDoc } from './docs/EndpointDoc'
import DocsTOC, { type DocsTOCItem } from './docs/DocsTOC'
import { QUICK_TOOLS, resolveTemplate, type QuickTool } from './docs/quickStartTools'
import { buildAdminSpecs, buildDocsMarkdown, buildEndpointSpecs } from './docs/docsContent'

const FALLBACK_MODELS = ['gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex', 'claude-sonnet-4-5-20250514']

const SECTION_ICON: Record<string, ReactNode> = {
  'quick-start': <Sparkles className="size-4" />,
  'client-config': <Terminal className="size-4" />,
  'authentication': <KeyRound className="size-4" />,
  'model-api': <Wand2 className="size-4" />,
  'admin-api': <Server className="size-4" />,
}

const SECTION_TONE: Record<string, { text: string; bg: string; ring: string }> = {
  'quick-start': { text: 'text-amber-600 dark:text-amber-400', bg: 'bg-amber-500/10 dark:bg-amber-500/15', ring: 'ring-amber-500/20' },
  'client-config': { text: 'text-emerald-600 dark:text-emerald-400', bg: 'bg-emerald-500/10 dark:bg-emerald-500/15', ring: 'ring-emerald-500/20' },
  'authentication': { text: 'text-fuchsia-600 dark:text-fuchsia-400', bg: 'bg-fuchsia-500/10 dark:bg-fuchsia-500/15', ring: 'ring-fuchsia-500/20' },
  'model-api': { text: 'text-sky-600 dark:text-sky-400', bg: 'bg-sky-500/10 dark:bg-sky-500/15', ring: 'ring-sky-500/20' },
  'admin-api': { text: 'text-rose-600 dark:text-rose-400', bg: 'bg-rose-500/10 dark:bg-rose-500/15', ring: 'ring-rose-500/20' },
}

function OsTabs({ active, onChange }: { active: 'unix' | 'windows'; onChange: (v: 'unix' | 'windows') => void }) {
  const { t } = useTranslation()
  return (
    <div className="border-b border-border mb-4">
      <nav className="-mb-px flex space-x-4">
        <button
          onClick={() => onChange('unix')}
          className={`whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
            active === 'unix'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground hover:border-border'
          }`}
        >
          macOS / Linux
        </button>
        <button
          onClick={() => onChange('windows')}
          className={`whitespace-nowrap py-2.5 px-1 border-b-2 font-medium text-sm transition-colors flex items-center gap-2 ${
            active === 'windows'
              ? 'border-primary text-primary'
              : 'border-transparent text-muted-foreground hover:text-foreground hover:border-border'
          }`}
        >
          Windows
        </button>
      </nav>
    </div>
  )
}

function UnderlineTabs<T extends string>({ tabs, active, onChange }: {
  tabs: { value: T; label: string; hint?: string }[]
  active: T
  onChange: (value: T) => void
}) {
  return (
    <div className="border-b border-border">
      <nav className="-mb-px flex space-x-4">
        {tabs.map((tab) => {
          const selected = active === tab.value
          return (
          <button
            key={tab.value}
            type="button"
            onClick={() => onChange(tab.value)}
            className={`whitespace-nowrap border-b-2 px-1 py-2.5 text-sm font-medium transition-colors ${
              selected
                ? 'border-primary text-primary'
                : 'border-transparent text-muted-foreground hover:border-border hover:text-foreground'
            }`}
            title={tab.hint}
          >
            {tab.label}
          </button>
          )
        })}
      </nav>
    </div>
  )
}

function QuickToolCard({ tool, baseUrl, apiKey, onCopied, onLaunched }: {
  tool: QuickTool
  baseUrl: string
  apiKey: string
  onCopied: (name: string) => void
  onLaunched: (name: string) => void
}) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const isProtocol = tool.kind === 'protocol'
  const hasKey = Boolean(apiKey)
  const previewKey = hasKey ? apiKey : 'YOUR_API_KEY'
  const resolved = resolveTemplate(tool, baseUrl, previewKey)

  const handleClick = async () => {
    if (isProtocol) {
      if (!hasKey) return
      window.open(resolved, '_blank')
      onLaunched(tool.name)
      return
    }
    try {
      await navigator.clipboard.writeText(resolved)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = resolved
      ta.style.cssText = 'position:fixed;left:-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    setCopied(true)
    onCopied(tool.name)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="group relative flex flex-col gap-2.5 rounded-xl border border-border bg-card/70 p-4 transition-all hover:-translate-y-0.5 hover:border-primary/30 hover:shadow-md">
      <div className="flex items-start gap-3">
        <div className={`inline-flex size-10 shrink-0 items-center justify-center rounded-lg text-sm font-bold ${tool.iconHue}`}>
          {tool.glyph}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <h4 className="truncate text-[14px] font-bold text-foreground">{tool.name}</h4>
            <Badge variant="outline" className="shrink-0 px-1.5 py-0 text-[10px] font-bold">
              {tool.badge}
            </Badge>
          </div>
          <p className="mt-1 line-clamp-2 text-[12px] leading-snug text-muted-foreground">
            {tool.blurb}
          </p>
        </div>
      </div>
      <Button
        variant={isProtocol ? 'default' : 'outline'}
        size="sm"
        disabled={isProtocol && !hasKey}
        onClick={() => void handleClick()}
        className="mt-1 w-full justify-center gap-1.5"
      >
        {isProtocol ? (
          <>
            <ExternalLink className="size-3.5" />
            {hasKey ? t('docs.quickStart.launch') : t('docs.quickStart.needKey')}
          </>
        ) : copied ? (
          <>
            <ClipboardCheck className="size-3.5 text-emerald-500" />
            {t('docs.quickStart.copied')}
          </>
        ) : (
          <>
            <Copy className="size-3.5" />
            {t('docs.quickStart.copyConfig')}
          </>
        )}
      </Button>
    </div>
  )
}

function buildCCSwitchImportUrl({
  app,
  name,
  endpoint,
  apiKey,
  model,
  homepage,
}: {
  app: 'claude' | 'codex'
  name: string
  endpoint: string
  apiKey: string
  model: string
  homepage: string
}) {
  const params = new URLSearchParams()
  params.set('resource', 'provider')
  params.set('app', app)
  params.set('name', name)
  params.set('endpoint', endpoint)
  params.set('apiKey', apiKey)
  params.set('model', model)
  params.set('homepage', homepage)
  params.set('enabled', 'true')
  return `ccswitch://v1/import?${params.toString()}`
}

function encodeBase64(text: string): string {
  return btoa(unescape(encodeURIComponent(text)))
}

function SectionHeader({ id, icon, tone, eyebrow, title, description }: { id: string; icon: ReactNode; tone: { text: string; bg: string; ring: string }; eyebrow?: string; title: string; description?: string }) {
  return (
    <div id={id} className="scroll-mt-20 mt-6 mb-4 first:mt-2">
      <div className="flex items-start gap-3">
        <span className={`mt-0.5 inline-flex size-9 shrink-0 items-center justify-center rounded-xl ring-1 ${tone.bg} ${tone.text} ${tone.ring}`}>
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          {eyebrow ? (
            <div className={`text-[10.5px] font-bold uppercase tracking-[0.14em] ${tone.text}`}>{eyebrow}</div>
          ) : null}
          <h2 className="mt-0.5 text-[22px] font-bold leading-tight text-foreground">{title}</h2>
          {description ? <p className="mt-1 max-w-[640px] text-[13px] leading-relaxed text-muted-foreground">{description}</p> : null}
        </div>
      </div>
    </div>
  )
}

export default function Docs() {
  const { t } = useTranslation()
  const baseUrl = useMemo(() => window.location.origin, [])
  const [codexOs, setCodexOs] = useState<'unix' | 'windows'>('unix')
  const [claudeOs, setClaudeOs] = useState<'unix' | 'windows'>('unix')
  const [firstKey, setFirstKey] = useState('')
  const [allKeys, setAllKeys] = useState<{ name: string; key: string }[]>([])
  const [copyingMd, setCopyingMd] = useState(false)
  const { toast, showToast } = useToast()
  const [selectedKey, setSelectedKey] = useState('')
  const [activeToolTab, setActiveToolTab] = useState<'codex-cli' | 'claude-code' | 'cc-switch' | 'cherry-studio'>('codex-cli')
  const [quickStartModel, setQuickStartModel] = useState('gpt-5.4')
  const [activeCurl, setActiveCurl] = useState<'responses' | 'chat' | 'messages'>('responses')
  const [curlModel, setCurlModel] = useState('gpt-5.4')
  const [models, setModels] = useState(FALLBACK_MODELS)

  useEffect(() => {
    api.getAPIKeys().then((res) => {
      const keys = (res.keys ?? []).map((k) => ({ name: k.name, key: k.raw_key || k.key }))
      setAllKeys(keys)
      if (keys.length > 0) {
        setFirstKey(keys[0].key)
        setSelectedKey(keys[0].key)
      }
    }).catch(() => {})
  }, [])

  useEffect(() => {
    api.getModels().then((res) => {
      const next = [
        ...(res.models ?? []),
        ...((res.items ?? []).map((item) => item.id)),
      ].filter(Boolean)
      const unique = Array.from(new Set(next))
      if (unique.length === 0) return
      setModels(unique)
      const preferred = unique.includes('gpt-5.4') ? 'gpt-5.4' : unique[0]
      setQuickStartModel((current) => unique.includes(current) ? current : preferred)
      setCurlModel((current) => unique.includes(current) ? current : preferred)
    }).catch(() => {})
  }, [])

  const modelEndpoints = useMemo(() => buildEndpointSpecs(baseUrl), [baseUrl])
  const adminEndpoints = useMemo(() => buildAdminSpecs(baseUrl), [baseUrl])

  const tocItems: DocsTOCItem[] = useMemo(() => [
    {
      id: 'quick-start',
      label: t('docs.toc.quickStart'),
      children: [
        { id: 'qs-tools', label: t('docs.toc.qsTools') },
        { id: 'qs-curl', label: t('docs.toc.qsCurl') },
      ],
    },
    {
      id: 'client-config',
      label: t('docs.toc.clientConfig'),
      children: [
        { id: 'client-codex', label: 'Codex CLI' },
        { id: 'client-claude', label: 'Claude Code' },
        { id: 'client-mapping', label: t('docs.toc.modelMapping') },
      ],
    },
    {
      id: 'authentication',
      label: t('docs.toc.authentication'),
    },
    {
      id: 'model-api',
      label: t('docs.toc.modelApi'),
      children: modelEndpoints.map((e) => ({ id: e.id, label: e.path, method: e.method })),
    },
    {
      id: 'admin-api',
      label: t('docs.toc.adminApi'),
      children: adminEndpoints.map((e) => ({ id: e.id, label: e.path, method: e.method })),
    },
  ], [t, modelEndpoints, adminEndpoints])

  const handleCopyMarkdown = async () => {
    setCopyingMd(true)
    const md = buildDocsMarkdown({
      baseUrl,
      quickTools: QUICK_TOOLS,
      apiKeyExample: firstKey || 'YOUR_API_KEY',
    })
    try {
      await navigator.clipboard.writeText(md)
      showToast(t('docs.markdownCopied'), 'success')
    } catch {
      const ta = document.createElement('textarea')
      ta.value = md
      ta.style.cssText = 'position:fixed;left:-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      showToast(t('docs.markdownCopied'), 'success')
    } finally {
      setTimeout(() => setCopyingMd(false), 1200)
    }
  }

  const codexConfigDir = codexOs === 'windows' ? '%userprofile%\\.codex' : '~/.codex'
  const claudeConfigDir = claudeOs === 'windows' ? '%userprofile%\\.claude' : '~/.claude'
  const activeKey = selectedKey || firstKey || 'YOUR_API_KEY'

  const codexConfigToml = `model_provider = "OpenAI"
model = "${quickStartModel}"
review_model = "${quickStartModel}"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`

  const codexAuthJson = `{
  "OPENAI_API_KEY": "${activeKey}"
}`

  const claudeSettingsJson = `{
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "${activeKey}",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}`

  const claudeEnvUnix = `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="${activeKey}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`

  const claudeEnvWindows = `set ANTHROPIC_BASE_URL=${baseUrl}
set ANTHROPIC_AUTH_TOKEN=${activeKey}
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`

  const responsesCurl = `curl -X POST ${baseUrl}/v1/responses \\
  -H "Authorization: Bearer ${activeKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${curlModel}",
    "input": [{"role": "user", "content": [{"type": "input_text", "text": "Hello"}]}],
    "stream": true
  }'`
  const chatCurl = `curl -X POST ${baseUrl}/v1/chat/completions \\
  -H "Authorization: Bearer ${activeKey}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${curlModel}",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'`
  const messagesCurl = `curl -X POST ${baseUrl}/v1/messages \\
  -H "x-api-key: ${activeKey}" \\
  -H "Content-Type: application/json" \\
  -H "anthropic-version: 2023-06-01" \\
  -d '{
    "model": "${curlModel.startsWith('claude-') ? curlModel : 'claude-sonnet-4-5-20250514'}",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello"}]
  }'`
  const curlExamples = {
    responses: responsesCurl,
    chat: chatCurl,
    messages: messagesCurl,
  }
  const ccSwitchApp = quickStartModel.startsWith('claude-') ? 'claude' : 'codex'
  const ccSwitchUrl = buildCCSwitchImportUrl({
    app: ccSwitchApp,
    name: ccSwitchApp === 'codex' ? 'Codex2API Codex' : 'Codex2API Claude',
    endpoint: ccSwitchApp === 'codex' ? `${baseUrl}/v1` : baseUrl,
    apiKey: activeKey,
    model: quickStartModel,
    homepage: baseUrl,
  })
  const cherryConfig = `cherrystudio://providers/api-keys?v=1&data=${encodeURIComponent(encodeBase64(JSON.stringify({
    id: 'codex2api',
    baseUrl,
    apiKey: activeKey,
  })))}`
  return (
    <>
      <PageHeader
        title={t('docs.title')}
        description={t('docs.description')}
        actions={
          <Button variant="outline" onClick={() => void handleCopyMarkdown()} disabled={copyingMd} className="gap-1.5">
            {copyingMd ? <Check className="size-3.5 text-emerald-500" /> : <Copy className="size-3.5" />}
            {t('docs.copyMarkdown')}
          </Button>
        }
      />

      <ToastNotice toast={toast} />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_260px]">
        <div className="min-w-0">
          {/* Mobile horizontal nav */}
          <div className="xl:hidden mb-4 -mx-2 overflow-x-auto px-2">
            <div className="flex gap-1.5 pb-1">
              {tocItems.map((parent) => (
                <a
                  key={parent.id}
                  href={`#${parent.id}`}
                  className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1.5 text-[12px] font-semibold text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground"
                >
                  {parent.label}
                </a>
              ))}
            </div>
          </div>

          {/* Section 1: Quick Start */}
          <SectionHeader
            id="quick-start"
            icon={SECTION_ICON['quick-start']}
            tone={SECTION_TONE['quick-start']}
            eyebrow={t('docs.section1Eyebrow')}
            title={t('docs.quickStart.title')}
            description={t('docs.quickStart.description')}
          />

          <Card id="qs-tools" className="mb-4 scroll-mt-20 py-0">
            <CardContent className="p-5">
              <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-[15px] font-semibold text-foreground">{t('docs.quickStart.toolsTitle')}</h3>
                  <p className="mt-0.5 text-[12.5px] text-muted-foreground">{t('docs.quickStart.toolsDesc')}</p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  {allKeys.length > 0 ? (
                    <>
                      <span className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">{t('docs.quickStart.useKey')}</span>
                      <Select
                        compact
                        className="w-44"
                        value={selectedKey}
                        onValueChange={setSelectedKey}
                        options={allKeys.map((k) => ({
                          label: k.name ? `${k.name} · ${k.key.slice(0, 6)}…${k.key.slice(-4)}` : k.key,
                          value: k.key,
                        }))}
                      />
                    </>
                  ) : (
                    <a
                      href="/admin/api-keys"
                      className="inline-flex items-center gap-1 rounded-md border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[11px] font-bold text-amber-600 dark:text-amber-400"
                    >
                      {t('docs.quickStart.createKeyFirst')}
                    </a>
                  )}
                </div>
              </div>
              <div className="mb-3">
                <UnderlineTabs
                  active={activeToolTab}
                  onChange={setActiveToolTab}
                  tabs={[
                    { value: 'codex-cli', label: 'Codex CLI' },
                    { value: 'claude-code', label: 'Claude Code' },
                    { value: 'cc-switch', label: 'CC Switch' },
                    { value: 'cherry-studio', label: 'Cherry Studio' },
                  ]}
                />
              </div>
              <div className="mb-4 grid gap-3 md:grid-cols-[minmax(0,1fr)_220px]">
                <div className="rounded-xl border border-border bg-muted/30 p-3">
                  <div className="text-[11px] font-bold uppercase tracking-wide text-muted-foreground">Endpoint</div>
                  <code className="mt-1 block truncate text-sm font-semibold text-foreground">
                    {activeToolTab === 'cc-switch' && ccSwitchApp === 'codex' ? `${baseUrl}/v1` : baseUrl}
                  </code>
                </div>
                <div>
                  <label className="mb-1.5 block text-[11px] font-bold uppercase tracking-wide text-muted-foreground">Model</label>
                  <Select
                    compact
                    value={quickStartModel}
                    onValueChange={setQuickStartModel}
                    options={models.map((model) => ({ label: model, value: model }))}
                  />
                </div>
              </div>
              <div className="space-y-4">
                {activeToolTab === 'codex-cli' && (
                  <>
                    <OsTabs active={codexOs} onChange={setCodexOs} />
                    <CodeBlock label={`${codexConfigDir}/config.toml`} content={codexConfigToml} lang="toml" />
                    <CodeBlock label={`${codexConfigDir}/auth.json`} content={codexAuthJson} lang="json" />
                  </>
                )}
                {activeToolTab === 'claude-code' && (
                  <>
                    <OsTabs active={claudeOs} onChange={setClaudeOs} />
                    <CodeBlock
                      label={claudeOs === 'unix' ? 'Terminal' : 'Command Prompt'}
                      content={claudeOs === 'unix' ? claudeEnvUnix : claudeEnvWindows}
                      lang="bash"
                    />
                    <CodeBlock label={`${claudeConfigDir}/settings.json`} content={claudeSettingsJson} lang="json" />
                  </>
                )}
                {activeToolTab === 'cc-switch' && (
                  <>
                    <CodeBlock label="CC Switch deeplink" content={ccSwitchUrl} lang="bash" />
                    <Button
                      type="button"
                      disabled={!selectedKey && !firstKey}
                      onClick={() => {
                        window.open(ccSwitchUrl, '_blank')
                        showToast(t('docs.quickStart.launchedToast', { name: 'CC Switch' }), 'success')
                      }}
                      className="gap-1.5"
                    >
                      <ExternalLink className="size-3.5" />
                      {t('docs.quickStart.launch')}
                    </Button>
                  </>
                )}
                {activeToolTab === 'cherry-studio' && (
                  <>
                    <CodeBlock label="Cherry Studio deeplink" content={cherryConfig} lang="bash" />
                    <Button
                      type="button"
                      disabled={!selectedKey && !firstKey}
                      onClick={() => {
                        window.open(cherryConfig, '_blank')
                        showToast(t('docs.quickStart.launchedToast', { name: 'Cherry Studio' }), 'success')
                      }}
                      className="gap-1.5"
                    >
                      <ExternalLink className="size-3.5" />
                      {t('docs.quickStart.launch')}
                    </Button>
                  </>
                )}
              </div>
            </CardContent>
          </Card>

          <Card id="qs-curl" className="mb-4 scroll-mt-20 py-0">
            <CardContent className="p-6">
              <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-base font-semibold text-foreground mb-1">{t('docs.quickStart.curlTitle')}</h3>
                  <p className="text-sm text-muted-foreground">{t('docs.quickStart.curlDesc')}</p>
                </div>
                <Select
                  compact
                  className="w-52"
                  value={curlModel}
                  onValueChange={setCurlModel}
                  options={[
                    { label: 'gpt-5.5', value: 'gpt-5.5' },
                    { label: 'gpt-5.4', value: 'gpt-5.4' },
                    { label: 'gpt-5.4-mini', value: 'gpt-5.4-mini' },
                    { label: 'gpt-5.3-codex', value: 'gpt-5.3-codex' },
                    { label: 'claude-sonnet-4-5', value: 'claude-sonnet-4-5-20250514' },
                  ]}
                />
              </div>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <UnderlineTabs
                  active={activeCurl}
                  onChange={setActiveCurl}
                  tabs={[
                    { value: 'responses', label: 'Responses', hint: '/v1/responses' },
                    { value: 'chat', label: 'Chat', hint: '/v1/chat/completions' },
                    { value: 'messages', label: 'Messages', hint: '/v1/messages' },
                  ]}
                />
                <code className="code-inline text-[11px]">
                  {activeCurl === 'responses' ? '/v1/responses' : activeCurl === 'chat' ? '/v1/chat/completions' : '/v1/messages'}
                </code>
              </div>
              <CodeBlock label="cURL" content={curlExamples[activeCurl]} lang="bash" />
            </CardContent>
          </Card>

          {/* Section 2: Client Config */}
          <SectionHeader
            id="client-config"
            icon={SECTION_ICON['client-config']}
            tone={SECTION_TONE['client-config']}
            eyebrow={t('docs.section2Eyebrow')}
            title={t('docs.clientConfig.title')}
            description={t('docs.clientConfig.description')}
          />

          <Card id="client-codex" className="mb-4 scroll-mt-20 py-0">
            <CardContent className="p-6">
              <h3 className="text-base font-semibold text-foreground mb-1">Codex CLI</h3>
              <p className="mb-4 text-sm text-muted-foreground">{t('docs.clientConfig.codexDesc')}</p>
              <OsTabs active={codexOs} onChange={setCodexOs} />
              <p className="mb-3 flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
                ⓘ {t('docs.clientConfig.codexConfigHint')}
              </p>
              <div className="space-y-4">
                <CodeBlock label={`${codexConfigDir}/config.toml`} content={codexConfigToml} lang="toml" />
                <CodeBlock label={`${codexConfigDir}/auth.json`} content={codexAuthJson} lang="json" />
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                {codexOs === 'windows' ? t('docs.clientConfig.codexNoteWindows') : t('docs.clientConfig.codexNoteUnix')}
              </p>
            </CardContent>
          </Card>

          <Card id="client-claude" className="mb-4 scroll-mt-20 py-0">
            <CardContent className="p-6">
              <h3 className="text-base font-semibold text-foreground mb-1">Claude Code</h3>
              <p className="mb-4 text-sm text-muted-foreground">{t('docs.clientConfig.claudeDesc')}</p>
              <OsTabs active={claudeOs} onChange={setClaudeOs} />
              <div className="space-y-4">
                <CodeBlock
                  label={claudeOs === 'unix' ? 'Terminal' : 'Command Prompt'}
                  content={claudeOs === 'unix' ? claudeEnvUnix : claudeEnvWindows}
                  lang="bash"
                />
                <p className="text-xs text-muted-foreground">{t('docs.clientConfig.claudeEnvNote')}</p>
                <CodeBlock label={`${claudeConfigDir}/settings.json`} content={claudeSettingsJson} lang="json" />
                <p className="text-xs text-muted-foreground">{t('docs.clientConfig.claudeSettingsNote')}</p>
              </div>
            </CardContent>
          </Card>

          <Card id="client-mapping" className="mb-4 scroll-mt-20 py-0">
            <CardContent className="p-6">
              <h3 className="text-base font-semibold text-foreground mb-1">{t('docs.clientConfig.mappingTitle')}</h3>
              <p className="text-sm text-muted-foreground">{t('docs.clientConfig.mappingDesc')}</p>
            </CardContent>
          </Card>

          {/* Section 3: Authentication */}
          <SectionHeader
            id="authentication"
            icon={SECTION_ICON['authentication']}
            tone={SECTION_TONE['authentication']}
            eyebrow={t('docs.section3Eyebrow')}
            title={t('docs.authentication.title')}
            description={t('docs.authentication.description')}
          />

          <Card className="mb-6 scroll-mt-20 py-0">
            <CardContent className="p-6">
              <div className="space-y-2.5">
                <div className="flex items-center gap-2.5 px-3.5 py-2.5 rounded-xl bg-muted/40 border border-border">
                  <Badge variant="outline" className="text-[10px] font-bold shrink-0">Header</Badge>
                  <code className="code-inline">Authorization: Bearer <span className="text-muted-foreground italic">&lt;key&gt;</span></code>
                  <span className="ml-auto text-xs text-muted-foreground">{t('docs.authentication.bearerNote')}</span>
                </div>
                <div className="flex items-center gap-2.5 px-3.5 py-2.5 rounded-xl bg-muted/40 border border-border">
                  <Badge variant="outline" className="text-[10px] font-bold shrink-0">Header</Badge>
                  <code className="code-inline">x-api-key: <span className="text-muted-foreground italic">&lt;key&gt;</span></code>
                  <span className="ml-auto text-xs text-muted-foreground">{t('docs.authentication.xApiKeyNote')}</span>
                </div>
                <div className="flex items-center gap-2.5 px-3.5 py-2.5 rounded-xl bg-muted/40 border border-border">
                  <Badge variant="outline" className="text-[10px] font-bold shrink-0">Header</Badge>
                  <code className="code-inline">X-Admin-Key: <span className="text-muted-foreground italic">&lt;admin_secret&gt;</span></code>
                  <span className="ml-auto text-xs text-muted-foreground">{t('docs.authentication.adminNote')}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Section 4: Model API */}
          <SectionHeader
            id="model-api"
            icon={SECTION_ICON['model-api']}
            tone={SECTION_TONE['model-api']}
            eyebrow={t('docs.section4Eyebrow')}
            title={t('docs.modelApi.title')}
            description={t('docs.modelApi.description')}
          />

          {modelEndpoints.map((endpoint) => (
            <EndpointDoc
              key={endpoint.id}
              id={endpoint.id}
              method={endpoint.method}
              path={endpoint.path}
              title={endpoint.title}
              description={endpoint.description}
              curlExample={endpoint.curl}
              defaultBody={endpoint.defaultBody}
              responseExamples={endpoint.responses}
              apiKey={activeKey}
              baseUrl={baseUrl}
              allKeys={allKeys}
            />
          ))}

          {/* Section 5: Admin API */}
          <SectionHeader
            id="admin-api"
            icon={SECTION_ICON['admin-api']}
            tone={SECTION_TONE['admin-api']}
            eyebrow={t('docs.section5Eyebrow')}
            title={t('docs.adminApi.title')}
            description={t('docs.adminApi.description')}
          />

          {adminEndpoints.map((endpoint) => (
            <EndpointDoc
              key={endpoint.id}
              id={endpoint.id}
              method={endpoint.method}
              path={endpoint.path}
              title={endpoint.title}
              description={endpoint.description}
              curlExample={endpoint.curl}
              defaultBody={endpoint.defaultBody}
              responseExamples={endpoint.responses}
              apiKey={activeKey}
              baseUrl={baseUrl}
              allKeys={allKeys}
            />
          ))}
        </div>

        <DocsTOC items={tocItems} title={t('docs.tocTitle')} />
      </div>
    </>
  )
}
