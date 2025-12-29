# Configuration Reference

This document details all configuration options for DeP2P.

---

## Overview

```mermaid
flowchart TB
    subgraph Config [Configuration Categories]
        Basic["Basic Config"]
        Conn["Connection Config"]
        Discovery["Discovery Config"]
        NAT["NAT Config"]
        Relay["Relay Config"]
        Realm["Realm Config"]
    end
    
    Preset["Preset"] --> Config
    Custom["Custom"] --> Config
```

DeP2P uses functional options pattern for configuration:

```go
node, err := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithListenPort(4001),
    dep2p.WithBootstrapPeers(bootstrapAddrs...),
)
```

---

## Basic Configuration

### WithPreset

Uses a preset configuration.

```go
func WithPreset(preset Preset) Option
```

**Parameters**:
| Value | Description |
|-------|-------------|
| `PresetMinimal` | Minimal configuration |
| `PresetDesktop` | Desktop application |
| `PresetServer` | Server |
| `PresetMobile` | Mobile device |

**Example**:

```go
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
```

---

### WithIdentity

Sets node identity.

```go
func WithIdentity(identity crypto.PrivKey) Option
```

**Notes**:
- If not set, a new identity is automatically generated
- Same private key produces same NodeID

**Example**:

```go
// Use existing private key
privKey, _ := crypto.UnmarshalPrivateKey(keyBytes)
node, _ := dep2p.StartNode(ctx, dep2p.WithIdentity(privKey))

// Load from file
key, _ := dep2p.LoadIdentity("node.key")
node, _ := dep2p.StartNode(ctx, dep2p.WithIdentity(key))
```

---

### WithListenPort

Sets the listen port.

```go
func WithListenPort(port int) Option
```

**Notes**:
- Uses random port by default
- Port 0 means random allocation

**Example**:

```go
node, _ := dep2p.StartNode(ctx, dep2p.WithListenPort(4001))
```

---

### WithListenAddrs

Sets listen addresses.

```go
func WithListenAddrs(addrs ...string) Option
```

**Notes**:
- Supports multiple listen addresses
- Supports IPv4 and IPv6

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithListenAddrs(
        "/ip4/0.0.0.0/udp/4001/quic-v1",
        "/ip6/::/udp/4001/quic-v1",
    ),
)
```

---

## Connection Configuration

### WithConnectionLimits

Sets connection limits.

```go
func WithConnectionLimits(low, high int) Option
```

**Parameters**:
| Parameter | Description |
|-----------|-------------|
| `low` | Low watermark (don't trim) |
| `high` | High watermark (start trimming) |

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithConnectionLimits(50, 100),
)
```

---

### WithConnectionTimeout

Sets connection timeout.

```go
func WithConnectionTimeout(d time.Duration) Option
```

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithConnectionTimeout(30*time.Second),
)
```

---

### WithIdleTimeout

Sets idle connection timeout.

```go
func WithIdleTimeout(d time.Duration) Option
```

**Notes**:
- Connections with no data transfer beyond this time will be closed

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithIdleTimeout(5*time.Minute),
)
```

---

## Discovery Configuration

### WithBootstrapPeers

Sets bootstrap nodes.

```go
func WithBootstrapPeers(addrs ...string) Option
```

**Notes**:
- Uses full address format (with /p2p/<NodeID>)
- Can configure multiple bootstrap nodes

**Example**:

```go
bootstrapAddrs := []string{
    "/ip4/104.131.131.82/udp/4001/quic-v1/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
    "/dnsaddr/bootstrap.dep2p.io/p2p/12D3KooWLQj...",
}
node, _ := dep2p.StartNode(ctx,
    dep2p.WithBootstrapPeers(bootstrapAddrs...),
)
```

---

### WithDHT

Enables/configures DHT.

```go
func WithDHT(mode DHTMode) Option
```

**Parameters**:
| Value | Description |
|-------|-------------|
| `DHTClient` | Client mode only |
| `DHTServer` | Server mode |
| `DHTAuto` | Auto mode |

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithDHT(dep2p.DHTServer),
)
```

---

### WithMDNS

Enables mDNS local discovery.

```go
func WithMDNS(enabled bool) Option
```

**Notes**:
- Only effective within LAN
- Enabled by default in desktop preset

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithMDNS(true),
)
```

---

## NAT Configuration

### WithNAT

Enables NAT traversal.

