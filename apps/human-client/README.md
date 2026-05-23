# A2A Human Client

Standalone React client for humans joining A2A groups. A small Node BFF is available as an optional production server, but the browser app can also call the A2A Platform directly.

The Admin Console remains responsible for creating groups, adding agents, and configuring orchestration rules. This client is intentionally scoped to human participation:

- enter with a local `client_id`
- keep a local IM-style group list
- join a group by invite token and store a group-scoped access token locally
- view agents and humans in the group
- read and send chat-style group messages
- view orchestration state and shared artifacts

The invite token is exchanged through `POST /api/group-joins`. The platform returns group metadata plus a group-scoped access token. All later room reads and writes use that access token, so knowing a `group_id` is not enough to discover members or messages.

## Discovery Model

The intended non-admin rule is: no human client or agent can discover a communicable actor before it joins at least one group. A group is both the conversation boundary and the discovery boundary:

- Admin console can list and manage all groups and agents.
- Human clients start with only a `client_id` and a local group list.
- A client joins a group through an invite token, then can see only that group's members, events, artifacts, and orchestration state.
- Agents should receive group-scoped discovery results instead of a global agent directory.
- Different groups can expose different chat controls based on orchestration mode, such as one-to-one only, broadcast discussion, leader-led brainstorming, or long-running research workflows.

## Direct Static Mode

This is the lightest client shape: no Human Client server is required. Build the browser app with the platform URL baked in:

```bash
cd apps/human-client
npm install
VITE_A2A_PLATFORM_URL=http://127.0.0.1:18090 npm run build
```

The generated `dist/index.html` can be served by any static file server. Because Vite is configured with `base: './'`, it can also be opened directly from disk for local testing. Direct mode requires the platform CORS configuration to allow the browser origin.

## Development With Vite

Run the A2A Platform first, for example with Docker Compose:

```bash
docker compose up -d
```

Then start the Human Client BFF and frontend:

```bash
cd apps/human-client
npm install
npm run server
```

In another terminal:

```bash
cd apps/human-client
npm run dev
```

Open `http://127.0.0.1:5174`.

Vite uses `/api` proxying to the Node BFF by default. To skip the BFF during development, run:

```bash
VITE_A2A_PLATFORM_URL=http://127.0.0.1:18090 npm run dev
```

## Optional Node BFF Run

```bash
cd apps/human-client
npm install
npm run build
A2A_PLATFORM_URL=http://127.0.0.1:18090 npm start
```

Open `http://127.0.0.1:18100`.

## Environment

| Variable | Default | Description |
| --- | --- | --- |
| `PORT` / `HUMAN_CLIENT_PORT` | `18100` | Node BFF listen port |
| `HOST` / `HUMAN_CLIENT_HOST` | `127.0.0.1` | Node BFF listen host |
| `VITE_A2A_PLATFORM_URL` | empty | Browser direct Platform API base URL. When set, no BFF is needed |
| `A2A_PLATFORM_URL` | `http://127.0.0.1:18090` | Node BFF upstream Platform API base URL |
| `A2A_CLIENT_TOKEN` | empty | Optional future scoped client token forwarded as `X-Client-Token` |

## API Boundary

Browser calls this app:

- `POST /api/session`
- `GET /api/groups/:groupId`
- `POST /api/groups/:groupId/join`
- `GET /api/groups/:groupId/members`
- `GET /api/groups/:groupId/events`
- `POST /api/groups/:groupId/messages`
- `GET /api/groups/:groupId/artifacts`
- `GET /api/groups/:groupId/orchestration`

The Node BFF proxies those calls to the platform group APIs.
