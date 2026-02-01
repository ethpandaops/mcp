# Architecture Overview

This document describes the system architecture of the ethpandaops MCP server, including component interactions, data flow, and design patterns.

## Table of Contents

- [High-Level Architecture](#high-level-architecture)
- [Component Overview](#component-overview)
- [Data Flow](#data-flow)
- [Plugin System](#plugin-system)
- [Sandbox Execution](#sandbox-execution)
- [Security Model](#security-model)
- [Deployment Modes](#deployment-modes)
- [Configuration Flow](#configuration-flow)
- [Request Lifecycle](#request-lifecycle)

## High-Level Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   MCP Client    │────▶│  MCP Server      │────▶│  Credential     │
│  (Claude, etc.) │     │  (this project)  │     │  Proxy          │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │                           │
                               ▼                           ▼
                        ┌──────────────────┐     ┌─────────────────┐
                        │  Sandbox         │     │  Datasources    │
                        │  (Docker/gVisor) │     │  (ClickHouse,   │
                        │                  │     │   Prometheus,   │
                        └──────────────────┘     │   Loki)         │
                                                 └─────────────────┘
```

## Component Overview

### MCP Server (`pkg/server/`)

The core server that implements the Model Context Protocol (MCP).

| Component | Purpose |
|-----------|---------|
| `server.go` | Transport handlers (stdio, SSE, HTTP), request routing |
| `builder.go` | Dependency injection, service wiring, lifecycle management |

**Key Responsibilities:**
- Expose MCP tools (`execute_python`, `search_examples`, `search_runbooks`)
- Expose MCP resources (`datasources://`, `networks://`, `examples://`, etc.)
- Manage authentication (GitHub OAuth for HTTP transports)
- Route requests to appropriate handlers

### Plugin System (`pkg/plugin/`, `plugins/`)

Extensible datasource plugins that provide:
- Configuration schemas
- Query examples for semantic search
- Python modules for sandbox execution
- MCP resources (optional)

**Existing Plugins:**

| Plugin | Purpose | Features |
|--------|---------|----------|
| `clickhouse` | Query ClickHouse blockchain data | Schema discovery, table resources |
| `prometheus` | Query Prometheus metrics | Basic queries |
| `loki` | Query Loki logs | Log analysis |
| `dora` | Beacon chain explorer | Network discovery |

### Sandbox (`pkg/sandbox/`)

Secure code execution environment.

| Component | Purpose |
|-----------|---------|
| `sandbox.go` | Service interface definition |
| `docker.go` | Docker-based execution (development) |
| `gvisor.go` | gVisor-based execution (production) |
| `session.go` | Session management for persistent containers |

**Security Features:**
- Container isolation (Docker/gVisor)
- No network access to host
- Credential-free environment
- Resource limits (CPU, memory)
- Timeout enforcement

### Credential Proxy (`pkg/proxy/`)

Trust boundary that holds all datasource credentials.

| Component | Purpose |
|-----------|---------|
| `server.go` | Proxy server that validates JWT tokens |
| `auth.go` | JWT token validation |
| `handlers/*.go` | Per-datasource reverse proxies |

**Key Design:**
- MCP server never holds datasource credentials
- Sandboxes receive only proxy URL + JWT token
- All data access flows through the proxy
- Supports audit logging and rate limiting

### Tools (`pkg/tool/`)

MCP tool implementations.

| Tool | Purpose |
|------|---------|
| `execute_python.go` | Execute Python in sandbox |
| `search_examples.go` | Semantic search over query examples |
| `search_runbooks.go` | Semantic search over runbooks |
| `manage_session.go` | Manage persistent sandbox sessions |

### Resources (`pkg/resource/`)

MCP resource implementations.

| Resource | Purpose |
|----------|---------|
| `datasources.go` | List configured datasources |
| `networks.go` | Ethereum network information |
| `examples.go` | Query example library |
| `api_docs.go` | Python API documentation |
| `getting_started.go` | Getting started guide |

### Semantic Search (`pkg/embedding/`)

GGUF-based embedding model for semantic search.

- Model: All-MiniLM-L6-v2 (Q8_0 quantized)
- Indexes: Example index, runbook index
- Cosine similarity for ranking

## Data Flow

### Query Execution Flow

```
┌─────────┐    ┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ Client  │───▶│ MCP Server  │───▶│   Sandbox   │───▶│    Proxy    │
└─────────┘    └─────────────┘    └─────────────┘    └─────────────┘
     │                │                  │                  │
     │ 1. Tool call   │                  │                  │
     │───────────────▶│                  │                  │
     │                │ 2. Build env     │                  │
     │                │ 3. Inject proxy  │                  │
     │                │    token         │                  │
     │                │─────────────────▶│                  │
     │                │                  │ 4. Execute       │
     │                │                  │    Python        │
     │                │                  │─────────────────▶│
     │                │                  │                  │ 5. Query
     │                │                  │                  │    datasource
     │                │                  │◀─────────────────│
     │                │◀─────────────────│ 6. Return        │
     │◀───────────────│ 7. Return result │    results       │
     │ 8. Tool result │                  │                  │
```

### Schema Discovery Flow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  Plugin     │───▶│   Proxy     │───▶│ ClickHouse  │
│  (Start)    │    │   Client    │    │   Schema    │
└─────────────┘    └─────────────┘    └─────────────┘
       │                  │                  │
       │ 1. Discover      │                  │
       │    datasources   │                  │
       │─────────────────▶│                  │
       │                  │ 2. Query schema  │
       │                  │─────────────────▶│
       │                  │◀─────────────────│ 3. Return
       │◀─────────────────│ 4. Cache schema  │    schema
       │ 5. Register      │                  │
       │    resources     │                  │
```

## Plugin System

### Plugin Lifecycle

```
┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  New()  │───▶│  Init() │───▶│Defaults │───▶│Validate │───▶│ Start() │
└─────────┘    └─────────┘    └─────────┘    └─────────┘    └─────────┘
   Create        Parse          Apply          Check          Async
   instance      config         defaults       config         init
                                                              (optional)
```

### Plugin Interface

```go
type Plugin interface {
    Name() string
    Init(rawConfig []byte) error
    ApplyDefaults()
    Validate() error
    SandboxEnv() (map[string]string, error)
    DatasourceInfo() []types.DatasourceInfo
    Examples() map[string]types.ExampleCategory
    PythonAPIDocs() map[string]types.ModuleDoc
    GettingStartedSnippet() string
    RegisterResources(log logrus.FieldLogger, reg ResourceRegistry) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Builder Pattern

The `Builder` in `pkg/server/builder.go` wires all dependencies:

1. Create plugin registry
2. Create sandbox service
3. Create and start proxy client
4. Inject proxy client into plugins
5. Start plugins (async initialization)
6. Create cartographoor client
7. Create auth service
8. Create embedding model and indexes
9. Create tool and resource registries
10. Create server service

## Sandbox Execution

### Security Model

```
┌─────────────────────────────────────────────────────────────┐
│                      Host System                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Sandbox Container                      │   │
│  │  ┌─────────────────────────────────────────────┐   │   │
│  │  │         Python Process                      │   │   │
│  │  │  - No host network access                   │   │   │
│  │  │  - Read-only root filesystem                │   │   │
│  │  │  - Writable /tmp and /output                │   │   │
│  │  │  - No credentials in environment            │   │   │
│  │  │  - Resource limits enforced                 │   │   │
│  │  └─────────────────────────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Environment Variables

The sandbox receives these environment variables:

| Variable | Source | Purpose |
|----------|--------|---------|
| `ETHPANDAOPS_PROXY_URL` | Config | Proxy server URL |
| `ETHPANDAOPS_PROXY_TOKEN` | Auth service | JWT token for proxy auth |
| `ETHPANDAOPS_*_DATASOURCES` | Plugins | Datasource metadata (no credentials) |
| `ETHPANDAOPS_STORAGE_*` | Config | S3 storage for outputs |

### Execution Flow

1. **Receive request** - `execute_python` tool receives code
2. **Build environment** - Collect env vars from all plugins + platform
3. **Create/attach session** - New container or reuse existing session
4. **Execute code** - Run Python in container with 60s timeout (configurable)
5. **Collect outputs** - stdout, stderr, /output files, metrics
6. **Upload files** - Copy output files to S3 storage
7. **Return results** - Execution result with file URLs

## Security Model

### Credential Isolation

```
┌─────────────────────────────────────────────────────────────┐
│                    Credential Flow                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   ┌──────────┐     ┌──────────┐     ┌──────────┐          │
│   │  Config  │────▶│  Proxy   │────▶│   DB     │          │
│   │  (creds) │     │  Server  │     │ (ClickHouse│          │
│   └──────────┘     └──────────┘     │  etc.)   │          │
│         │                ▲          └──────────┘          │
│         │                │                                 │
│         │           JWT token                             │
│         │                │                                 │
│   ┌──────────┐     ┌──────────┐                          │
│   │  MCP     │────▶│ Sandbox  │                          │
│   │  Server  │env  │ (no creds)│                          │
│   │ (no creds)│     └──────────┘                          │
│   └──────────┘                                            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Key Security Principles

1. **Credential proxy is the sole credential holder**
2. **MCP server never sees datasource credentials**
3. **Sandboxes receive only metadata and proxy tokens**
4. **Proxy validates JWT tokens on every request**
5. **All data access is auditable at the proxy**

## Deployment Modes

### Mode 1: Development (Local)

```
┌─────────────────────────────────────────────┐
│              Local Machine                  │
│  ┌─────────┐  ┌─────────┐  ┌─────────────┐ │
│  │  MCP    │  │  Proxy  │  │   MinIO     │ │
│  │  Server │  │  (none) │  │   (S3)      │ │
│  │  :2480  │  │  :18081 │  │   :2400     │ │
│  └─────────┘  └─────────┘  └─────────────┘ │
│        │            │            │          │
│        └────────────┴────────────┘          │
│                     │                       │
│              ┌─────────────┐                │
│              │   Sandbox   │                │
│              │  (Docker)   │                │
│              └─────────────┘                │
└─────────────────────────────────────────────┘
```

- Proxy runs with `auth.mode: none`
- Direct Docker access
- Local MinIO for S3 storage

### Mode 2: Local Agent (Prod Datasources)

```
┌─────────────────┐         ┌─────────────────────────────┐
│  Local Machine  │◀───────▶│       Production            │
│  ┌─────────┐    │  JWT    │  ┌─────────┐  ┌─────────┐  │
│  │  MCP    │────┘─────────▶│  │  Proxy  │──│   DB    │  │
│  │  Server │    │         │  │  (JWT)  │  │         │  │
│  └─────────┘    │         │  └─────────┘  └─────────┘  │
│       │         │         └─────────────────────────────┘
│       │         │
│  ┌─────────────┐│
│  │   Sandbox   ││
│  │  (Docker)   ││
│  └─────────────┘│
└─────────────────┘
```

- User runs `mcp auth login` to get JWT
- JWT injected into sandboxes
- Sandboxes reach production proxy

### Mode 3: Remote Agent (Hosted)

```
┌─────────────┐     ┌─────────────────────────────────────────┐
│   Client    │────▶│           Production Cluster            │
│  (Browser)  │OAuth│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│             │────▶│  │  MCP    │  │  Proxy  │  │   DB    │ │
└─────────────┘     │  │  Server │  │  (JWT)  │  │         │ │
                    │  │ (OAuth) │  └─────────┘  └─────────┘ │
                    │  └─────────┘         │                  │
                    │        │             │                  │
                    │   ┌─────────────┐    │                  │
                    │   │   Sandbox   │────┘                  │
                    │   │ (gVisor)    │                       │
                    │   └─────────────┘                       │
                    └─────────────────────────────────────────┘
```

- GitHub OAuth for client authentication
- gVisor for stronger isolation
- All components in production network

## Configuration Flow

```yaml
# config.yaml
plugins:
  clickhouse:
    schema_discovery:
      enabled: true
      refresh_interval: 15m
  
  prometheus:
    # No config needed if using proxy

sandbox:
  backend: docker
  timeout: 60

storage:
  endpoint: "${S3_ENDPOINT}"
  # ...

proxy:
  url: "http://localhost:18081"
```

1. **Load config** - YAML parsed with env var substitution
2. **Initialize plugins** - Each plugin receives its config section
3. **Validate** - Plugins validate their configuration
4. **Build environment** - Plugins contribute env vars for sandbox
5. **Start services** - Async initialization of plugins, proxy client, etc.

## Request Lifecycle

### Tool Call Lifecycle

```
┌─────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Start  │────▶│   Parse     │────▶│  Validate   │────▶│  Execute    │
└─────────┘     └─────────────┘     └─────────────┘     └─────────────┘
     │               │                    │                   │
     │ 1. Receive    │ 2. Unmarshal       │ 3. Check          │ 4. Build
     │    request    │    arguments       │    permissions    │    env
     │               │                    │                   │
     │               │                    │                   │ 5. Run
     │               │                    │                   │    sandbox
     │               │                    │                   │
┌─────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   End   │◀────│   Upload    │◀────│   Collect   │◀────│   Return    │
└─────────┘     └─────────────┘     └─────────────┘     └─────────────┘
     ▲               │                    │                   │
     │ 8. Send       │ 7. Upload          │ 6. Gather         │
     │    response   │    files to S3     │    outputs        │
     │               │                    │                   │
```

### Resource Request Lifecycle

```
┌─────────┐     ┌─────────────┐     ┌─────────────┐
│  Start  │────▶│   Parse     │────▶│   Handler   │
└─────────┘     │    URI      │     └─────────────┘
                └─────────────┘            │
                                           │
                                    ┌─────────────┐
                                    │  Generate   │
                                    │   Content   │
                                    └─────────────┘
                                           │
                                    ┌─────────────┐
                                    │   Return    │
                                    │   Response  │
                                    └─────────────┘
```

Resources are static (fixed content) or template-based (dynamic based on URI parameters).

## Code Organization

```
.
├── cmd/
│   ├── mcp/              # Main MCP server CLI
│   │   ├── cmd/          # Cobra commands
│   │   └── main.go
│   └── proxy/            # Credential proxy CLI
│       └── main.go
├── pkg/
│   ├── auth/             # GitHub OAuth authentication
│   ├── config/           # Configuration loading
│   ├── embedding/        # GGUF embedding model
│   ├── plugin/           # Plugin interface and registry
│   ├── proxy/            # Credential proxy client/server
│   ├── resource/         # MCP resources
│   ├── sandbox/          # Sandbox execution backends
│   ├── server/           # MCP server implementation
│   ├── tool/             # MCP tools
│   └── types/            # Shared types
├── plugins/              # Datasource plugins
│   ├── clickhouse/
│   ├── dora/
│   ├── loki/
│   └── prometheus/
├── sandbox/              # Sandbox Docker image
│   ├── Dockerfile
│   └── ethpandaops/      # Python package
├── runbooks/             # Embedded runbooks
└── tests/eval/           # LLM evaluation harness
```

## Related Documentation

- [Plugin Development Guide](./PLUGIN_DEVELOPMENT.md) - Creating new plugins
- [Deployment Modes](./deployments.md) - Detailed deployment configurations
- [Contributing Guidelines](../CONTRIBUTING.md) - Contributing to the project
