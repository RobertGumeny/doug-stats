import { useState, useEffect, useCallback } from 'react'
import {
  buildQuery,
  deriveDurationLabel,
  formatCost,
  formatDuration,
  formatTimestamp,
  getTextFromContentRaw,
  isToolContentType,
} from './transcript'

interface ProjectItem {
  path: string
  cost_usd: number
  unknown: boolean
}

interface TaskItem {
  task_id: string
  cost_usd: number
  unknown: boolean
}

interface SessionItem {
  id: string
  provider: string
  project_path: string
  task_id: string
  model: string
  class: string
  start_time: string
  cost_usd: number
  unknown: boolean
}

interface MessageContentPart {
  type: string
  raw: unknown
}

interface MessageItem {
  uuid: string
  parent_uuid?: string
  role: string
  model?: string
  timestamp: string
  is_sidechain: boolean
  cost_usd: number
  cost_unknown: boolean
  content: MessageContentPart[]
}

type View = 'connecting' | 'projects' | 'tasks' | 'sessions' | 'transcript'

const KNOWN_PROVIDERS = ['claude', 'gemini', 'codex']
const CACHE_TIER_NOTE = 'Cost assumes the 5-minute cache-read pricing tier for cache_read tokens.'

function CostBadge({ cost_usd, unknown }: { cost_usd: number; unknown: boolean }) {
  return (
    <span className={`text-2xl font-bold tabular-nums ${unknown ? 'text-yellow-400' : 'text-green-400'}`}>
      {formatCost(cost_usd, unknown)}
    </span>
  )
}

function FilterBar({
  providers,
  dougOnly,
  onToggleProvider,
  onClearProviders,
  onToggleDougOnly,
}: {
  providers: string[]
  dougOnly: boolean
  onToggleProvider: (p: string) => void
  onClearProviders: () => void
  onToggleDougOnly: () => void
}) {
  return (
    <div className="flex flex-wrap gap-4 mb-6 items-center bg-gray-800 rounded-lg px-4 py-3">
      <div className="flex items-center gap-2">
        <span className="text-gray-400 text-sm font-medium">Provider:</span>
        {KNOWN_PROVIDERS.map((p) => (
          <button
            key={p}
            onClick={() => onToggleProvider(p)}
            className={`px-3 py-1 rounded text-sm font-medium transition-colors ${
              providers.includes(p)
                ? 'bg-blue-600 text-white'
                : 'bg-gray-700 text-gray-300 hover:bg-gray-600'
            }`}
          >
            {p}
          </button>
        ))}
        {providers.length > 0 && (
          <button
            onClick={onClearProviders}
            className="text-xs text-gray-500 hover:text-gray-300 transition-colors ml-1"
          >
            clear
          </button>
        )}
      </div>
      <label className="flex items-center gap-2 cursor-pointer select-none">
        <input
          type="checkbox"
          checked={dougOnly}
          onChange={onToggleDougOnly}
          className="w-4 h-4 accent-blue-500"
        />
        <span className="text-sm text-gray-300">Doug tasks only</span>
      </label>
    </div>
  )
}

function renderContentPart(
  msg: MessageItem,
  part: MessageContentPart,
  index: number,
  expandedToolParts: Record<string, boolean>,
  onToggleToolPart: (key: string) => void,
) {
  const key = `${msg.uuid}:${index}`

  if (isToolContentType(part.type)) {
    const expanded = !!expandedToolParts[key]
    const label = part.type === 'tool_use' ? 'Tool Use' : 'Tool Result'
    const maybeName =
      typeof part.raw === 'object' && part.raw !== null && 'name' in part.raw
        ? String((part.raw as { name: unknown }).name)
        : undefined

    return (
      <div key={key} className="border border-gray-700 rounded-md bg-gray-900/70 overflow-hidden">
        <button
          onClick={() => onToggleToolPart(key)}
          className="w-full text-left px-3 py-2 bg-gray-800 hover:bg-gray-700 transition-colors flex items-center justify-between"
          aria-expanded={expanded}
        >
          <span className="text-sm font-medium text-cyan-300">
            {label}
            {maybeName ? `: ${maybeName}` : ''}
          </span>
          <span className="text-xs text-gray-400">{expanded ? 'Hide details' : 'Show details'}</span>
        </button>
        {expanded && (
          <pre className="text-xs text-gray-200 p-3 overflow-x-auto whitespace-pre-wrap break-words">
            {JSON.stringify(part.raw, null, 2)}
          </pre>
        )}
      </div>
    )
  }

  const text = getTextFromContentRaw(part.raw)
  if (text !== null) {
    return (
      <div key={key} className="text-sm text-gray-100 whitespace-pre-wrap break-words">
        {text}
      </div>
    )
  }

  return (
    <div key={key} className="border border-gray-700 rounded-md bg-gray-900/70 p-3">
      <div className="text-xs uppercase tracking-wide text-gray-400 mb-2">{part.type || 'content'}</div>
      <pre className="text-xs text-gray-200 overflow-x-auto whitespace-pre-wrap break-words">
        {JSON.stringify(part.raw, null, 2)}
      </pre>
    </div>
  )
}

