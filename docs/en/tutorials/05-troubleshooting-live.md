# Live Troubleshooting: Log Analysis Framework

This tutorial will guide you through using DeP2P's log analysis framework to troubleshoot common issues, including connection failures, message loss, NAT traversal problems, and more.

---

## Tutorial Goals

After completing this tutorial, you will learn:

- Using the 8-dimension log analysis framework
- Troubleshooting connection failures
- Diagnosing message loss causes
- Analyzing NAT traversal effectiveness
- Using analysis scripts for quick problem identification

---

## Log Analysis Framework Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    P2P Log Analysis Framework - 8 Dimensions                │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌──────────────┐ │
│  │ 1. Connection │  │ 2. NAT        │  │ 3. Message    │  │ 4. Peer      │ │
│  │    Quality    │  │    Traversal  │  │    Delivery   │  │    Discovery │ │
│  │ - RTT latency │  │ - Success rate│  │ - E2E latency │  │ - Discovery  │ │
│  │ - Success rate│  │ - NAT type    │  │ - Loss rate   │  │   time       │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └──────────────┘ │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐  ┌──────────────┐ │
│  │ 5. Resource   │  │ 6. Failure    │  │ 7. Security   │  │ 8. Geographic│ │
│  │    Efficiency │  │    Recovery   │  │    Detection  │  │    Dist.     │ │
│  │ - Bandwidth   │  │ - Reconnect   │  │ - Anomaly     │  │ - Cross-     │ │
│  │ - CPU/Memory  │  │   time        │  │   detection   │  │   region RTT │ │
│  └───────────────┘  └───────────────┘  └───────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Health Metrics Reference

| Dimension | Key Metric | Healthy Threshold | Alert Threshold |
|-----------|------------|-------------------|-----------------|
| **Connection Quality** | Connection success rate | ≥ 80% | < 50% |
| | Average RTT | < 100ms | > 500ms |
| | Disconnect rate | < 0.1/min | > 1/min |
| **NAT Traversal** | Direct connection rate | ≥ 60% | < 30% |
| | Hole punch success rate | ≥ 50% | < 20% |
| | Relay usage rate | < 20% | > 50% |
| **Message Delivery** | End-to-end latency | < 500ms | > 2s |
| | Message loss rate | < 1% | > 5% |
| **Disconnect Detection** | Detection latency | < 6s | > 15s |
| | Witness confirmation rate | ≥ 50% | < 30% |

---

## Enabling Detailed Logs

### Setting Log Level

```go
import "github.com/dep2p/go-dep2p/pkg/lib/log"

// Enable Debug level logging
log.SetLevel("dep2p", log.LevelDebug)

// Or via environment variable
// export DEP2P_LOG_LEVEL=debug
```

### Logging to File

```go
// Configure log output
log.SetOutput(os.Stdout) // or file

// Command line redirect
// go run main.go 2>&1 | tee dep2p.log
```

---

## Scenario 1: Connection Failure Troubleshooting

### Problem Description

Node A cannot connect to Node B, reporting `dial failed` or timeout.

### Troubleshooting Steps

#### Step 1: Check Connection Logs

```bash
# View connection attempts and failures
grep -E "connecting|dial|connection" dep2p.log | tail -50

# Common log patterns
# Success: connection established peerID=12D3KooW... connType=direct
# Failure: connection failed peerID=12D3KooW... error=dial backoff
```

#### Step 2: Analyze Failure Reasons

```bash
# Connection success rate statistics
echo "=== Connection Statistics ==="
echo -n "Success: "; grep -c "connection.*established\|connected" dep2p.log
echo -n "Failed: "; grep -c "connection.*failed\|dial.*failed" dep2p.log

# Common error types
grep "dial.*failed" dep2p.log | awk -F'error=' '{print $2}' | sort | uniq -c | sort -rn
```

#### Step 3: Check Target Node Status

```bash
# Check if target node was discovered
grep "12D3KooWxxxxx" dep2p.log  # Replace with target PeerID

# Check if addresses exist
grep -E "addrs.*12D3KooWxxxxx|peerstore.*added" dep2p.log
```

#### Common Causes and Solutions