```go
func WithNAT(enabled bool) Option
```

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithNAT(true),
)
```

---

### WithAutoNAT

Enables AutoNAT (auto-detect public reachability).

```go
func WithAutoNAT(enabled bool) Option
```

---

### WithHolePunching

Enables hole punching.

```go
func WithHolePunching(enabled bool) Option
```

---

### WithExternalAddrs

Declares external addresses.

```go
func WithExternalAddrs(addrs ...string) Option
```

**Notes**:
- For scenarios with known public IP
- Skips NAT detection

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithExternalAddrs("/ip4/203.0.113.1/udp/4001/quic-v1"),
)
```

---

### WithSTUNServers

Sets STUN servers.

```go
func WithSTUNServers(servers ...string) Option
```

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithSTUNServers(
        "stun.l.google.com:19302",
        "stun.dep2p.io:3478",
    ),
)
```

---

## Relay Configuration

### WithRelay

Enables relay client.

```go
func WithRelay(enabled bool) Option
```

**Notes**:
- Allows connecting through relay nodes
- Enabled by default

---

### WithAutoRelay

Enables auto relay.

```go
func WithAutoRelay(enabled bool) Option
```

**Notes**:
- Automatically discovers and uses relay nodes
- Recommended for nodes behind NAT

---

### WithRelayServer

Enables relay server.

```go
func WithRelayServer(enabled bool) Option
```

**Notes**:
- Serves as relay node for other nodes
- Requires public IP

**Example**:

```go
// Configure as relay server
node, _ := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithRelayServer(true),
    dep2p.WithListenPort(4001),
)
```

---

### WithStaticRelays

Sets static relay nodes.

```go
func WithStaticRelays(addrs ...string) Option
```

**Notes**:
- Prioritizes specified relay nodes
- Suitable for private deployments

**Example**:

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithStaticRelays(
        "/ip4/relay1.example.com/udp/4001/quic-v1/p2p/12D3KooW...",
    ),
)
```

---

## Realm Configuration

### WithRealmAuth

Enables Realm authentication.

```go
func WithRealmAuth(enabled bool) Option
```

---

### WithRealmAuthTimeout

Sets Realm authentication timeout.

```go
func WithRealmAuthTimeout(d time.Duration) Option
```

---

## Configuration Parameter Table

### Basic Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithPreset` | `Preset` | - | Use preset configuration |
| `WithIdentity` | `crypto.PrivKey` | Auto-generated | Node identity |
| `WithListenPort` | `int` | Random | Listen port |
| `WithListenAddrs` | `[]string` | Default addresses | Listen address list |

### Connection Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithConnectionLimits` | `int, int` | See preset | Connection limits |
| `WithConnectionTimeout` | `Duration` | `30s` | Connection timeout |
| `WithIdleTimeout` | `Duration` | `5m` | Idle timeout |

### Discovery Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithBootstrapPeers` | `[]string` | Public nodes | Bootstrap nodes |
| `WithDHT` | `DHTMode` | `DHTClient` | DHT mode |
| `WithMDNS` | `bool` | See preset | mDNS discovery |

### NAT Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithNAT` | `bool` | `true` | Enable NAT |
| `WithAutoNAT` | `bool` | `true` | Auto NAT |
| `WithHolePunching` | `bool` | `true` | Hole punching |
| `WithExternalAddrs` | `[]string` | - | External addresses |
| `WithSTUNServers` | `[]string` | Public servers | STUN servers |

### Relay Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `WithRelay` | `bool` | `true` | Relay client |
| `WithAutoRelay` | `bool` | `true` | Auto relay |
| `WithRelayServer` | `bool` | `false` | Relay server |
| `WithStaticRelays` | `[]string` | - | Static relays |

---

## Configuration Examples

### Minimal Configuration

```go
node, _ := dep2p.StartNode(ctx)
```

### Desktop Application

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithMDNS(true),
)
```

### Server Configuration

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithListenPort(4001),
    dep2p.WithDHT(dep2p.DHTServer),
    dep2p.WithRelayServer(true),
)
```

### Mobile Configuration

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetMobile),
    dep2p.WithAutoRelay(true),
)
```

### Private Network

```go
node, _ := dep2p.StartNode(ctx,
    dep2p.WithBootstrapPeers(privateBootstraps...),
    dep2p.WithStaticRelays(privateRelays...),
    dep2p.WithMDNS(false),  // Disable public discovery
)
```

---

## Related Documentation

- [Preset Configuration](presets.md)
- [Node API](api/node.md)
- [Quick Start](../getting-started/quickstart.md)
