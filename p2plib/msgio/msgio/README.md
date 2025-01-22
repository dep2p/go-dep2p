# msgio 消息头工具

方便地输出 msgio 消息头。

## 安装

```
go get github.com/dep2p/p2plib/msgio/msgio
```

## 使用

```
> msgio -h
msgio - tool to wrap messages with msgio header

Usage
    msgio header 1020 >header
    cat file | msgio wrap >wrapped

Commands
    header <size>   output a msgio header of given size
    wrap            wrap incoming stream with msgio
```
