import http from 'node:http'
import { createReadStream, existsSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const rootDir = path.resolve(__dirname, '..')
const distDir = path.join(rootDir, 'dist')

const port = Number(process.env.PORT || process.env.HUMAN_CLIENT_PORT || 18100)
const host = process.env.HOST || process.env.HUMAN_CLIENT_HOST || '127.0.0.1'
const platformBase = trimSlash(process.env.A2A_PLATFORM_URL || 'http://127.0.0.1:18090')
const clientToken = process.env.A2A_CLIENT_TOKEN || ''

const server = http.createServer(async (req, res) => {
  try {
    if (!req.url) {
      sendJSON(res, 400, { error: 'missing url' })
      return
    }

    const url = new URL(req.url, `http://${req.headers.host || `${host}:${port}`}`)
    if (url.pathname === '/health') {
      sendJSON(res, 200, { status: 'ok', platform: platformBase })
      return
    }

    if (url.pathname.startsWith('/api/')) {
      await handleAPI(req, res, url)
      return
    }

    await serveStatic(res, url.pathname)
  } catch (error) {
    sendJSON(res, 500, { error: error instanceof Error ? error.message : 'internal error' })
  }
})

server.listen(port, host, () => {
  console.log(`A2A Human Client listening on http://${host}:${port}`)
  console.log(`Proxying platform API from ${platformBase}`)
})

async function handleAPI(req, res, url) {
  if (url.pathname === '/api/config' && req.method === 'GET') {
    sendJSON(res, 200, { platform_url: platformBase })
    return
  }

  if (url.pathname === '/api/session' && req.method === 'GET') {
    sendJSON(res, 200, { ok: true })
    return
  }

  if (url.pathname === '/api/session' && req.method === 'POST') {
    const body = await readJSON(req)
    const clientId = String(body.client_id || '').trim()
    if (!clientId) {
      sendJSON(res, 400, { error: 'client_id is required' })
      return
    }
    sendJSON(res, 200, { client_id: clientId })
    return
  }

  const match = url.pathname.match(/^\/api\/groups\/([^/]+)(?:\/([^/]+))?$/)
  if (!match) {
    sendJSON(res, 404, { error: 'not found' })
    return
  }

  const groupId = encodeURIComponent(match[1])
  const action = match[2] || ''

  if (!action && req.method === 'GET') {
    await proxyPlatform(req, res, `/api/groups/${groupId}`)
    return
  }

  if (action === 'join' && req.method === 'POST') {
    await proxyPlatform(req, res, `/api/groups/${groupId}/join`)
    return
  }

  if (action === 'members' && req.method === 'GET') {
    await proxyPlatform(req, res, `/api/groups/${groupId}/members`)
    return
  }

  if (action === 'events' && req.method === 'GET') {
    await proxyPlatform(req, res, `/api/groups/${groupId}/events${url.search}`)
    return
  }

  if (action === 'messages' && req.method === 'POST') {
    const body = await readJSON(req)
    const payload = {
      event_type: 'message',
      sender_type: body.sender_type || 'human',
      sender_id: body.sender_id,
      content: body.content,
      metadata: body.metadata,
    }
    if (!payload.sender_id || !payload.content) {
      sendJSON(res, 400, { error: 'sender_id and content are required' })
      return
    }
    await proxyPlatformJSON(res, `/api/groups/${groupId}/events`, 'POST', payload)
    return
  }

  if (action === 'artifacts' && req.method === 'GET') {
    await proxyPlatform(req, res, `/api/groups/${groupId}/artifacts`)
    return
  }

  if (action === 'orchestration' && req.method === 'GET') {
    await proxyPlatform(req, res, `/api/groups/${groupId}/orchestration`)
    return
  }

  sendJSON(res, 405, { error: 'method not allowed' })
}

async function proxyPlatform(req, res, platformPath) {
  const body = req.method === 'GET' || req.method === 'HEAD' ? undefined : await readRaw(req)
  const headers = {
    'content-type': req.headers['content-type'] || 'application/json',
  }
  if (clientToken) {
    headers['x-client-token'] = clientToken
  }

  const upstream = await fetch(`${platformBase}${platformPath}`, {
    method: req.method,
    headers,
    body,
  })
  await forwardResponse(res, upstream)
}

async function proxyPlatformJSON(res, platformPath, method, payload) {
  const headers = { 'content-type': 'application/json' }
  if (clientToken) {
    headers['x-client-token'] = clientToken
  }
  const upstream = await fetch(`${platformBase}${platformPath}`, {
    method,
    headers,
    body: JSON.stringify(payload),
  })
  await forwardResponse(res, upstream)
}

async function forwardResponse(res, upstream) {
  const text = await upstream.text()
  res.statusCode = upstream.status
  res.setHeader('content-type', upstream.headers.get('content-type') || 'application/json; charset=utf-8')
  res.end(text)
}

async function serveStatic(res, pathname) {
  let filePath = path.join(distDir, decodeURIComponent(pathname))
  if (pathname === '/' || !existsSync(filePath) || !filePath.startsWith(distDir)) {
    filePath = path.join(distDir, 'index.html')
  }
  if (!existsSync(filePath)) {
    sendHTML(res, 200, fallbackHTML())
    return
  }
  res.statusCode = 200
  res.setHeader('content-type', contentType(filePath))
  createReadStream(filePath).pipe(res)
}

async function readJSON(req) {
  const raw = await readRaw(req)
  if (!raw) return {}
  return JSON.parse(raw)
}

async function readRaw(req) {
  const chunks = []
  for await (const chunk of req) {
    chunks.push(chunk)
  }
  return Buffer.concat(chunks).toString('utf8')
}

function sendJSON(res, status, body) {
  res.statusCode = status
  res.setHeader('content-type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(body))
}

function sendHTML(res, status, html) {
  res.statusCode = status
  res.setHeader('content-type', 'text/html; charset=utf-8')
  res.end(html)
}

function trimSlash(value) {
  return value.endsWith('/') ? value.slice(0, -1) : value
}

function contentType(filePath) {
  if (filePath.endsWith('.js')) return 'text/javascript; charset=utf-8'
  if (filePath.endsWith('.css')) return 'text/css; charset=utf-8'
  if (filePath.endsWith('.svg')) return 'image/svg+xml'
  if (filePath.endsWith('.png')) return 'image/png'
  return 'text/html; charset=utf-8'
}

function fallbackHTML() {
  return `<!doctype html>
<html lang="en">
  <head><meta charset="utf-8"><title>A2A Human Client</title></head>
  <body style="font-family: system-ui; margin: 40px">
    <h1>A2A Human Client</h1>
    <p>Run <code>npm run build</code> before <code>npm start</code>, or use <code>npm run dev</code> for the Vite frontend.</p>
  </body>
</html>`
}
