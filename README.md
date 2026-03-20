# CTF Anti-AI Proxy Gateway

A production-ready proxy system that prevents CTF contestants from using AI tools while solving challenges. The system blocks AI websites and APIs, tracks user sessions, integrates with CTFd for flag submission verification, and provides a real-time admin dashboard.

## Architecture

```
User Browser  →  Proxy Gateway  →  AI Filter Engine  →  Internet  →  CTFd
                       │
              Admin Dashboard (Next.js)
                       │
              Backend API (Go)
                       │
              PostgreSQL + Redis
```

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Git

### 1. Clone & Configure

```bash
git clone https://github.com/Sazidul0/Anti-Ai_Proxy.git
cd Anti_AI_Proxy
cp .env.example .env
# Edit .env with your secrets
```

### 2. Start All Services

```bash
docker compose up -d
```

This starts 6 services:
| Service | Port | Description |
|---------|------|-------------|
| `proxy` | 8080 | HTTP/HTTPS Proxy |
| `proxy` | 8081 | REST API |
| `ctfd` | 8000 | CTFd Platform |
| `dashboard` | 3000 | Admin Dashboard |
| `postgres` | 5432 | PostgreSQL |
| `redis` | 6379 | Redis |

### 3. Configure Browsers

Contestants must configure their browser proxy settings:
- **HTTP Proxy**: `<server-ip>:8080`
- **HTTPS Proxy**: `<server-ip>:8080`

### 4. Access Dashboard

Open `http://<server-ip>:3000` and login with:
- **Username**: `admin`
- **Password**: `admin`

> ⚠️ Change the default admin password immediately!

## User Flow

1. **Registration**: User registers via API (`POST /api/auth/register`)
2. **Connect**: User authenticates and gets a session (`POST /api/proxy/connect`)
3. **Browse**: All traffic flows through the proxy, AI domains are blocked
4. **Heartbeat**: Client sends periodic heartbeats to keep session alive
5. **Submit Flags**: CTFd verifies proxy connection before accepting flags

## Proxy Configuration

### Blocked AI Domains (default)
- openai.com, chat.openai.com, api.openai.com
- claude.ai, anthropic.com
- perplexity.ai, poe.com
- huggingface.co, replicate.com, phind.com
- deepseek.com, groq.com, mistral.ai
- copilot.microsoft.com, cursor.sh, v0.dev, bolt.new
- And more (45+ domains)

### Wildcard Blocking
- All `.ai` TLD domains are blocked

### API Pattern Detection
- `/v1/chat/completions`
- `/v1/completions`
- `/generate`
- `/api/chat`
- And more

### Dynamic Management
Add/remove domains via the API:
```bash
# Add domain
curl -X POST http://localhost:8081/api/filter/domains \
  -H "Authorization: Bearer <token>" \
  -d '{"domain":"newdomain.com"}'

# Remove domain
curl -X DELETE http://localhost:8081/api/filter/domains/newdomain.com \
  -H "Authorization: Bearer <token>"
```

## Anti-Cheat System

The suspicion scoring system tracks:

| Event | Score |
|-------|-------|
| AI domain access attempt | +10 |
| Proxy disconnect before flag | +20 |
| Multiple blocked domains | +5 |

Users exceeding a score of **50** are flagged as suspicious.

## CTFd Plugin

The plugin is automatically mounted into CTFd via Docker. It:
- Intercepts flag submission requests
- Verifies the user has an active proxy session
- Rejects submissions from disconnected users
- Logs all submission metadata

Configure in CTFd Admin → Plugins → Anti-AI Proxy.

## API Endpoints

### Public
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/auth/login` | Login |
| `POST` | `/api/auth/register` | Register user |
| `POST` | `/api/proxy/connect` | Start proxy session |
| `POST` | `/api/proxy/heartbeat` | Session keepalive |
| `POST` | `/api/proxy/disconnect` | End session |

### Admin (JWT required)
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/users` | List all users |
| `GET` | `/api/sessions` | Active sessions |
| `GET` | `/api/blocked-requests` | Blocked request log |
| `GET` | `/api/stats` | System statistics |
| `GET` | `/api/alerts` | Suspicion alerts |
| `GET` | `/api/filter/domains` | Filter rules |
| `GET` | `/api/ws` | WebSocket real-time |

### CTFd Integration (API secret required)
| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/sessions/:userId/active` | Check user proxy status |
| `POST` | `/api/flag-submissions` | Record submission |

## Scaling

The system is designed for **1000+ concurrent users**:
- Go proxy uses goroutine-per-connection (efficient for 10K+ connections)
- PostgreSQL connection pool (50 max connections)
- Redis caching for sessions and rate limiting
- Batched log writes (100 entries per flush)
- All services are Docker-containerized for horizontal scaling

## Development

### Go Proxy
```bash
cd proxy
go run ./cmd/proxy
```

### Dashboard
```bash
cd dashboard
npm run dev
```

### Run Tests
```bash
cd proxy
go test ./internal/filter/ -v
```

## License

MIT
