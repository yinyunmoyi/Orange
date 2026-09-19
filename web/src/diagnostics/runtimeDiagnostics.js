const MAX_EVENTS = 300
const events = []

export function recordRuntimeDiagnostic(type, data = {}) {
  const event = {
    type,
    at: new Date().toISOString(),
    ...data,
  }
  events.push(event)
  if (events.length > MAX_EVENTS) events.splice(0, events.length - MAX_EVENTS)

  if (
    import.meta.env?.DEV &&
    (type.endsWith(':error') || type.endsWith(':fallback'))
  ) {
    console.warn('[orange diagnostics]', event)
  }
  return event
}

export async function fetchWithDiagnostics(operation, input, init) {
  const startedAt = performance.now()
  const target = requestTarget(input)
  recordRuntimeDiagnostic('api:start', {
    operation,
    method: init?.method || 'GET',
    target,
  })

  try {
    const response = await globalThis.fetch(input, init)
    recordRuntimeDiagnostic('api:response', {
      operation,
      method: init?.method || 'GET',
      target,
      status: response.status,
      ok: response.ok,
      durationMs: elapsed(startedAt),
    })
    return response
  } catch (error) {
    recordRuntimeDiagnostic('api:error', {
      operation,
      method: init?.method || 'GET',
      target,
      durationMs: elapsed(startedAt),
      name: error?.name || 'Error',
      message: error?.message || '',
    })
    throw error
  }
}

export function installRuntimeDiagnostics() {
  if (typeof window === 'undefined' || window.__orangeDiagnostics) return

  window.__orangeDiagnostics = {
    clear() {
      events.splice(0, events.length)
    },
    snapshot() {
      return events.map((event) => ({ ...event }))
    },
  }
  window.addEventListener('error', (event) => {
    recordRuntimeDiagnostic('window:error', {
      message: event.message || '',
      source: event.filename || '',
      line: event.lineno || 0,
    })
  })
  window.addEventListener('unhandledrejection', (event) => {
    recordRuntimeDiagnostic('promise:error', {
      name: event.reason?.name || 'Error',
      message: event.reason?.message || String(event.reason || ''),
    })
  })
}

function requestTarget(input) {
  try {
    return new URL(
      typeof input === 'string' ? input : input.url,
      window.location.origin,
    ).pathname
  } catch {
    return String(input || '')
  }
}

function elapsed(startedAt) {
  return Math.round((performance.now() - startedAt) * 100) / 100
}
