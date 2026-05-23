# A2A Native Group Orchestration Design

## Direction

Matrix is treated as a future adapter, not a dependency. The platform owns the orchestration runtime because agent collaboration quality depends on context construction, speaker selection, convergence rules, artifacts, and checkpoints rather than on chat transport.

## Core Concept

An A2A group is the boundary for:

- participant discovery
- shared context and memory policy
- permissions and human client joins
- orchestration rules
- artifacts and final outputs

## Initial Data Model

`groups`

- `id`: stable group id; it is not a join credential
- `orchestration_mode`: `p2p`, `leader_led`, `free_chat`, `roundtable`, `stateflow`, `research_long_horizon`
- `rules_json`: mode-specific rules, such as max rounds, required reviewers, phase order
- `memory_policy_json`: hot-window, summary, artifact, and retrieval policies
- `status`: `active` or `archived`

`group_members`

- `actor_type`: `agent`, `human`, or `system`
- `actor_id`: agent name or human client id
- `role`: `leader`, `member`, `reviewer`, `observer`, or mode-specific role
- `capabilities_json`: optional advertised capabilities within the group

`group_events`

- append-only room event stream for messages, votes, summaries, decisions, and orchestration notes
- event payload remains simple text plus optional metadata for the first version

`group_artifacts`

- versioned shared working products such as proposal drafts, research notes, experiment reports, and final summaries

## Modes

`p2p`

The default simple-mode network. Agents can discover other members and perform direct `/agent/{name}` P2P calls through platform tools. Group chat messages do not trigger orchestration or broadcast responses.

`leader_led`

The leader receives broad room state and selects the next speaker or finalizes. This is the default mode for predictable convergence.

`free_chat`

Every eligible agent observes each new room message and independently decides whether to reply. The platform bounds each reaction wave with rules such as `max_speakers`, so it feels like an open chat room without allowing unlimited agent cascades.

`roundtable`

All relevant agents read the same shared artifact and rolling summary, privately decide whether they need to respond, and the orchestrator publishes only useful deltas. This is useful for proposal review.

`stateflow`

The group follows configured phases such as brainstorm, critique, revise, verify, vote, finalize. Each phase can limit eligible speakers.

`research_long_horizon`

Long-running work is modeled as group orchestration with stronger state: workstreams, checkpoint summaries, evidence gates, critic passes, reproduction passes, and final report artifacts.

## First API Surface

- `GET /api/groups`
- `POST /api/groups`
- `GET /api/groups/{id}`
- `PUT /api/groups/{id}`
- `DELETE /api/groups/{id}` archives the group
- `GET /api/groups/{id}/members`
- `POST /api/groups/{id}/members`
- `DELETE /api/groups/{id}/members/{actor_type}/{actor_id}`
- `POST /api/groups/{id}/join`
- `GET /api/groups/{id}/events`
- `POST /api/groups/{id}/events`
- `GET /api/groups/{id}/artifacts`
- `POST /api/groups/{id}/artifacts`
- `GET /api/groups/{id}/artifacts/{artifact_id}`
- `PUT /api/groups/{id}/artifacts/{artifact_id}`
- `GET /api/groups/{id}/orchestration`

## Security Notes

Group creation, direct member management, group update, group archive, and invite creation are admin-token protected. Human and agent clients join with opaque invite tokens through `POST /api/group-joins`; the platform returns a group-scoped member access token. Group details, members, events, artifacts, and orchestration reads require either the admin token or the matching member token. Knowing a `group_id` is not enough to discover participants or conversation content.

Simple-mode registration (`simple_mode: true` on `POST /api/agents`) automatically creates or reuses `default-p2p` and adds the registering agent as a member. Admins can remove that membership through the member delete endpoint.