| Error | Cause | Solution |
|-------|-------|----------|
| `dial backoff` | Connection frequency too high, triggered backoff | Wait for backoff to end, or check connection logic |
| `no addresses` | Target address not discovered | Check mDNS/DHT/Bootstrap, or use known_peers |
| `connection refused` | Target port not listening | Confirm target node is running and listening on correct port |
| `timeout` | Network unreachable or firewall | Check network connectivity and firewall rules |
| `peer id mismatch` | PeerID doesn't match address | Verify known_peers configuration is correct |

### Quick Diagnosis Script

```bash
#!/bin/bash
# check-connection.sh - Quick connection problem diagnosis

LOG=${1:-dep2p.log}
PEER=${2:-""}

echo "=== Connection Problem Diagnosis ==="
echo "Log file: $LOG"
echo

# 1. Connection statistics
echo "[1. Connection Statistics]"
TOTAL_SUCCESS=$(grep -c "connection.*established\|connected.*success" "$LOG" 2>/dev/null || echo 0)
TOTAL_FAIL=$(grep -c "connection.*failed\|dial.*failed" "$LOG" 2>/dev/null || echo 0)
TOTAL=$((TOTAL_SUCCESS + TOTAL_FAIL))
if [ $TOTAL -gt 0 ]; then
    RATE=$((TOTAL_SUCCESS * 100 / TOTAL))
    echo "  Success: $TOTAL_SUCCESS, Failed: $TOTAL_FAIL, Success rate: $RATE%"
else
    echo "  No connection records"
fi
echo

# 2. Error distribution
echo "[2. Error Type Distribution]"
grep "dial.*failed\|connection.*failed" "$LOG" 2>/dev/null | \
    sed -n 's/.*error=\([^,}]*\).*/\1/p' | \
    sort | uniq -c | sort -rn | head -5
echo

# 3. Specific node check
if [ -n "$PEER" ]; then
    echo "[3. Node $PEER Connection Records]"
    grep "$PEER" "$LOG" | grep -E "connect|dial" | tail -10
fi
```

---

## Scenario 2: Message Loss Troubleshooting

### Problem Description

Sent messages are not received by the recipient, or have very high latency.

### Troubleshooting Steps

#### Step 1: Trace Message Flow

```bash
# Sender logs
grep -E "publish.*message|Publish|message.*sent" dep2p_sender.log

# Receiver logs
grep -E "received.*message|Received|message.*received" dep2p_receiver.log

# Compare timestamps to calculate latency
```

#### Step 2: Check PubSub Status

```bash
# Check topic subscription
grep -E "Subscribe|topic.*joined" dep2p.log

# Check message propagation
grep -E "gossip|mesh|prune|graft" dep2p.log | tail -20
```

#### Step 3: Check Duplicate Messages

```bash
# Duplicate messages are discarded
grep -E "duplicate|already.*seen" dep2p.log

# If many duplicates, check sending logic
```

#### Common Causes and Solutions

| Symptom | Cause | Solution |
|---------|-------|----------|
| No messages received | Topic mismatch | Confirm sender/receiver use same topic name |
| No messages received | Not in same Realm | Confirm both parties are in the same Realm |
| High latency | Mesh not yet established | Wait for PubSub routing to establish (~5s) |
| Partial loss | Network jitter | Check connection stability |
| Many duplicates | Sender sending duplicates | Check sending logic, add deduplication |

### Message Tracing Script

```bash
#!/bin/bash
# trace-message.sh - Message tracing

LOG=${1:-dep2p.log}
MSG_ID=${2:-""}

echo "=== Message Tracing ==="

# Statistics
echo "[Message Statistics]"
SENT=$(grep -c "publish.*message\|Publish" "$LOG" 2>/dev/null || echo 0)
RECV=$(grep -c "received.*message\|Received" "$LOG" 2>/dev/null || echo 0)
DUP=$(grep -c "duplicate" "$LOG" 2>/dev/null || echo 0)
echo "  Sent: $SENT, Received: $RECV, Duplicates: $DUP"
echo

# Recent messages
echo "[Recent Messages]"
grep -E "publish.*message|received.*message" "$LOG" | tail -10
```

---

## Scenario 3: NAT Traversal Issues

### Problem Description

Two nodes behind NAT cannot connect directly, can only communicate through relay.

### Troubleshooting Steps

