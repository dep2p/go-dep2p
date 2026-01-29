# Relay API Reference

**Last Updated**: 2025-12-31  
**Version**: v2.0/v2.1

---

## 1. Configuration API

### 1.1 WithRelayMap

Configure RelayMap (required for v2.0/v2.1)

```go
func WithRelayMap(relayMap *relayif.RelayMap) Option
```

**Parameters**:
- `relayMap`: RelayMap configuration object

**Example**:
```go
relayMap := &relayif.RelayMap{
    Version: "2025.1",
    Entries: []relayif.RelayMapEntry{
        {
            NodeID:   relay1ID,
            Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
            Region:   "AS",
            AuthMode: relayif.AuthModePublic,
        },
        {
            NodeID:   relay2ID,
            Addrs:    []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
            Region:   "NA",
            AuthMode: relayif.AuthModePublic,
        },
    },
}

node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
)
```

### 1.2 WithGeoIP (v2.1)

Configure GeoIP module for region-aware routing

```go
func WithGeoIP(config *geoipif.Config) Option
```

**Parameters**:
- `config`: GeoIP configuration

**Example**:
```go
geoipConfig := &geoipif.Config{
    Enabled: true,
    DBPath:  "/path/to/GeoLite2-City.mmdb",
    CacheSize: 1000,
}

node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
    dep2p.WithGeoIP(geoipConfig),
)
```

---

## 2. RelayMap Data Structures

### 2.1 RelayMap

```go
type RelayMap struct {
    Entries []RelayMapEntry `json:"entries"`  // At least 2 entries
    Version string          `json:"version,omitempty"`
}
```

**Methods**:

- `Validate() error` - Validate configuration

### 2.2 RelayMapEntry

```go
type RelayMapEntry struct {
    NodeID   types.NodeID   `json:"node_id"`           // Required
    Addrs    []string       `json:"addrs"`             // Required, at least one
    Region   string         `json:"region,omitempty"`   // v2.1: Region identifier
    Weight   int            `json:"weight,omitempty"`   // Load balancing weight
    RealmID  types.RealmID  `json:"realm_id,omitempty"` // v2.1: Security realm
    AuthMode AuthMode       `json:"auth_mode,omitempty"` // v2.1: Authentication mode
}
```

**Field Description**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `NodeID` | `types.NodeID` | Yes | Node ID of the Relay server |
| `Addrs` | `[]string` | Yes | Dialable addresses of the Relay server (multiaddr) |
| `Region` | `string` | No | Geographic region identifier (e.g., "AS", "NA", "EU") |
| `Weight` | `int` | No | Load balancing weight (default 100) |
| `RealmID` | `types.RealmID` | No | Associated Realm ID (only valid for realm_psk) |
| `AuthMode` | `AuthMode` | No | Authentication mode ("public" or "realm_psk") |

**AuthMode Constants**:

```go
const (
    AuthModePublic  AuthMode = "public"    // Public Relay
    AuthModeRealmPSK AuthMode = "realm_psk" // Private Relay
)
```

---

## 3. RelayManager API

### 3.1 Getting RelayManager

```go
rm := node.RelayManager()
if rm == nil {
    // Relay client not enabled
    return
}
```

### 3.2 Status Queries

#### HomeRelay

Get current Home Relay

```go
func (rm *RelayManager) HomeRelay() *relayif.ActiveRelay
```

**Returns**: Current Home Relay, `nil` if not established

**Example**:
```go
home := rm.HomeRelay()
if home != nil {
    fmt.Printf("Home Relay: %s, Latency: %v\n",
        home.Entry.NodeID.ShortString(),
        home.Latency)
}
```

#### BackupRelay

Get current Backup Relay

```go
func (rm *RelayManager) BackupRelay() *relayif.ActiveRelay
```

**Returns**: Current Backup Relay, `nil` if not established

#### Status

Get complete RelayManager status

```go
func (rm *RelayManager) Status() *relayif.RelayManagerStatus
```

**Returns**: Contains Home Relay, Backup Relay, health status, etc.

