# msgio - 消息 IO

这是一个简单的包,用于帮助读写长度分隔的切片。它对构建网络协议很有帮助。

## 使用方法

### Reading

```go
import "github.com/dep2p/go-dep2p/p2plib/msgio"
rdr := ... // some reader from a wire
mrdr := msgio.NewReader(rdr)

for {
  msg, err := mrdr.ReadMsg()
  if err != nil {
    return err
  }

  doSomething(msg)
}
```

### Writing

```go
import "github.com/dep2p/go-dep2p/p2plib/msgio"
wtr := genReader()
mwtr := msgio.NewWriter(wtr)

for {
  msg := genMessage()
  err := mwtr.WriteMsg(msg)
  if err != nil {
    return err
  }
}
```

### Duplex

```go
import "github.com/dep2p/go-dep2p/p2plib/msgio"
rw := genReadWriter()
mrw := msgio.NewReadWriter(rw)

for {
  msg, err := mrdr.ReadMsg()
  if err != nil {
    return err
  }

  // echo it back :)
  err = mwtr.WriteMsg(msg)
  if err != nil {
    return err
  }
}
```