#### Step 1: Check Connection Types

```bash
# View connection type distribution
echo "=== Connection Type Distribution ==="
grep "connType=" dep2p.log | \
    sed -n 's/.*connType=\([a-z]*\).*/\1/p' | \
    sort | uniq -c | sort -rn

# Expected output:
#    45 direct      # Direct connection
#    12 holepunch   # Hole punching
#     3 relay       # Relay
```

#### Step 2: Check NAT Type

```bash
# NAT type detection results
grep -E "NAT.*type|NAT.*detected|STUN" dep2p.log

# View observed addresses
grep -E "observed.*addr|external.*addr|STUN.*address" dep2p.log
```

#### Step 3: Check Hole Punching Process

```bash
# Hole punch attempts
grep -E "hole.*punch|DCUtR" dep2p.log

# Success/failure statistics
echo -n "Hole punch success: "; grep -c "holepunch.*success" dep2p.log
echo -n "Hole punch failed: "; grep -c "holepunch.*failed" dep2p.log
```

#### NAT Type Reference Table

| NAT Type | Direct Connection Probability | Hole Punch Success Rate | Recommendation |
|----------|------------------------------|-------------------------|----------------|
| Full Cone | High | 90%+ | Ideal |
| Restricted Cone | Medium | 70%+ | Can hole punch |
| Port Restricted | Low | 40%+ | Need relay backup |
| Symmetric | Very Low | <10% | Rely on relay |

### NAT Analysis Script

```bash
#!/bin/bash
# analyze-nat.sh - NAT traversal analysis

LOG=${1:-dep2p.log}

echo "=== NAT Traversal Analysis ==="
echo

# 1. Connection type statistics
echo "[Connection Type Distribution]"
grep "connType=" "$LOG" | \
    sed -n 's/.*connType=\([a-z]*\).*/\1/p' | \
    sort | uniq -c | sort -rn
echo

# 2. Calculate direct connection rate
DIRECT=$(grep -c "connType=direct" "$LOG" 2>/dev/null || echo 0)
RELAY=$(grep -c "connType=relay" "$LOG" 2>/dev/null || echo 0)
HOLEPUNCH=$(grep -c "connType=holepunch" "$LOG" 2>/dev/null || echo 0)
TOTAL=$((DIRECT + RELAY + HOLEPUNCH))
if [ $TOTAL -gt 0 ]; then
    DIRECT_RATE=$((DIRECT * 100 / TOTAL))
    RELAY_RATE=$((RELAY * 100 / TOTAL))
    echo "[Traversal Effectiveness]"
    echo "  Direct rate: $DIRECT_RATE%"
    echo "  Relay rate: $RELAY_RATE%"
    
    if [ $RELAY_RATE -gt 50 ]; then
        echo "  ⚠️  Warning: Relay usage rate too high, check NAT configuration"
    fi
fi
echo

# 3. Hole punch statistics
echo "[Hole Punch Statistics]"
HP_SUCCESS=$(grep -c "holepunch.*success" "$LOG" 2>/dev/null || echo 0)
HP_FAIL=$(grep -c "holepunch.*failed" "$LOG" 2>/dev/null || echo 0)
echo "  Success: $HP_SUCCESS, Failed: $HP_FAIL"
```

---

## Scenario 4: Disconnect Detection Latency

### Problem Description

After a node disconnects, other nodes receive the MemberLeft event with high latency.

### Troubleshooting Steps

#### Step 1: Check Disconnect Detection Logs

```bash
# Connection disconnect detection
grep -E "connection.*closed|idle.*timeout" dep2p.log

# MemberLeft events
grep -E "MemberLeft" dep2p.log

# Compare timestamps to calculate detection latency
```

#### Step 2: Check Witness Mechanism

```bash
# Witness reports
grep -E "WitnessReport|witness.*report" dep2p.log

# Witness confirmation
grep -E "witness.*quorum" dep2p.log
```

#### Step 3: Check Reconnect Grace Period

```bash
# Reconnect attempts during grace period
grep -E "grace.*period|reconnect" dep2p.log
```

#### Tuning Recommendations

