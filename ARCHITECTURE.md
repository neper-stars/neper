# Neper Architecture

## WebSocket Notifications System

The notification system pushes real-time updates to clients when database resources are modified. Clients receive minimal notifications (resource type, ID, action, timestamp) and can decide whether to refetch.

### Architecture

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Handler   │────▶│  NATS (internal)│────▶│  All Servers    │
│ (publishes) │     │                 │     │                 │
└─────────────┘     └─────────────────┘     └────────┬────────┘
                                                     │
                                            Each server filters
                                            for its own clients
                                                     │
                                                     ▼
                                            ┌────────────────-─┐
                                            │ WebSocket Client │
                                            │ (only sees what  │
                                            │  they can access)│
                                            └────────────────-─┘
```

NATS is an internal message bus between server components - it's not exposed to clients. Each server instance:

1. Receives all resource change notifications via NATS
2. Filters them per-client based on access control checks
3. Only sends relevant notifications over each user's WebSocket

### Message Format

```json
{
  "type": "session",
  "id": "abc123",
  "action": "updated",
  "timestamp": 1702400000
}
```

**Actions**: `created`, `updated`, `deleted`

**Types**: `session`, `invitation`, `race`, `ruleset`, `session_player_race`

### Access Control

Users only receive notifications for resources they can access:

| Resource Type     | Access Rule                                       |
|-------------------|---------------------------------------------------|
| Session           | User is a member/manager OR session is public     |
| Invitation        | Invitation is addressed to this user              |
| Race              | Race belongs to this user                         |
| Ruleset           | User can access the session that owns the ruleset |
| SessionPlayerRace | User is a member of the session                   |

Global managers can see all notifications.

### Key Files

- `lib/notify/service.go` - NotifyService that publishes changes to NATS
- `lib/notify/subjects.go` - NATS subject constants
- `restapi/handlers/notifications_responder.go` - WebSocket responder with access filtering
- `restapi/handlers/notifications_get.go` - HTTP handler that upgrades to WebSocket
