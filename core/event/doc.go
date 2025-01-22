// Package event 包含本地事件总线的抽象,以及 dep2p 子系统可能发出的标准事件。
//
// 源代码组织如下:
//   - doc.go: 本文件。
//   - bus.go: 事件总线的抽象。
//   - rest: 事件结构体,按实体合理分类到不同文件中,并遵循以下命名约定:
//     Evt[实体(名词)][事件(动词过去式/动名词)]
//     使用过去式表示某事已经发生,而动词的动名词形式(-ing)
//     表示过程正在进行中。示例: EvtConnEstablishing, EvtConnEstablished。
package event