```json
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",    // Reduce: faster detection
      "max_idle_timeout": "6s"       // Reduce: faster timeout
    },
    "reconnect_grace_period": "10s", // Reduce: faster MemberLeft trigger
    "witness": {
      "enabled": true,
      "count": 3,
      "quorum": 2,
      "timeout": "3s"                // Reduce: faster witness confirmation
    }
  }
}
```

---

## Comprehensive Analysis Script

Save the following script as `p2p-log-analyze.sh`:

```bash
#!/bin/bash
# p2p-log-analyze.sh - P2P Log Comprehensive Analysis Script
#
# Usage: ./p2p-log-analyze.sh [log_file] [analysis_dimension]
# Dimensions: all, connection, nat, message, discovery, disconnect

LOG=${1:-dep2p.log}
MODE=${2:-all}

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║              P2P Log Analysis Report                           ║"
echo "╠════════════════════════════════════════════════════════════════╣"
echo "║  Log file: $LOG"
echo "║  Analysis mode: $MODE"
echo "║  Generated: $(date '+%Y-%m-%d %H:%M:%S')"
echo "╚════════════════════════════════════════════════════════════════╝"
echo

# Check log file
if [ ! -f "$LOG" ]; then
    echo "Error: Log file not found: $LOG"
    exit 1
fi

# Connection quality analysis
analyze_connection() {
    echo "━━━ 1. Connection Quality Analysis ━━━"
    echo
    
    # Connection statistics
    SUCCESS=$(grep -c "connection.*established" "$LOG" 2>/dev/null || echo 0)
    FAIL=$(grep -c "connection.*failed\|dial.*failed" "$LOG" 2>/dev/null || echo 0)
    TOTAL=$((SUCCESS + FAIL))
    
    echo "[Connection Statistics]"
    echo "  Success: $SUCCESS"
    echo "  Failed: $FAIL"
    if [ $TOTAL -gt 0 ]; then
        RATE=$((SUCCESS * 100 / TOTAL))
        echo "  Success rate: $RATE%"
        if [ $RATE -lt 50 ]; then
            echo "  ⚠️  Warning: Connection success rate below 50%"
        fi
    fi
    echo
    
    # Error types
    echo "[Error Types Top 5]"
    grep "dial.*failed\|connection.*failed" "$LOG" 2>/dev/null | \
        sed -n 's/.*error=\([^,}]*\).*/\1/p' | \
        sort | uniq -c | sort -rn | head -5
    echo
}

# NAT traversal analysis
analyze_nat() {
    echo "━━━ 2. NAT Traversal Analysis ━━━"
    echo
    
    # Connection types
    echo "[Connection Type Distribution]"
    DIRECT=$(grep -c "connType=direct" "$LOG" 2>/dev/null || echo 0)
    RELAY=$(grep -c "connType=relay" "$LOG" 2>/dev/null || echo 0)
    HOLEPUNCH=$(grep -c "connType=holepunch" "$LOG" 2>/dev/null || echo 0)
    
    echo "  Direct: $DIRECT"
    echo "  Holepunch: $HOLEPUNCH"
    echo "  Relay: $RELAY"
    
    TOTAL=$((DIRECT + RELAY + HOLEPUNCH))
    if [ $TOTAL -gt 0 ]; then
        DIRECT_RATE=$((DIRECT * 100 / TOTAL))
        RELAY_RATE=$((RELAY * 100 / TOTAL))
        echo "  Direct rate: $DIRECT_RATE%"
        if [ $RELAY_RATE -gt 50 ]; then
            echo "  ⚠️  Warning: Relay usage rate $RELAY_RATE% too high"
        fi
    fi
    echo
}

# Message delivery analysis
analyze_message() {
    echo "━━━ 3. Message Delivery Analysis ━━━"
    echo
    
    SENT=$(grep -c "publish.*message\|Publish\|message.*sent" "$LOG" 2>/dev/null || echo 0)
    RECV=$(grep -c "received.*message\|Received\|message.*received" "$LOG" 2>/dev/null || echo 0)
    DUP=$(grep -c "duplicate" "$LOG" 2>/dev/null || echo 0)
    
    echo "[Message Statistics]"
    echo "  Sent: $SENT"
    echo "  Received: $RECV"
    echo "  Duplicates: $DUP"
    echo
}

# Peer discovery analysis
analyze_discovery() {
    echo "━━━ 4. Peer Discovery Analysis ━━━"
    echo
    
    MDNS=$(grep -c "mDNS.*discover\|SourceMDNS" "$LOG" 2>/dev/null || echo 0)
    DHT=$(grep -c "DHT.*Find\|SourceDHT" "$LOG" 2>/dev/null || echo 0)
    BOOTSTRAP=$(grep -c "Bootstrap.*connect\|SourceBootstrap" "$LOG" 2>/dev/null || echo 0)
    KNOWN=$(grep -c "known.*peer\|KnownPeers" "$LOG" 2>/dev/null || echo 0)
    
    echo "[Discovery Source Distribution]"
    echo "  mDNS: $MDNS"
    echo "  DHT: $DHT"
    echo "  Bootstrap: $BOOTSTRAP"
    echo "  KnownPeers: $KNOWN"
    echo
}

# Disconnect detection analysis
analyze_disconnect() {
    echo "━━━ 5. Disconnect Detection Analysis ━━━"
    echo
    
    DISCONNECT=$(grep -c "connection.*closed" "$LOG" 2>/dev/null || echo 0)
    MEMBER_LEFT=$(grep -c "MemberLeft" "$LOG" 2>/dev/null || echo 0)
    WITNESS=$(grep -c "WitnessReport\|witness" "$LOG" 2>/dev/null || echo 0)
    RECONNECT=$(grep -c "reconnect.*success" "$LOG" 2>/dev/null || echo 0)
    
    echo "[Disconnect Detection Statistics]"
    echo "  Disconnections detected: $DISCONNECT"
    echo "  MemberLeft events: $MEMBER_LEFT"
    echo "  Witness reports: $WITNESS"
    echo "  Successful reconnects: $RECONNECT"
    echo
}

# Execute analysis
case $MODE in
    connection)
        analyze_connection
        ;;
    nat)
        analyze_nat
        ;;
    message)
        analyze_message
        ;;
    discovery)
        analyze_discovery
        ;;
    disconnect)
        analyze_disconnect
        ;;
    all|*)
        analyze_connection
        analyze_nat
        analyze_message
        analyze_discovery
        analyze_disconnect
        ;;
esac

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Analysis complete"
```