**Example**:
```go
status := rm.Status()
fmt.Printf("Home: %s, Backup: %s, Health: %s\n",
    status.HomeRelay.Entry.NodeID.ShortString(),
    status.BackupRelay.Entry.NodeID.ShortString(),
    status.HealthStatus)
```

### 3.3 v2.1 Enhanced API

#### GetRelayForContext

Get appropriate Relay based on connection context (v2.1)

```go
func (rm *RelayManager) GetRelayForContext(realmID types.RealmID) *relayif.ActiveRelay
```

**Parameters**:
- `realmID`: RealmID of the target connection (empty string for system/non-Realm connections)

**Returns**: Appropriate Relay, `nil` if none available

**Routing Rules**:
- System connections (`realmID == ""`): Only returns `public` Relay
- Realm connections (`realmID != ""`): Only returns `realm_psk` Relay with matching RealmID

**Example**:
```go
// System connection
systemRelay := rm.GetRelayForContext("")
if systemRelay == nil {
    log.Error("no public relay available")
}

// Realm connection
realmRelay := rm.GetRelayForContext("my-realm")
if realmRelay == nil {
    log.Error("no realm relay available for my-realm")
}
```

#### GetCandidates

Get current candidate pool (v2.1)

```go
func (rm *RelayManager) GetCandidates() []*namespace.RelayCandidate
```

**Returns**: All Relay candidates in the current candidate pool

**Example**:
```go
candidates := rm.GetCandidates()
for _, c := range candidates {
    fmt.Printf("Candidate: %s, Source: %s, Region: %s, AuthMode: %s\n",
        c.NodeID.ShortString(),
        c.Source,      // "config" or "dht"
        c.Region,
        c.AuthMode)
}
```

#### SetAllowRealmFallbackToPublic

Set whether to allow Realm connections to fall back to public Relay (v2.1)

```go
func (rm *RelayManager) SetAllowRealmFallbackToPublic(allow bool)
```

**Parameters**:
- `allow`: Whether to allow fallback (default `false`, strict isolation)

**Note**: Enabling fallback may affect security, use with caution

---

## 4. ActiveRelay Data Structure

```go
type ActiveRelay struct {
    Entry       RelayMapEntry
    Reservation Reservation
    Latency     time.Duration
    LastProbe   time.Time
    FailCount   int
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| `Entry` | `RelayMapEntry` | Relay configuration info |
| `Reservation` | `Reservation` | Reservation object |
| `Latency` | `time.Duration` | Current latency |
| `LastProbe` | `time.Time` | Last probe time |
| `FailCount` | `int` | Consecutive failure count |

---

## 5. RelayManagerStatus Data Structure

```go
type RelayManagerStatus struct {
    HomeRelay   *ActiveRelay
    BackupRelay *ActiveRelay
    HealthStatus map[types.NodeID]HealthStatus
    LatencyStats map[types.NodeID]*LatencyStats
}
```

**Field Description**:

| Field | Type | Description |
|-------|------|-------------|
| `HomeRelay` | `*ActiveRelay` | Current Home Relay |
| `BackupRelay` | `*ActiveRelay` | Current Backup Relay |
| `HealthStatus` | `map[types.NodeID]HealthStatus` | Health status of each Relay |
| `LatencyStats` | `map[types.NodeID]*LatencyStats` | Latency statistics of each Relay |

---

## 6. HealthStatus Enum

```go
type HealthStatus int