export default function App() {
  const [view, setView] = useState<View>('connecting')
  const [selectedProject, setSelectedProject] = useState<string | null>(null)
  const [selectedTask, setSelectedTask] = useState<string | null>(null)
  const [selectedSession, setSelectedSession] = useState<SessionItem | null>(null)

  const [providerFilter, setProviderFilter] = useState<string[]>([])
  const [dougOnly, setDougOnly] = useState(false)

  const [projects, setProjects] = useState<ProjectItem[]>([])
  const [tasks, setTasks] = useState<TaskItem[]>([])
  const [sessions, setSessions] = useState<SessionItem[]>([])
  const [messages, setMessages] = useState<MessageItem[]>([])

  const [loadingProjects, setLoadingProjects] = useState(false)
  const [loadingTasks, setLoadingTasks] = useState(false)
  const [loadingSessions, setLoadingSessions] = useState(false)
  const [loadingMessages, setLoadingMessages] = useState(false)

  const [sessionDurations, setSessionDurations] = useState<Record<string, string | null>>({})
  const [expandedToolParts, setExpandedToolParts] = useState<Record<string, boolean>>({})

  useEffect(() => {
    let cancelled = false

    async function poll() {
      while (!cancelled) {
        try {
          const resp = await fetch('/api/health')
          if (resp.ok) {
            const data = await resp.json()
            if (data.status === 'ok') {
              if (!cancelled) setView('projects')
              return
            }
          }
        } catch {
          // server not ready yet
        }
        await new Promise<void>((r) => setTimeout(r, 1000))
      }
    }

    poll()
    return () => {
      cancelled = true
    }
  }, [])

  const fetchProjects = useCallback(async () => {
    setLoadingProjects(true)
    try {
      const q = buildQuery({ provider: providerFilter, doug_only: dougOnly })
      const resp = await fetch(`/api/projects${q}`)
      const data: ProjectItem[] = await resp.json()
      setProjects(data.sort((a, b) => b.cost_usd - a.cost_usd))
    } catch {
      setProjects([])
    }
    setLoadingProjects(false)
  }, [providerFilter, dougOnly])

  const fetchTasks = useCallback(async () => {
    if (!selectedProject) return

    setLoadingTasks(true)
    try {
      const q = buildQuery({ project: selectedProject, provider: providerFilter, doug_only: dougOnly })
      const resp = await fetch(`/api/tasks${q}`)
      const data: TaskItem[] = await resp.json()
      setTasks(data)
    } catch {
      setTasks([])
    }
    setLoadingTasks(false)
  }, [selectedProject, providerFilter, dougOnly])

  const fetchSessions = useCallback(async () => {
    if (!selectedTask) return

    setLoadingSessions(true)
    try {
      const q = buildQuery({ project: selectedProject || undefined, provider: providerFilter, doug_only: dougOnly })
      const resp = await fetch(`/api/sessions?task=${encodeURIComponent(selectedTask)}${q ? '&' + q.slice(1) : ''}`)
      const data: SessionItem[] = await resp.json()
      setSessions(
        data.sort((a, b) => {
          const ta = Date.parse(a.start_time)
          const tb = Date.parse(b.start_time)
          if (Number.isNaN(ta) && Number.isNaN(tb)) return a.id.localeCompare(b.id)
          if (Number.isNaN(ta)) return 1
          if (Number.isNaN(tb)) return -1
          return tb - ta
        }),
      )
    } catch {
      setSessions([])
    }
    setLoadingSessions(false)
  }, [selectedTask, selectedProject, providerFilter, dougOnly])

  const fetchMessages = useCallback(async () => {
    if (!selectedSession) return

    setLoadingMessages(true)
    try {
      const resp = await fetch(`/api/sessions/${encodeURIComponent(selectedSession.id)}/messages`)
      const data: MessageItem[] = await resp.json()
      const withIndex = data.map((msg, index) => ({ msg, index }))
      withIndex.sort((a, b) => {
        const ta = Date.parse(a.msg.timestamp)
        const tb = Date.parse(b.msg.timestamp)
        if (Number.isNaN(ta) && Number.isNaN(tb)) return a.index - b.index
        if (Number.isNaN(ta)) return 1
        if (Number.isNaN(tb)) return -1
        if (ta !== tb) return ta - tb
        return a.index - b.index
      })
      setMessages(withIndex.map((item) => item.msg))
    } catch {
      setMessages([])
    }
    setLoadingMessages(false)
  }, [selectedSession])

  useEffect(() => {
    if (view === 'projects') fetchProjects()
  }, [view, fetchProjects])

  useEffect(() => {
    if (view === 'tasks') fetchTasks()
  }, [view, fetchTasks])

  useEffect(() => {
    if (view === 'sessions') fetchSessions()
  }, [view, fetchSessions])

  useEffect(() => {
    if (view === 'transcript') fetchMessages()
  }, [view, fetchMessages])

  useEffect(() => {
    if (view !== 'sessions' || sessions.length === 0) return

    let cancelled = false

    async function loadDurations() {
      const idsToFetch = sessions
        .map((session) => session.id)
        .filter((id) => !(id in sessionDurations))

      if (idsToFetch.length === 0) return

      const updates: Record<string, string | null> = {}
      await Promise.all(
        idsToFetch.map(async (id) => {
          try {
            const resp = await fetch(`/api/sessions/${encodeURIComponent(id)}/messages`)
            const data: MessageItem[] = await resp.json()
            const durationMs = deriveDurationLabel(data)
            updates[id] = durationMs ? formatDuration(durationMs) : null
          } catch {
            updates[id] = null
          }
        }),
      )

      if (!cancelled) {
        setSessionDurations((prev) => ({ ...prev, ...updates }))
      }
    }

    loadDurations()

    return () => {
      cancelled = true
    }
  }, [view, sessions, sessionDurations])

  function handleProjectClick(path: string) {
    setSelectedProject(path)
    setSelectedTask(null)
    setSelectedSession(null)
    setSessions([])
    setMessages([])
    setView('tasks')
  }

  function handleTaskClick(taskID: string) {
    setSelectedTask(taskID)
    setSelectedSession(null)
    setSessions([])
    setMessages([])
    setView('sessions')
  }

  function handleSessionClick(session: SessionItem) {
    setSelectedSession(session)
    setMessages([])
    setExpandedToolParts({})
    setView('transcript')
  }

  function handleBreadcrumbProjects() {
    setSelectedProject(null)
    setSelectedTask(null)
    setSelectedSession(null)
    setTasks([])
    setSessions([])
    setMessages([])
    setView('projects')
  }

  function handleBreadcrumbTasks() {
    if (!selectedProject) return
    setSelectedTask(null)
    setSelectedSession(null)
    setSessions([])
    setMessages([])
    setView('tasks')
  }

  function handleBreadcrumbSessions() {
    if (!selectedTask) return
    setSelectedSession(null)
    setMessages([])
    setView('sessions')
  }

  function toggleProvider(p: string) {
    setProviderFilter((prev) => (prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p]))
  }

  function toggleToolPart(key: string) {
    setExpandedToolParts((prev) => ({ ...prev, [key]: !prev[key] }))
  }

  if (view === 'connecting') {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin mx-auto mb-4" />
          <div className="text-white text-xl font-medium mb-1">Connecting…</div>
          <div className="text-gray-400 text-sm">Waiting for server to be ready</div>
        </div>
      </div>
    )
  }

  const regularTasks = tasks
    .filter((t) => t.task_id !== 'manual')
    .sort((a, b) => b.cost_usd - a.cost_usd)
  const manualTask = tasks.find((t) => t.task_id === 'manual')

  return (
    <div className="min-h-screen bg-gray-900 text-white">
      <div className="max-w-6xl mx-auto px-4 py-8">
        <div className="mb-6">
          <h1 className="text-3xl font-bold text-white tracking-tight">doug-stats</h1>
          <p className="text-gray-400 text-sm mt-1">AI coding assistant usage &amp; cost</p>
        </div>

        <nav className="mb-4 text-sm" aria-label="breadcrumb">
          {view === 'projects' ? (
            <span className="text-gray-300 font-medium">Projects</span>
          ) : (
            <div className="flex flex-wrap items-center gap-2 text-gray-400">
              <button onClick={handleBreadcrumbProjects} className="hover:text-white transition-colors font-medium">
                Projects
              </button>
              {selectedProject && (
                <>
                  <span>/</span>
                  {view === 'tasks' ? (
                    <span className="text-white font-medium font-mono text-xs break-all">{selectedProject}</span>
                  ) : (
                    <button
                      onClick={handleBreadcrumbTasks}
                      className="hover:text-white transition-colors font-medium font-mono text-xs break-all"
                    >
                      {selectedProject}
                    </button>
                  )}
                </>
              )}
              {selectedTask && (
                <>
                  <span>/</span>
                  {view === 'sessions' ? (
                    <span className="text-white font-medium font-mono text-xs">{selectedTask}</span>
                  ) : (
                    <button
                      onClick={handleBreadcrumbSessions}
                      className="hover:text-white transition-colors font-medium font-mono text-xs"
                    >
                      {selectedTask}
                    </button>
                  )}
                </>
              )}
              {selectedSession && view === 'transcript' && (
                <>
                  <span>/</span>
                  <span className="text-white font-medium font-mono text-xs">{selectedSession.id}</span>
                </>
              )}
            </div>
          )}
        </nav>

        <FilterBar
          providers={providerFilter}
          dougOnly={dougOnly}
          onToggleProvider={toggleProvider}
          onClearProviders={() => setProviderFilter([])}
          onToggleDougOnly={() => setDougOnly((v) => !v)}
        />

        {view === 'projects' && (
          <section>
            <h2 className="text-lg font-semibold text-gray-300 mb-3">
              All Projects
              {!loadingProjects && (
                <span className="ml-2 text-sm font-normal text-gray-500">({projects.length})</span>
              )}
            </h2>
            {loadingProjects ? (
              <div className="text-gray-500 py-8 text-center">Loading…</div>
            ) : projects.length === 0 ? (
              <div className="text-gray-500 py-8 text-center">No projects found.</div>
            ) : (
              <div className="space-y-2">
                {projects.map((p) => (
                  <button
                    key={p.path}
                    onClick={() => handleProjectClick(p.path)}
                    className="w-full text-left bg-gray-800 hover:bg-gray-700 active:bg-gray-600 rounded-lg px-5 py-4 flex justify-between items-center transition-colors"
                  >
                    <span className="text-gray-200 font-mono text-sm break-all mr-4">{p.path}</span>
                    <CostBadge cost_usd={p.cost_usd} unknown={p.unknown} />
                  </button>
                ))}
              </div>
            )}
          </section>
        )}

        {view === 'tasks' && (
          <section>
            <h2 className="text-lg font-semibold text-gray-300 mb-3">
              Tasks
              {!loadingTasks && <span className="ml-2 text-sm font-normal text-gray-500">({tasks.length})</span>}
            </h2>
            {loadingTasks ? (
              <div className="text-gray-500 py-8 text-center">Loading…</div>
            ) : tasks.length === 0 ? (
              <div className="text-gray-500 py-8 text-center">No tasks found.</div>
            ) : (
              <>
                {regularTasks.length > 0 && (
                  <div className="space-y-2">
                    {regularTasks.map((t) => (
                      <button
                        key={t.task_id}
                        onClick={() => handleTaskClick(t.task_id)}
                        className="w-full text-left bg-gray-800 hover:bg-gray-700 active:bg-gray-600 rounded-lg px-5 py-4 flex justify-between items-center transition-colors"
                      >
                        <span className="text-gray-200 font-mono text-sm mr-4">{t.task_id}</span>
                        <CostBadge cost_usd={t.cost_usd} unknown={t.unknown} />
                      </button>
                    ))}
                  </div>
                )}

                {manualTask && (
                  <div className="mt-8">
                    <h3 className="text-base font-semibold text-gray-400 mb-3 flex items-center gap-2">
                      <span>Manual &amp; Untagged Sessions</span>
                      <span className="text-xs font-normal text-gray-600 italic">(excluded from Doug-only view)</span>
                    </h3>
                    <button
                      onClick={() => handleTaskClick(manualTask.task_id)}
                      className="w-full text-left bg-gray-800 border border-gray-700 rounded-lg px-5 py-4 flex justify-between items-center hover:bg-gray-700 active:bg-gray-600 transition-colors"
                    >
                      <div>
                        <span className="text-gray-300 text-sm font-medium">Manual sessions</span>
                        <p className="text-gray-500 text-xs mt-0.5">Includes manually-initiated and untagged sessions</p>
                      </div>
                      <CostBadge cost_usd={manualTask.cost_usd} unknown={manualTask.unknown} />
                    </button>
                  </div>
                )}
              </>
            )}
          </section>
        )}

        {view === 'sessions' && (
          <section>
            <h2 className="text-lg font-semibold text-gray-300 mb-3">
              Sessions
              {!loadingSessions && <span className="ml-2 text-sm font-normal text-gray-500">({sessions.length})</span>}
            </h2>
            {loadingSessions ? (
              <div className="text-gray-500 py-8 text-center">Loading…</div>
            ) : sessions.length === 0 ? (
              <div className="text-gray-500 py-8 text-center">No sessions found.</div>
            ) : (
              <div className="space-y-3">
                {sessions.map((s) => (
                  <button
                    key={s.id}
                    onClick={() => handleSessionClick(s)}
                    className="w-full text-left bg-gray-800 hover:bg-gray-700 active:bg-gray-600 rounded-lg px-5 py-4 transition-colors"
                  >
                    <div className="flex flex-wrap gap-2 items-center mb-3">
                      <span className="font-mono text-xs text-blue-300">{s.id}</span>
                      <span className="px-2 py-0.5 rounded bg-gray-700 text-gray-200 text-xs uppercase">{s.provider}</span>
                      <span className="px-2 py-0.5 rounded bg-gray-700 text-gray-200 text-xs uppercase">{s.class}</span>
                      {s.model && (
                        <span className="px-2 py-0.5 rounded bg-gray-700 text-gray-300 text-xs font-mono">{s.model}</span>
                      )}
                    </div>

                    <div className="flex flex-wrap gap-x-6 gap-y-2 text-xs text-gray-400">
                      <span>Started: {formatTimestamp(s.start_time)}</span>
                      {sessionDurations[s.id] && <span>Duration: {sessionDurations[s.id]}</span>}
                    </div>

                    <div className="mt-3 flex items-center justify-end">
                      <CostBadge cost_usd={s.cost_usd} unknown={s.unknown} />
                    </div>
                  </button>
                ))}
              </div>
            )}
          </section>
        )}

        {view === 'transcript' && (
          <section>
            <div className="flex flex-wrap justify-between gap-4 items-start mb-3">
              <h2 className="text-lg font-semibold text-gray-300">Transcript</h2>
              <p className="text-xs text-gray-500 max-w-md text-right" title={CACHE_TIER_NOTE}>
                Cost note: assumes 5-minute cache-read tier.
              </p>
            </div>

            {loadingMessages ? (
              <div className="text-gray-500 py-8 text-center">Loading…</div>
            ) : messages.length === 0 ? (
              <div className="text-gray-500 py-8 text-center">No transcript messages found.</div>
            ) : (
              <div className="space-y-3">
                {messages.map((msg) => (
                  <article
                    key={msg.uuid}
                    className={`rounded-lg border px-4 py-3 ${
                      msg.role === 'user'
                        ? 'bg-blue-950/30 border-blue-900'
                        : 'bg-gray-800 border-gray-700'
                    } ${msg.is_sidechain ? 'ring-1 ring-amber-500/40' : ''}`}
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold text-white capitalize">{msg.role}</span>
                        {msg.model && <span className="text-xs text-gray-400 font-mono">{msg.model}</span>}
                        {msg.is_sidechain && (
                          <span className="text-xs px-2 py-0.5 rounded bg-amber-800/50 text-amber-200 uppercase">
                            sidechain
                          </span>
                        )}
                        <span className="text-xs text-gray-500">{formatTimestamp(msg.timestamp)}</span>
                      </div>

                      <span className="text-xs text-gray-300" title={CACHE_TIER_NOTE}>
                        Turn cost: {formatCost(msg.cost_usd, msg.cost_unknown)}
                      </span>
                    </div>

                    <div className="space-y-3">
                      {msg.content.map((part, index) =>
                        renderContentPart(msg, part, index, expandedToolParts, toggleToolPart),
                      )}
                    </div>
                  </article>
                ))}
              </div>
            )}
          </section>
        )}
      </div>
    </div>
  )
}
