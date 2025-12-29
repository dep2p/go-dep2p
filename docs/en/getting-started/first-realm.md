# Join Your First Realm

This document introduces the Realm concept and how to join your first Realm.

---

## What is Realm?

Realm is DeP2P's **business isolation tenant**, similar to:

- Kubernetes Namespace
- Cloud VPC (Virtual Private Cloud)
- Database Schema

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Realm Business Isolation                        │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                 │
│   │  Realm A    │    │  Realm B    │    │  Realm C    │                 │
│   │  Blockchain │    │  GameFi     │    │  Storage    │                 │
│   │  Mainnet    │    │  Testnet    │    │  Network    │                 │
│   │             │    │             │    │             │                 │
│   │ ├─ Discovery│    │ ├─ Discovery│    │ ├─ Discovery│                 │
│   │ ├─ Messaging│    │ ├─ Messaging│    │ ├─ Messaging│                 │
│   │ └─ PubSub   │    │ └─ PubSub   │    │ └─ PubSub   │                 │
│   └─────────────┘    └─────────────┘    └─────────────┘                 │
│          │                 │                  │                          │
│          └─────────────────┴──────────────────┘                          │
│                            │                                             │
│   ┌────────────────────────┴────────────────────────────────────────┐   │
│   │              System Infrastructure (Shared DHT/Relay/NAT)        │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Why Realm?

### Core Principles

1. **Each node can only join one Realm at a time**  
   Strict single-Realm model to avoid confusion and security issues.

2. **Business APIs require JoinRealm first**  
   The following APIs require joining a Realm:
   - `Node.Send` / `Node.Request`
   - `Node.Publish` / `Node.Subscribe`

3. **System APIs are unrestricted**  
   The following APIs don't require JoinRealm:
   - `Node.Connect` / `Node.ConnectToAddr`
   - `Node.ListenAddrs` / `Node.AdvertisedAddrs`
   - Discovery / NAT / Relay

### Use Cases

- **Isolate data between applications**: App A's messages won't be received by App B
- **Prevent cross-tenant attacks**: Malicious nodes cannot impersonate other Realm members
- **Simplify programming model**: Framework-level isolation, no need to pass `tenantID` every time

---

## Joining a Realm

### JoinRealm Flow

```mermaid
flowchart TD
    A[Create Node] --> B{Current Realm?}
    B -->|Empty| C[Call JoinRealm]
    B -->|Not Empty| D[Return ErrAlreadyJoined]
    C --> E{Join Success?}
    E -->|Yes| F[Can Use Business APIs]
    E -->|No| G[Handle Error]
    D --> H[LeaveRealm First]
    H --> C
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // Step 1: Create node
    node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()
    
    fmt.Printf("Node ID: %s\n", node.ID())
    fmt.Printf("Current Realm: '%s'\n", node.Realm().CurrentRealm())  // Output: ''
    
    // Step 2: Join Realm
    err = node.Realm().JoinRealm(ctx, "my-blockchain-mainnet")
    if err != nil {
        log.Fatalf("Failed to join Realm: %v", err)
    }
    
    fmt.Printf("Joined: %s\n", node.Realm().CurrentRealm())
    
    // Step 3: Now you can use business APIs
    fmt.Println("Ready to send messages!")
}
```

---

## Realm Types

DeP2P supports three Realm types:

| Type | Description | Join Method |
|------|-------------|-------------|
| **Public** | Anyone can join | `JoinRealm(ctx, realmID)` |
| **Protected** | Requires JoinKey | `JoinRealmWithKey(ctx, realmID, key)` |
| **Private** | Requires invitation | `JoinRealmWithInvite(ctx, realmID, invite)` |

### Public Realm

```go
// Anyone can join
err := node.Realm().JoinRealm(ctx, "public-chat-room")
```

### Protected Realm

```go
// Create protected Realm (admin)
joinKey, err := node.Realm().CreateProtectedRealm(ctx, "vip-club")

// Join protected Realm (member, needs key)
err = node.Realm().JoinRealmWithKey(ctx, "vip-club", joinKey)
```

### Private Realm

```go
// Create invitation (existing member)
invite, err := node.Realm().CreateInvite(ctx, "team-internal", targetNodeID)

// Join with invitation (invitee)
err = node.Realm().JoinRealmWithInvite(ctx, "team-internal", invite)
```

---

## Switching Realms

**Strict Single-Realm**: A node can only be in one business Realm at a time.

```go
// Join mainnet
err := node.Realm().JoinRealm(ctx, "chain-mainnet")
fmt.Println(node.Realm().CurrentRealm())  // chain-mainnet

// Try to switch to testnet directly (will fail)
err = node.Realm().JoinRealm(ctx, "chain-testnet")
// err == ErrAlreadyJoined

// Correct approach: Leave first, then Join
_ = node.Realm().LeaveRealm()
err = node.Realm().JoinRealm(ctx, "chain-testnet")
fmt.Println(node.Realm().CurrentRealm())  // chain-testnet
```

---

## Common Errors

### ErrNotMember

Calling business API without joining Realm:

```go
// ❌ Wrong
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
err := node.Send(ctx, peerID, "/dep2p/app/chat/1.0.0", []byte("hello"))
fmt.Println(err)  // ErrNotMember

// ✅ Correct
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
node.Realm().JoinRealm(ctx, "my-realm")
err := node.Send(ctx, peerID, "/dep2p/app/chat/1.0.0", []byte("hello"))
fmt.Println(err)  // nil
```

### ErrAlreadyJoined

Already in a Realm, trying to join another:

```go
// ❌ Wrong
node.Realm().JoinRealm(ctx, "mainnet")
err := node.Realm().JoinRealm(ctx, "testnet")
fmt.Println(err)  // ErrAlreadyJoined

// ✅ Correct
node.Realm().JoinRealm(ctx, "mainnet")
_ = node.Realm().LeaveRealm()  // Leave first
err := node.Realm().JoinRealm(ctx, "testnet")
fmt.Println(err)  // nil
```

---

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // Create node
    node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()
    
    rm := node.Realm()
    
    // Demonstrate Realm lifecycle
    fmt.Println("=== Realm Lifecycle Demo ===")
    
    // 1. Initial state
    fmt.Printf("1. Initial Realm: '%s'\n", rm.CurrentRealm())
    
    // 2. Join Realm
    if err := rm.JoinRealm(ctx, "demo-realm"); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("2. After joining: '%s'\n", rm.CurrentRealm())
    
    // 3. Try to join again (will fail)
    err = rm.JoinRealm(ctx, "another-realm")
    fmt.Printf("3. Duplicate join result: %v\n", err)
    
    // 4. Leave Realm
    if err := rm.LeaveRealm(); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("4. After leaving: '%s'\n", rm.CurrentRealm())
    
    // 5. Join new Realm
    if err := rm.JoinRealm(ctx, "new-realm"); err != nil {
        log.Fatal(err)
    }
    fmt.Printf("5. New Realm: '%s'\n", rm.CurrentRealm())
}
```

Output:

```
=== Realm Lifecycle Demo ===
1. Initial Realm: ''
2. After joining: 'demo-realm'
3. Duplicate join result: ErrAlreadyJoined
4. After leaving: ''
5. New Realm: 'new-realm'
```

---

## Next Steps

- [FAQ](faq.md) - More answers
- [Core Concepts](../concepts/core-concepts.md) - Deep dive into Realm
- [Realm Tutorial](../tutorials/04-realm-application.md) - Build Realm applications
