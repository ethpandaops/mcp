---
name: investigate
description: Debug Ethereum devnet or network issues. Use when diagnosing finality delays, network splits, offline nodes, client bugs, or general network health problems. Drives a multi-phase investigation using Dora, Loki, ClickHouse, and Prometheus via the panda CLI.
argument-hint: <network-name and/or issue description>
user-invocable: false
---

# Investigate Ethereum Network Issues

Systematic debugging of Ethereum devnets and testnets. Covers finality delays, network splits, offline nodes, and client bugs.

**The user MUST specify which network to debug.** If not provided, ask them.

## Step 0: Local or Remote?

First, determine whether this is a **local Kurtosis devnet** or a **remote hosted deployment**. Check both:

```bash
# Check for local Kurtosis enclaves
kurtosis enclave ls 2>/dev/null

# Check for remote datasources via panda
panda datasources --json 2>/dev/null
```

**Routing:**
- If the target network matches a **Kurtosis enclave name** → this is a **local devnet**. Use `panda search runbooks "debug local devnet"` to load the local debugging runbook, then follow that procedure. It covers Kurtosis service discovery, local Dora/Loki detection (localhost:3100), direct CL/EL API queries, and the Kurtosis-specific Loki label schema (`job="kurtosis"`, `kurtosis_service_name`, etc.).
- If the target network is found in **panda datasources** (Dora networks, Loki instances) → this is a **remote deployment**. Continue with Phase 0 below.
- If found in **neither** → stop, tell the user the network was not found in any local enclave or remote datasource.

## Phase 0: Remote Discovery

**Step 1: Discover all available datasources.** Do not assume instance names — they vary between deployments.

```bash
panda datasources --json
```

This returns all configured datasources with their names and types. Note the Loki and Dora instance names for use in later phases.

**Step 2: Find which networks are available across datasources:**

```bash
panda execute --code '
from ethpandaops import dora, loki

# Discover Dora networks
try:
    networks = dora.list_networks()
    print("Dora networks:", [n["name"] for n in networks])
except Exception as e:
    print(f"Dora unavailable: {e}")

# Discover Loki networks across ALL instances
loki_instances = loki.list_datasources()
print(f"Loki instances: {[d["name"] for d in loki_instances]}")
for ds in loki_instances:
    try:
        testnets = loki.get_label_values(ds["name"], "testnet")
        print(f"  {ds[\"name\"]} networks: {testnets}")
    except Exception as e:
        print(f"  {ds[\"name\"]} error: {e}")
'
```

**Step 3: Determine the data profile** for the target network. Use the actual instance names discovered above:

```bash
panda execute --code '
from ethpandaops import dora, loki

network = "<NETWORK>"

# Check Dora
try:
    networks = dora.list_networks()
    has_dora = network in [n["name"] for n in networks]
except Exception:
    has_dora = False

# Check Loki — search ALL instances, not just "ethpandaops"
loki_instance = None
for ds in loki.list_datasources():
    try:
        testnets = loki.get_label_values(ds["name"], "testnet")
        if network in testnets:
            loki_instance = ds["name"]
            break
    except Exception:
        pass

has_loki = loki_instance is not None
print(f"has_dora={has_dora}, has_loki={has_loki}, loki_instance={loki_instance}")
'
```

Use the discovered `loki_instance` name in all subsequent Loki calls (Phases 2+). Do NOT hardcode `"ethpandaops"`.

**Routing:**
- Neither datasource has the network → stop, tell the user
- `has_dora=true` → run Phase 1 (Dora data collection)
- `has_dora=false` → skip Phase 1, Loki becomes primary
- `has_loki=false` → log investigation unavailable

## Phase 1: Dora Data Collection

Skip if `has_dora=false`. Use `panda search runbooks "debug devnet"` for the full procedure.

Collect in a single execution (use `--session` to reuse the container):

```bash
# Create a session for the investigation
panda session create
# Use returned session ID for all subsequent calls

panda execute --session <id> --code '
from ethpandaops import dora
import json

network = "<NETWORK>"

# Network overview
overview = dora.get_network_overview(network)
print("=== Overview ===")
print(json.dumps(overview, indent=2, default=str))

# Check for network splits
splits = dora.get_network_forks(network)
print(f"\n=== Forks: {len(splits)} ===")
print(json.dumps(splits, indent=2, default=str))

# Finality status
epochs_behind = overview["current_epoch"] - overview.get("finalized_epoch", 0)
print(f"\nFinality lag: {epochs_behind} epochs")
if epochs_behind > 2:
    print("WARNING: Finality delayed")
'
```

**Key thresholds:**
- Finality requires >66.7% participation
- Normal lag: 2 epochs (~13 min on mainnet)
- >4 epochs: concern
- >8 epochs: significant issue

If multiple forks detected, the split overrides the investigation timeframe — focus on the divergence point.

## Phase 2: Log Investigation (Loki)

Target specific nodes from Phase 1 findings, or scan broadly if Loki-only. **Use the `loki_instance` name discovered in Phase 0** — do not hardcode `"ethpandaops"`.

```bash
# Discover available labels for the network
panda execute --session <id> --code '
from ethpandaops import loki

network = "<NETWORK>"
loki_instance = "<LOKI_INSTANCE>"  # from Phase 0 discovery
instances = loki.get_label_values(loki_instance, "instance", f'\''{{testnet="{network}"}}'\'')
cl_clients = loki.get_label_values(loki_instance, "ethereum_cl", f'\''{{testnet="{network}"}}'\'')
print(f"Instances: {instances}")
print(f"CL clients: {cl_clients}")
'

# Fetch CL error logs for a specific node
panda execute --session <id> --code '
from ethpandaops import loki

loki_instance = "<LOKI_INSTANCE>"  # from Phase 0 discovery
logs = loki.query(
    loki_instance,
    '\''{{testnet="<NETWORK>", instance="<INSTANCE>"}} |~ "(?i)(CRIT|ERR)"'\'',
    start="now-1h",
    limit=100
)
for entry in logs:
    print(entry)
'
```

**Instance naming:** `<cl_type>-<el_type>-<number>` (e.g. `lighthouse-geth-1`)

**Investigation order:**
1. CL logs first (consensus drives the network)
2. EL logs only if CL logs point to execution issues (engine API errors, payload failures)
3. Broaden from CRIT/ERR → WARN → INFO if inconclusive

**CL/EL diagnostic matrix:**
- Errors only in CL → consensus issue
- CL engine errors + EL errors → execution issue
- Both layers erroring → shared dependency (disk/memory/network)

## Phase 3: Root Cause Analysis

Classify by scope:
- **Single node** — local issue (crash, disk, OOM)
- **Client-specific** — all nodes of one client affected (client bug)
- **Network split** — focus on divergence point
- **Widespread** — infrastructure or consensus rule edge case

Present findings:
1. What is happening (symptoms)
2. Most likely root cause with evidence
3. Affected nodes/clients
4. Dora links for relevant slots/epochs
5. Suggested next steps

## Useful Search Commands

```bash
# Find relevant query examples
panda search examples "attestation participation"
panda search examples "missed slots"
panda search examples "client distribution"
panda search examples "network overview"

# Find relevant runbooks
panda search runbooks "finality delay"
panda search runbooks "debug devnet"
panda search runbooks "slow query"
```

## Notes

- Save intermediate data to `/workspace/` for multi-step analysis
- Use `--session` consistently to avoid container startup overhead
- Use `panda search examples` before writing complex queries from scratch
- Upload charts with `storage.upload()` for shareable URLs
- Generate Dora links with `dora.link_slot()`, `dora.link_epoch()`, `dora.link_validator()`
