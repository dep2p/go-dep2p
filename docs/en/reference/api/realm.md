# Realm API

RealmManager provides Realm (business isolation domain) management functionality, implementing multi-tenant isolation.

---

## Overview

```mermaid
flowchart TB
    subgraph RealmManager [RealmManager]
        JoinRealm["JoinRealm()"]
        LeaveRealm["LeaveRealm()"]
        CurrentRealm["CurrentRealm()"]
        IsMember["IsMember()"]
        RealmPeers["RealmPeers()"]
    end
    
    subgraph RealmTypes [Realm Types]
        Public["Public"]
        Protected["Protected"]
        Private["Private"]
    end
    
    RealmManager --> RealmTypes
```

Realm is the core mechanism for DeP2P business isolation:
- Shares underlying infrastructure (DHT, relay, NAT traversal)
- Business layer is completely isolated (different Realms are invisible to each other)
- Strict single-Realm model (a node can only join one business Realm at a time)

---

## Getting RealmManager

Obtain through Node's `Realm()` method:

```go
realmMgr := node.Realm()
```

---

## Realm Membership Management APIs

### JoinRealm

Joins a specified Realm.

```go
func (m RealmManager) JoinRealm(ctx context.Context, realmID types.RealmID, opts ...JoinOption) error
```

**Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `ctx` | `context.Context` | Context |
| `realmID` | `types.RealmID` | Realm ID |
| `opts` | `...JoinOption` | Join options |

**Returns**:
| Type | Description |
|------|-------------|
| `error` | Error information |

**Notes**:
- For Protected/Private Realms, JoinKey is required
- If already joined another Realm, returns `ErrAlreadyJoined`
- Must call `LeaveRealm()` first to leave current Realm

**Example**:

```go
// Join Public Realm
err := node.Realm().JoinRealm(ctx, types.RealmID("my-public-realm"))
if err != nil {
    if errors.Is(err, realm.ErrAlreadyJoined) {
        log.Println("Already joined another Realm, please leave first")
    }
    return
}

// Join Protected Realm
err := node.Realm().JoinRealm(ctx, 
    types.RealmID("protected-realm"),
    realm.WithJoinKey([]byte("secret-key")),
)

// Join Private Realm (with private bootstrap nodes)
err := node.Realm().JoinRealm(ctx,
    types.RealmID("private-realm"),
    realm.WithPrivateBootstrapPeers(bootstrapAddrs),
    realm.WithSkipDHTRegistration(),
)
```

---

### LeaveRealm

Leaves the current Realm.

```go
func (m RealmManager) LeaveRealm() error
```

**Returns**:
| Type | Description |
|------|-------------|
| `error` | Error information |

**Notes**:
- Sends Goodbye messages to Realm neighbors
- If not joined any Realm, returns `ErrNotMember`

**Example**:

```go
if err := node.Realm().LeaveRealm(); err != nil {
    if errors.Is(err, realm.ErrNotMember) {
        log.Println("Not joined any Realm")
    }
}
```

---

### CurrentRealm

Returns the current Realm.

```go
func (m RealmManager) CurrentRealm() types.RealmID
```

**Returns**:
| Type | Description |
|------|-------------|
| `types.RealmID` | Current Realm ID, empty means not joined |

**Example**:

```go
currentRealm := node.Realm().CurrentRealm()
if currentRealm == "" {
    fmt.Println("Not joined any Realm")
} else {
    fmt.Printf("Current Realm: %s\n", currentRealm)
}
```

---

### IsMember

Checks if joined a Realm.

```go
func (m RealmManager) IsMember() bool
```

**Returns**:
| Type | Description |
|------|-------------|
| `bool` | Whether joined a Realm |

**Example**:

```go
if node.Realm().IsMember() {
    fmt.Println("Joined a Realm")
}
```

---

### IsMemberOf

Checks if a member of specified Realm.

```go
func (m RealmManager) IsMemberOf(realmID types.RealmID) bool
```

**Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `realmID` | `types.RealmID` | Realm ID |

**Returns**:
| Type | Description |
|------|-------------|
| `bool` | Whether a member of that Realm |

---

## Realm Peer Management APIs

### RealmPeers

Returns the list of nodes in the Realm.

```go
func (m RealmManager) RealmPeers(realmID types.RealmID) []types.NodeID
```

**Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `realmID` | `types.RealmID` | Realm ID |

**Returns**:
| Type | Description |
|------|-------------|
| `[]types.NodeID` | List of node IDs |

**Example**:

```go
peers := node.Realm().RealmPeers(types.RealmID("my-realm"))
fmt.Printf("Realm has %d nodes\n", len(peers))
for _, peer := range peers {
    fmt.Printf("  - %s\n", peer.ShortString())
}
```

---

### RealmPeerCount

Returns the number of nodes in the Realm.

```go
func (m RealmManager) RealmPeerCount(realmID types.RealmID) int
```

**Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `realmID` | `types.RealmID` | Realm ID |

**Returns**:
| Type | Description |
|------|-------------|
| `int` | Node count |

---

## Realm Metadata API

### RealmMetadata

Returns Realm metadata.

```go
func (m RealmManager) RealmMetadata() (*types.RealmMetadata, error)
```

**Returns**:
| Type | Description |
|------|-------------|
| `*RealmMetadata` | Realm metadata |
| `error` | Error information |

---

## JoinOption Options

### WithJoinKey

Provides join key (for Protected/Private Realms).

```go
func WithJoinKey(key []byte) JoinOption
```

**Example**:

```go
node.Realm().JoinRealm(ctx, realmID, realm.WithJoinKey([]byte("secret")))
```

---