const (
    HealthUnknown HealthStatus = iota
    HealthGood                 // RTT < 200ms, no failures
    HealthDegraded             // RTT 200-500ms or minor failures
    HealthBad                  // RTT > 500ms or consecutive failures
    HealthUnreachable          // Completely unreachable
)
```

---

## 7. RelayCandidate Data Structure (v2.1)

```go
type RelayCandidate struct {
    NodeID       types.NodeID
    Addrs        []string
    Region       string
    AuthMode     string
    RealmID      types.RealmID
    Source       string        // "config" / "dht" / "cache"
    DiscoveredAt time.Time
    ExpiresAt    time.Time
}
```

**Methods**:

- `IsExpired() bool` - Check if expired
- `Key() string` - Generate unique key (for deduplication)

---

## 8. Error Handling

### 8.1 Common Errors

| Error | Description | Resolution |
|-------|-------------|------------|
| `ErrRelayMapEmpty` | RelayMap is empty | Provide at least 2 Relay entries |
| `ErrAllRelayProbesFailed` | All Relay probes failed | Check network connection and Relay addresses |
| `ErrRelayMapInvalid` | Invalid RelayMap configuration | Check NodeID and address format |

### 8.2 Error Example

```go
node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
)
if err != nil {
    if errors.Is(err, relayif.ErrRelayMapEmpty) {
        log.Fatal("RelayMap must contain at least 2 Relays")
    }
    if errors.Is(err, relayif.ErrAllRelayProbesFailed) {
        log.Fatal("All Relays unreachable, please check network connection")
    }
    log.Fatalf("Startup failed: %v", err)
}
```

---

## 9. Complete Examples

### 9.1 Basic Configuration

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dep2p/go-dep2p"
    relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()

    // Configure RelayMap
    relayMap := &relayif.RelayMap{
        Version: "2025.1",
        Entries: []relayif.RelayMapEntry{
            {
                NodeID:   relay1ID,
                Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModePublic,
            },
            {
                NodeID:   relay2ID,
                Addrs:    []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
                Region:   "NA",
                AuthMode: relayif.AuthModePublic,
            },
        },
    }

    // Create node
    node, err := dep2p.New(
        dep2p.WithRelayMap(relayMap),
    )
    if err != nil {
        log.Fatalf("Startup failed: %v", err)
    }
    defer node.Close()

    // Access RelayManager
    rm := node.RelayManager()
    if rm == nil {
        log.Fatal("RelayManager not enabled")
    }

    // Query status
    status := rm.Status()
    fmt.Printf("Home Relay: %s, Latency: %v\n",
        status.HomeRelay.Entry.NodeID.ShortString(),
        status.HomeRelay.Latency)
}
```

### 9.2 v2.1 Enhanced Configuration

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dep2p/go-dep2p"
    geoipif "github.com/dep2p/go-dep2p/pkg/interfaces/geoip"
    relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()

    // Configure RelayMap
    relayMap := &relayif.RelayMap{
        Version: "2025.1",
        Entries: []relayif.RelayMapEntry{
            // Public Relay
            {
                NodeID:   publicRelayID,
                Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModePublic,
            },
            // Private Relay
            {
                NodeID:   realmRelayID,
                Addrs:    []string{"/ip4/10.0.0.1/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModeRealmPSK,
                RealmID:  "my-realm",
            },
        },
    }

    // Configure GeoIP
    geoipConfig := &geoipif.Config{
        Enabled: true,
        DBPath:  "/path/to/GeoLite2-City.mmdb",
    }

    // Create node
    node, err := dep2p.New(
        dep2p.WithRelayMap(relayMap),
        dep2p.WithGeoIP(geoipConfig),
    )
    if err != nil {
        log.Fatalf("Startup failed: %v", err)
    }
    defer node.Close()

    // Access RelayManager
    rm := node.RelayManager()

    // View candidate pool
    candidates := rm.GetCandidates()
    fmt.Printf("Candidate pool size: %d\n", len(candidates))
    for _, c := range candidates {
        fmt.Printf("  - %s (source: %s, region: %s)\n",
            c.NodeID.ShortString(), c.Source, c.Region)
    }

    // Get Relay for connection
    systemRelay := rm.GetRelayForContext("")
    realmRelay := rm.GetRelayForContext("my-realm")

    fmt.Printf("System connection Relay: %s\n", systemRelay.Entry.NodeID.ShortString())
    fmt.Printf("Realm connection Relay: %s\n", realmRelay.Entry.NodeID.ShortString())
}
```

---

## 10. Reference Documentation

- [Usage Guide](../how-to/use-relay.md) - How to configure and use Relay
- [Architecture Design](../../design/architecture/relay-v2.md) - Detailed architecture design
- [Concept Documentation](../concepts/relay-architecture.md) - Relay architecture concepts
