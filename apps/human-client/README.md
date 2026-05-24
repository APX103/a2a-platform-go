# A2A Human Client

Standalone React client for humans joining A2A groups. A small Node BFF is available as an optional production server, but the browser app can also call the A2A Platform directly.

The Admin Console remains responsible for creating groups, adding agents, and configuring orchestration rules. This client is intentionally scoped to human participation:

- register with a globally unique human `handle` and display name
- log in with either the unique `handle` or the issued human token
- keep a local IM-style group list
- start in the default `default-p2p` group automatically
- join additional groups by invite token plus a human token
- open direct P2P chats with agents from the participant list
- store group-scoped access tokens locally after a successful join
- view agents and humans in the group
- read and send chat-style group messages
- view orchestration state and shared artifacts

The human identity is created through `POST /api/humans/register` with a unique `handle` and `display_name`. Login goes through `POST /api/humans/login` with either that unique handle or a previously issued human token. Registration and login both issue a fresh human token plus a group-scoped access token for the default `default-p2p` group. Invite tokens for additional groups are exchanged through `POST /api/group-joins` with the human token in `Authorization: Bearer ...`. The platform derives the real `human_id` from that token instead of trusting a browser-supplied actor id. All room reads and writes use group-scoped access tokens, so knowing a `group_id` is not enough to discover members or messages.

## Discovery Model

The intended non-admin rule is: no human client or agent can discover a communicable actor before it joins at least one group. A group is both the conversation boundary and the discovery boundary:

- Admin console can list and manage all groups and agents.
- Human clients start with a `human_id`, unique `handle`, display name, optionally saved human token, default group membership, and a local group list.
- A client can join additional groups through an invite token authenticated by the human token, then can see only that group's members, events, artifacts, and orchestration state.
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

Vite uses `/api` proxying to the platform at `http://127.0.0.1:18090` by default, so the Node BFF is optional during local development. To point the browser app at another platform API base URL, run:

```bash
VITE_A2A_PLATFORM_URL=http://127.0.0.1:18090 npm run dev
```

To develop through the Node BFF instead, run the BFF and set:

```bash
HUMAN_CLIENT_BFF=http://127.0.0.1:18100 npm run dev
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

- `POST /api/humans/register`
- `POST /api/humans/login`
- `GET /api/humans/me`
- `POST /api/group-joins`
- `GET /api/groups/:groupId`
- `GET /api/groups/:groupId/members`
- `GET /api/groups/:groupId/events`
- `POST /api/groups/:groupId/messages`
- `GET /api/groups/:groupId/artifacts`
- `GET /api/groups/:groupId/orchestration`
- `POST /agent/:agentName`

The Node BFF proxies those calls to the platform group APIs and A2A agent proxy. `POST /api/session` still exists only as a legacy local echo endpoint for older static clients; the current UI does not use it.
