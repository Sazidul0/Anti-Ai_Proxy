# System Architecture

## Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Docker Network (antiproxy)                        │
│                                                                             │
│  ┌───────────────……───┐      ┌────────────────┐      ┌──────────────┐      │
│  │   Contestant        │      │ Proxy Gateway  │      │   Internet   │      │
│  │   Browser           │─────▶│ Go :8080       │─────▶│              │      │
│  │                     │      │                │      │  (filtered)  │      │
│  │  proxy=server:8080  │      │  ┌──────────┐  │      └──────────────┘      │
│  └─────────────────────┘      │  │ AI Filter│  │                            │
│                               │  │ Engine   │  │      ┌──────────────┐      │
│                               │  └──────────┘  │      │   CTFd       │      │
│                               │                │      │   :8000      │      │
│                               │  ┌──────────┐  │      │              │      │
│                               │  │ REST API │──│─────▶│  ┌────────┐  │      │
│                               │  │ :8081    │  │      │  │Plugin  │  │      │
│                               │  └──────────┘  │      │  │Anti-AI │  │      │
│                               └────────────────┘      │  └────────┘  │      │
│                                   │       │           └──────────────┘      │
│                                   │       │                  │              │
│                                   ▼       ▼                  ▼              │
│                            ┌─────────┐ ┌───────┐     ┌──────────────┐      │
│                            │  Redis  │ │  PG   │     │ PG (CTFd)    │      │
│                            │  :6379  │ │ :5432 │     │              │      │
│                            └─────────┘ └───────┘     └──────────────┘      │
│                                                                             │
│  ┌─────────────────────┐                                                    │
│  │  Admin Dashboard    │                                                    │
│  │  Next.js :3000      │                                                    │
│  │  ┌───────────────┐  │                                                    │
│  │  │ Overview      │  │                                                    │
│  │  │ Users         │  │ ──── WebSocket + REST ──▶ Proxy API :8081          │
│  │  │ Sessions      │  │                                                    │
│  │  │ Blocked       │  │                                                    │
│  │  │ Health        │  │                                                    │
│  │  └───────────────┘  │                                                    │
│  └─────────────────────┘                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

### Request Flow
1. Contestant browser sends request through proxy (port 8080)
2. Proxy extracts session token from `Proxy-Authorization` header
3. Filter engine checks domain against blocklist, wildcards, and API patterns
4. **If blocked**: Log request, increment suspicion score, return 403
5. **If allowed**: Forward request to target, log request, return response

### HTTPS (CONNECT) Flow
1. Browser sends `CONNECT domain:443` to proxy
2. Proxy checks domain against filter (domain-level only)
3. **If blocked**: Return 403 (tunnel never established)
4. **If allowed**: Establish TCP tunnel, proxy data bidirectionally

### Flag Submission Flow
1. User submits flag to CTFd (`POST /api/v1/challenges/attempt`)
2. CTFd plugin intercepts the request
3. Plugin queries proxy API: `GET /api/sessions/{userId}/active`
4. **If connected**: Allow submission, log metadata
5. **If disconnected**: Reject submission, record anti-cheat event

### Session Lifecycle
1. User authenticates → receives JWT token
2. User connects to proxy → receives session token
3. Client sends periodic heartbeats (every 30s)
4. Server marks session inactive after 60s of no heartbeat
5. Background cleanup goroutine sweeps stale sessions every 30s

## Database Schema

### Tables
- **users**: User accounts with suspicion scores
- **sessions**: Proxy connection sessions with activity tracking
- **request_logs**: Every proxied request (allowed or blocked)
- **flag_submissions**: CTFd flag submission records
- **suspicion_events**: Anti-cheat event log

### Key Relationships
```
users (1) ─── (N) sessions
users (1) ─── (N) request_logs
users (1) ─── (N) suspicion_events
sessions (1) ─── (N) request_logs
sessions (1) ─── (N) flag_submissions
```

## Performance Design

| Component | Strategy | Target |
|-----------|----------|--------|
| Proxy | Goroutine-per-connection | 10K+ concurrent |
| Sessions | Redis cache + PG persistence | Sub-ms lookup |
| Logging | Async batched writes (100/flush) | No request blocking |
| Rate Limit | Redis INCR with TTL | O(1) per check |
| DB Pool | pgxpool (50 max, 5 min) | Connection reuse |
| Cleanup | Background goroutine (30s) | Minimal overhead |