### Usage

```bash
# Grant execute permission
chmod +x p2p-log-analyze.sh

# Full analysis
./p2p-log-analyze.sh dep2p.log

# Analyze connection quality only
./p2p-log-analyze.sh dep2p.log connection

# Analyze NAT traversal only
./p2p-log-analyze.sh dep2p.log nat
```

---

## Real-time Monitoring Script

```bash
#!/bin/bash
# p2p-log-monitor.sh - Real-time log monitoring

LOG=${1:-dep2p.log}

echo "Real-time monitoring P2P logs: $LOG"
echo "Press Ctrl+C to stop"
echo "═════════════════════════════════════════════"

tail -f "$LOG" | while read line; do
    # Highlight important events
    if echo "$line" | grep -qE "connection.*established|MemberJoined"; then
        echo -e "\033[32m$line\033[0m"  # Green
    elif echo "$line" | grep -qE "connection.*failed|MemberLeft|error"; then
        echo -e "\033[31m$line\033[0m"  # Red
    elif echo "$line" | grep -qE "holepunch|WitnessReport"; then
        echo -e "\033[33m$line\033[0m"  # Yellow
    elif echo "$line" | grep -qE "received.*message|Received"; then
        echo -e "\033[36m$line\033[0m"  # Cyan
    fi
done
```

---

## Summary

| Problem Type | Key Logs | First Check |
|--------------|----------|-------------|
| Connection failure | `dial failed`, `connection refused` | Network connectivity, firewall, address discovery |
| Message loss | `Publish`, `Received`, `duplicate` | Realm match, topic name, PubSub routing |
| NAT issues | `connType=`, `holepunch` | NAT type, relay availability |
| Disconnect delay | `MemberLeft`, `WitnessReport` | Disconnect detection config, grace period settings |

---

## Next Steps

- [Configuration Reference](../reference/configuration.md) - Complete configuration options
- [NAT Traversal](../how-to/nat-traversal.md) - Detailed NAT traversal guide
- [Observability](../how-to/observability.md) - Prometheus metrics and monitoring
