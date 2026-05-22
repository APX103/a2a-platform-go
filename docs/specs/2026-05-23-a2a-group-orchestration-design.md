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

- `id`: stable group id; human clients can join with this id
- `orchestration_mode`: `leader_led`, `roundtable`, `stateflow`, `research_long_horizon`
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

`leader_led`

The leader receives broad room state and selects the next speaker or finalizes. This is the default mode for predictable convergence.

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
- `POST /api/groups/{id}/join`
- `GET /api/groups/{id}/events`
- `POST /api/groups/{id}/events`
- `GET /api/groups/{id}/artifacts`
- `POST /api/groups/{id}/artifacts`
- `GET /api/groups/{id}/artifacts/{artifact_id}`
- `PUT /api/groups/{id}/artifacts/{artifact_id}`
- `GET /api/groups/{id}/orchestration`

## Security Notes

Group creation, direct member management, group update, group archive, and artifact updates are admin-token protected. Human client joins are intentionally lightweight in this first version: a client joins by `group_id` and `client_id`. A later version should add group invite tokens or scoped client tokens before public deployment.
