import { reactive } from 'vue'

// Hash routes: #/overview, #/devices, #/devices/:id, #/events, #/apps, #/apps/:id
// The selected app travels in the query (?app=) so links are shareable.
export const route = reactive({ name: 'overview', id: '', query: {} })

function parse() {
  const [path, search = ''] = location.hash.replace(/^#\/?/, '').split('?')
  const parts = path.split('/').filter(Boolean).map(decodeURIComponent)
  route.name = ['overview', 'devices', 'events', 'apps'].includes(parts[0]) ? parts[0] : 'overview'
  route.id = parts[1] || ''
  route.query = Object.fromEntries(new URLSearchParams(search))
}

export function href(name, id = '', query = {}) {
  const q = { ...query }
  for (const k of Object.keys(q)) if (q[k] === '' || q[k] == null) delete q[k]
  const search = new URLSearchParams(q).toString()
  return '#/' + name + (id ? '/' + encodeURIComponent(id) : '') + (search ? '?' + search : '')
}

export function go(name, id = '', query = {}) {
  location.hash = href(name, id, query)
}

// Update the query of the current route without adding history noise.
export function setQuery(patch) {
  const next = { ...route.query, ...patch }
  history.replaceState(null, '', href(route.name, route.id, next))
  parse()
}

window.addEventListener('hashchange', parse)
parse()