### WithTimeout

Sets join timeout.

```go
func WithTimeout(d time.Duration) JoinOption
```

**Example**:

```go
node.Realm().JoinRealm(ctx, realmID, realm.WithTimeout(30*time.Second))
```

---

### WithPrivateBootstrapPeers

Specifies private bootstrap nodes (for Private Realms).

```go
func WithPrivateBootstrapPeers(peers []string) JoinOption
```

**Example**:

```go
bootstrapAddrs := []string{
    "/ip4/192.168.1.100/udp/4001/quic-v1/p2p/12D3KooW...",
}
node.Realm().JoinRealm(ctx, realmID,
    realm.WithPrivateBootstrapPeers(bootstrapAddrs),
)
```

---

### WithSkipDHTRegistration

Skips DHT registration (for Private Realms).

```go
func WithSkipDHTRegistration() JoinOption
```

**Example**:

```go
node.Realm().JoinRealm(ctx, realmID,
    realm.WithPrivateBootstrapPeers(addrs),
    realm.WithSkipDHTRegistration(),
)
```

---

## Realm Types

```mermaid
flowchart LR
    subgraph Public [Public Realm]
        P1["No key required"]
        P2["Anyone can join"]
        P3["DHT discoverable"]
    end
    
    subgraph Protected [Protected Realm]
        PR1["Requires JoinKey"]
        PR2["Key validation"]
        PR3["DHT discoverable"]
    end
    
    subgraph Private [Private Realm]
        PV1["Requires JoinKey"]
        PV2["Private bootstrap"]
        PV3["DHT not discoverable"]
    end
```

### Public Realm

- No key required
- Anyone can join
- Registered in public DHT

**Use Cases**: Public chat rooms, public services

```go
node.Realm().JoinRealm(ctx, types.RealmID("public-chat"))
```

---

### Protected Realm

- Requires JoinKey
- Join after key validation
- Registered in public DHT

**Use Cases**: Paid services, member areas

```go
node.Realm().JoinRealm(ctx,
    types.RealmID("premium-service"),
    realm.WithJoinKey(membershipKey),
)
```

---

### Private Realm

- Requires JoinKey
- Uses private bootstrap nodes
- Not registered in public DHT

**Use Cases**: Corporate intranets, private communications

```go
node.Realm().JoinRealm(ctx,
    types.RealmID("company-internal"),
    realm.WithJoinKey(employeeKey),
    realm.WithPrivateBootstrapPeers(internalBootstraps),
    realm.WithSkipDHTRegistration(),
)
```

---

## Realm State Transitions

```mermaid
stateDiagram-v2
    [*] --> NoRealm: Node starts
    NoRealm --> InRealm: JoinRealm()
    InRealm --> NoRealm: LeaveRealm()
    InRealm --> InRealm: JoinRealm() → ErrAlreadyJoined
    NoRealm --> NoRealm: LeaveRealm() → ErrNotMember
```

---

## Error Handling

| Error | Description | Solution |
|-------|-------------|----------|
| `ErrNotMember` | Not joined any Realm | Call `JoinRealm()` first |
| `ErrAlreadyJoined` | Already joined another Realm | Call `LeaveRealm()` first |
| `ErrInvalidJoinKey` | Invalid JoinKey | Check if key is correct |
| `ErrRealmNotFound` | Realm not found | Check if RealmID is correct |

**Example**:

```go
err := node.Realm().JoinRealm(ctx, realmID)
if err != nil {
    switch {
    case errors.Is(err, realm.ErrAlreadyJoined):
        // Leave current Realm first
        node.Realm().LeaveRealm()
        node.Realm().JoinRealm(ctx, realmID)
    case errors.Is(err, realm.ErrInvalidJoinKey):
        log.Println("Invalid key")
    default:
        log.Printf("Join failed: %v", err)
    }
}
```

---

## Method List

| Method | Category | Description |
|--------|----------|-------------|
| `JoinRealm()` | Membership | Join Realm |
| `LeaveRealm()` | Membership | Leave Realm |
| `CurrentRealm()` | Membership | Returns current Realm |
| `IsMember()` | Membership | Checks if joined |
| `IsMemberOf()` | Membership | Checks specific membership |
| `RealmPeers()` | Peer Mgmt | Returns peer list |
| `RealmPeerCount()` | Peer Mgmt | Returns peer count |
| `RealmMetadata()` | Metadata | Returns metadata |

---

## RealmMetadata Structure

```go
type RealmMetadata struct {
    ID          RealmID     // Realm ID
    Name        string      // Human-readable name
    Creator     NodeID      // Creator
    AccessLevel AccessLevel // Access level
    CreatedAt   time.Time   // Creation time
    Description string      // Description
    Signature   []byte      // Creator signature
}

type AccessLevel int

const (
    Public    AccessLevel = 0  // Public
    Protected AccessLevel = 1  // Protected
    Private   AccessLevel = 2  // Private
)
```

---

## Configuration Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `Enable` | bool | `true` | Enable Realm management |
| `AutoJoin` | bool | `false` | Don't auto-join any business Realm |
| `RealmAuthEnabled` | bool | `true` | Enable RealmAuth protocol |
| `RealmAuthTimeout` | Duration | `10s` | RealmAuth timeout |
| `JoinProofTTL` | Duration | `24h` | JoinProof validity period |
| `MemberCacheSize` | int | `10000` | Member cache size |
| `MemberCacheTTL` | Duration | `5m` | Member cache TTL |

---

## Related Documentation

- [Node API](node.md)
- [Messaging API](messaging.md)
- [Join Realm Tutorial](../../getting-started/first-realm.md)
