import { useState, useEffect, useCallback } from 'react'

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

type View = 'connecting' | 'projects' | 'tasks'

const KNOWN_PROVIDERS = ['claude', 'gemini', 'codex']

function buildQuery(params: { provider?: string[]; doug_only?: boolean; project?: string }): string {
  const parts: string[] = []
  if (params.provider && params.provider.length > 0) {
    for (const p of params.provider) {
      parts.push(`provider=${encodeURIComponent(p)}`)
    }
  }
  if (params.doug_only) {
    parts.push('doug_only=true')
  }
  if (params.project) {
    parts.push(`project=${encodeURIComponent(params.project)}`)
  }
  return parts.length ? '?' + parts.join('&') : ''
}

function formatCost(cost: number, unknown: boolean): string {
  if (unknown) return '?'
  return `$${cost.toFixed(4)}`
}

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

export default function App() {
  const [view, setView] = useState<View>('connecting')
  const [selectedProject, setSelectedProject] = useState<string | null>(null)
  const [providerFilter, setProviderFilter] = useState<string[]>([])
  const [dougOnly, setDougOnly] = useState(false)
  const [projects, setProjects] = useState<ProjectItem[]>([])
  const [tasks, setTasks] = useState<TaskItem[]>([])
  const [loadingProjects, setLoadingProjects] = useState(false)
  const [loadingTasks, setLoadingTasks] = useState(false)

  // Poll /api/health until the server is ready
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

  useEffect(() => {
    if (view === 'projects') fetchProjects()
  }, [view, fetchProjects])

  useEffect(() => {
    if (view === 'tasks') fetchTasks()
  }, [view, fetchTasks])

  function handleProjectClick(path: string) {
    setSelectedProject(path)
    setView('tasks')
  }

  function handleBreadcrumbProjects() {
    setSelectedProject(null)
    setView('projects')
  }

  function toggleProvider(p: string) {
    setProviderFilter((prev) => (prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p]))
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
      <div className="max-w-5xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-6">
          <h1 className="text-3xl font-bold text-white tracking-tight">doug-stats</h1>
          <p className="text-gray-400 text-sm mt-1">AI coding assistant usage &amp; cost</p>
        </div>

        {/* Breadcrumb */}
        <nav className="mb-4 text-sm" aria-label="breadcrumb">
          {view === 'projects' ? (
            <span className="text-gray-300 font-medium">Projects</span>
          ) : (
            <div className="flex items-center gap-2 text-gray-400">
              <button
                onClick={handleBreadcrumbProjects}
                className="hover:text-white transition-colors font-medium"
              >
                Projects
              </button>
              <span>/</span>
              <span className="text-white font-medium font-mono text-xs break-all">
                {selectedProject}
              </span>
            </div>
          )}
        </nav>

        {/* Filters */}
        <FilterBar
          providers={providerFilter}
          dougOnly={dougOnly}
          onToggleProvider={toggleProvider}
          onClearProviders={() => setProviderFilter([])}
          onToggleDougOnly={() => setDougOnly((v) => !v)}
        />

        {/* Project List */}
        {view === 'projects' && (
          <section>
            <h2 className="text-lg font-semibold text-gray-300 mb-3">
              All Projects
              {!loadingProjects && (
                <span className="ml-2 text-sm font-normal text-gray-500">
                  ({projects.length})
                </span>
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
                    <span className="text-gray-200 font-mono text-sm break-all mr-4">
                      {p.path}
                    </span>
                    <CostBadge cost_usd={p.cost_usd} unknown={p.unknown} />
                  </button>
                ))}
              </div>
            )}
          </section>
        )}

        {/* Task List */}
        {view === 'tasks' && (
          <section>
            <h2 className="text-lg font-semibold text-gray-300 mb-3">
              Tasks
              {!loadingTasks && (
                <span className="ml-2 text-sm font-normal text-gray-500">
                  ({regularTasks.length})
                </span>
              )}
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
                      <div
                        key={t.task_id}
                        className="bg-gray-800 rounded-lg px-5 py-4 flex justify-between items-center"
                      >
                        <span className="text-gray-200 font-mono text-sm mr-4">{t.task_id}</span>
                        <CostBadge cost_usd={t.cost_usd} unknown={t.unknown} />
                      </div>
                    ))}
                  </div>
                )}

                {manualTask && (
                  <div className="mt-8">
                    <h3 className="text-base font-semibold text-gray-400 mb-3 flex items-center gap-2">
                      <span>Manual &amp; Untagged Sessions</span>
                      <span className="text-xs font-normal text-gray-600 italic">
                        (excluded from Doug-only view)
                      </span>
                    </h3>
                    <div className="bg-gray-800 border border-gray-700 rounded-lg px-5 py-4 flex justify-between items-center">
                      <div>
                        <span className="text-gray-300 text-sm font-medium">Manual sessions</span>
                        <p className="text-gray-500 text-xs mt-0.5">
                          Includes manually-initiated and untagged sessions
                        </p>
                      </div>
                      <CostBadge cost_usd={manualTask.cost_usd} unknown={manualTask.unknown} />
                    </div>
                  </div>
                )}
              </>
            )}
          </section>
        )}
      </div>
    </div>
  )
}
