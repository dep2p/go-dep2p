// 由 gen.go 生成; 请勿编辑。

package multicodec

const (
	// Identity 是一个永久性代码,标记为 "multihash",描述为:原始二进制。
	Identity Code = 0x00 // identity

	// Cidv1 是一个永久性代码,标记为 "cid",描述为:CIDv1。
	Cidv1 Code = 0x01 // cidv1

	// Cidv2 是一个草案代码,标记为 "cid",描述为:CIDv2。
	Cidv2 Code = 0x02 // cidv2

	// Cidv3 是一个草案代码,标记为 "cid",描述为:CIDv3。
	Cidv3 Code = 0x03 // cidv3

	// Ip4 是一个永久性代码,标记为 "multiaddr"。
	Ip4 Code = 0x04 // ip4

	// Tcp 是一个永久性代码,标记为 "multiaddr"。
	Tcp Code = 0x06 // tcp

	// Sha1 是一个永久性代码,标记为 "multihash"。
	Sha1 Code = 0x11 // sha1

	// Sha2_256 是一个永久性代码,标记为 "multihash"。
	Sha2_256 Code = 0x12 // sha2-256

	// Sha2_512 是一个永久性代码,标记为 "multihash"。
	Sha2_512 Code = 0x13 // sha2-512

	// Sha3_512 是一个永久性代码,标记为 "multihash"。
	Sha3_512 Code = 0x14 // sha3-512

	// Sha3_384 是一个永久性代码,标记为 "multihash"。
	Sha3_384 Code = 0x15 // sha3-384

	// Sha3_256 是一个永久性代码,标记为 "multihash"。
	Sha3_256 Code = 0x16 // sha3-256

	// Sha3_224 是一个永久性代码,标记为 "multihash"。
	Sha3_224 Code = 0x17 // sha3-224

	// Shake128 是一个草案代码,标记为 "multihash"。
	Shake128 Code = 0x18 // shake-128

	// Shake256 是一个草案代码,标记为 "multihash"。
	Shake256 Code = 0x19 // shake-256

	// Keccak224 是一个草案代码,标记为 "multihash",描述为:keccak 具有可变输出长度。数字指定核心长度。
	Keccak224 Code = 0x1a // keccak-224

	// Keccak256 是一个草案代码,标记为 "multihash"。
	Keccak256 Code = 0x1b // keccak-256

	// Keccak384 是一个草案代码,标记为 "multihash"。
	Keccak384 Code = 0x1c // keccak-384

	// Keccak512 是一个草案代码,标记为 "multihash"。
	Keccak512 Code = 0x1d // keccak-512

	// Blake3 是一个草案代码,标记为 "multihash",描述为:BLAKE3 默认输出长度为 32 字节。最大长度为 (2^64)-1 字节。
	Blake3 Code = 0x1e // blake3

	// Sha2_384 是一个永久性代码,标记为 "multihash",描述为:又称 SHA-384;由 FIPS 180-4 规范指定。
	Sha2_384 Code = 0x20 // sha2-384

	// Dccp 是一个草案代码,标记为 "multiaddr"。
	Dccp Code = 0x21 // dccp

	// Murmur3X64_64 是一个永久性代码,标记为 "hash",描述为:murmur3-x64-128 的前 64 位 - 用于 UnixFS 目录分片。
	Murmur3X64_64 Code = 0x22 // murmur3-x64-64

	// Murmur3_32 是一个草案代码,标记为 "hash"。
	Murmur3_32 Code = 0x23 // murmur3-32

	// Ip6 是一个永久性代码,标记为 "multiaddr"。
	Ip6 Code = 0x29 // ip6

	// Ip6zone 是一个草案代码,标记为 "multiaddr"。
	Ip6zone Code = 0x2a // ip6zone

	// Ipcidr 是一个草案代码,标记为 "multiaddr",描述为:IP 地址的 CIDR 掩码。
	Ipcidr Code = 0x2b // ipcidr

	// Path 是一个永久性代码,标记为 "namespace",描述为:字符串路径的命名空间。对应 ASCII 中的 `/`。
	Path Code = 0x2f // path

	// Multicodec 是一个草案代码,标记为 "multiformat"。
	Multicodec Code = 0x30 // multicodec

	// Multihash 是一个草案代码,标记为 "multiformat"。
	Multihash Code = 0x31 // multihash

	// Multiaddr 是一个草案代码,标记为 "multiformat"。
	Multiaddr Code = 0x32 // multiaddr

	// Multibase 是一个草案代码,标记为 "multiformat"。
	Multibase Code = 0x33 // multibase

	// Dns 是一个永久性代码,标记为 "multiaddr"。
	Dns Code = 0x35 // dns

	// Dns4 是一个永久性代码,标记为 "multiaddr"。
	Dns4 Code = 0x36 // dns4

	// Dns6 是一个永久性代码,标记为 "multiaddr"。
	Dns6 Code = 0x37 // dns6

	// Dnsaddr 是一个永久性代码,标记为 "multiaddr"。
	Dnsaddr Code = 0x38 // dnsaddr

	// Protobuf 是一个草案代码,标记为 "serialization",描述为:Protocol Buffers。
	Protobuf Code = 0x50 // protobuf

	// Cbor 是一个永久性代码,标记为 "ipld",描述为:CBOR。
	Cbor Code = 0x51 // cbor

	// Raw 是一个永久性代码,标记为 "ipld",描述为:原始二进制。
	Raw Code = 0x55 // raw

	// DblSha2_256 是一个草案代码,标记为 "multihash"。
	DblSha2_256 Code = 0x56 // dbl-sha2-256

	// Rlp 是一个草案代码,标记为 "serialization",描述为:递归长度前缀。
	Rlp Code = 0x60 // rlp

	// Bencode 是一个草案代码,标记为 "serialization",描述为:bencode。
	Bencode Code = 0x63 // bencode

	// DagPb 是一个永久性代码,标记为 "ipld",描述为:MerkleDAG protobuf。
	DagPb Code = 0x70 // dag-pb

	// DagCbor 是一个永久性代码,标记为 "ipld",描述为:MerkleDAG cbor。
	DagCbor Code = 0x71 // dag-cbor

	// Libp2pKey 是一个永久性代码,标记为 "ipld",描述为:Libp2p 公钥。
	Libp2pKey Code = 0x72 // libp2p-key

	// GitRaw 是一个永久性代码,标记为 "ipld",描述为:原始 Git 对象。
	GitRaw Code = 0x78 // git-raw

	// TorrentInfo 是一个草案代码,标记为 "ipld",描述为:Torrent 文件信息字段(bencode 编码)。
	TorrentInfo Code = 0x7b // torrent-info

	// TorrentFile 是一个草案代码,标记为 "ipld",描述为:Torrent 文件(bencode 编码)。
	TorrentFile Code = 0x7c // torrent-file

	// LeofcoinBlock 是一个草案代码,标记为 "ipld",描述为:Leofcoin 区块。
	LeofcoinBlock Code = 0x81 // leofcoin-block

	// LeofcoinTx 是一个草案代码,标记为 "ipld",描述为:Leofcoin 交易。
	LeofcoinTx Code = 0x82 // leofcoin-tx

	// LeofcoinPr 是一个草案代码,标记为 "ipld",描述为:Leofcoin 对等节点信誉。
	LeofcoinPr Code = 0x83 // leofcoin-pr

	// Sctp 是一个草案代码,标记为 "multiaddr"。
	Sctp Code = 0x84 // sctp

	// DagJose 是一个草案代码,标记为 "ipld",描述为:MerkleDAG JOSE。
	DagJose Code = 0x85 // dag-jose

	// DagCose 是一个草案代码,标记为 "ipld",描述为:MerkleDAG COSE。
	DagCose Code = 0x86 // dag-cose

	// EthBlock 是一个永久性代码,标记为 "ipld",描述为:以太坊区块头(RLP)。
	EthBlock Code = 0x90 // eth-block

	// EthBlockList 是一个永久性代码,标记为 "ipld",描述为:以太坊区块头列表(RLP)。
	EthBlockList Code = 0x91 // eth-block-list

	// EthTxTrie 是一个永久性代码,标记为 "ipld",描述为:以太坊交易树(Eth-Trie)。
	EthTxTrie Code = 0x92 // eth-tx-trie

	// EthTx 是一个永久性代码,标记为 "ipld",描述为:以太坊交易(MarshalBinary)。
	EthTx Code = 0x93 // eth-tx

	// EthTxReceiptTrie 是一个永久性代码,标记为 "ipld",描述为:以太坊交易收据树(Eth-Trie)。
	EthTxReceiptTrie Code = 0x94 // eth-tx-receipt-trie

	// EthTxReceipt 是一个永久性代码,标记为 "ipld",描述为:以太坊交易收据(MarshalBinary)。
	EthTxReceipt Code = 0x95 // eth-tx-receipt

	// EthStateTrie 是一个永久性代码,标记为 "ipld",描述为:以太坊状态树(Eth-Secure-Trie)。
	EthStateTrie Code = 0x96 // eth-state-trie

	// EthAccountSnapshot 是一个永久性代码,标记为 "ipld",描述为:以太坊账户快照(RLP)。
	EthAccountSnapshot Code = 0x97 // eth-account-snapshot

	// EthStorageTrie 是一个永久性代码,标记为 "ipld",描述为:以太坊合约存储树(Eth-Secure-Trie)。
	EthStorageTrie Code = 0x98 // eth-storage-trie

	// EthReceiptLogTrie 是一个草案代码,标记为 "ipld",描述为:以太坊交易收据日志树(Eth-Trie)。
	EthReceiptLogTrie Code = 0x99 // eth-receipt-log-trie

	// EthRecieptLog 是一个草案代码,标记为 "ipld",描述为:以太坊交易收据日志(RLP)。
	EthRecieptLog Code = 0x9a // eth-reciept-log

	// Aes128 是一个草案代码,标记为 "key",描述为:128 位 AES 对称密钥。
	Aes128 Code = 0xa0 // aes-128

	// Aes192 是一个草案代码,标记为 "key",描述为:192 位 AES 对称密钥。
	Aes192 Code = 0xa1 // aes-192

	// Aes256 是一个草案代码,标记为 "key",描述为:256 位 AES 对称密钥。
	Aes256 Code = 0xa2 // aes-256

	// Chacha128 是一个草案代码,标记为 "key",描述为:128 位 ChaCha 对称密钥。
	Chacha128 Code = 0xa3 // chacha-128

	// Chacha256 是一个草案代码,标记为 "key",描述为:256 位 ChaCha 对称密钥。
	Chacha256 Code = 0xa4 // chacha-256

	// BitcoinBlock 是一个永久性代码,标记为 "ipld",描述为:比特币区块。
	BitcoinBlock Code = 0xb0 // bitcoin-block

	// BitcoinTx 是一个永久性代码,标记为 "ipld",描述为:比特币交易。
	BitcoinTx Code = 0xb1 // bitcoin-tx

	// BitcoinWitnessCommitment 是一个永久性代码,标记为 "ipld",描述为:比特币见证承诺。
	BitcoinWitnessCommitment Code = 0xb2 // bitcoin-witness-commitment

	// ZcashBlock 是一个永久性代码,标记为 "ipld",描述为:Zcash 区块。
	ZcashBlock Code = 0xc0 // zcash-block

	// ZcashTx 是一个永久性代码,标记为 "ipld",描述为:Zcash 交易。
	ZcashTx Code = 0xc1 // zcash-tx

	// Caip50 是一个草案代码,标记为 "multiformat",描述为:CAIP-50 多链账户 ID。
	Caip50 Code = 0xca // caip-50

	// Streamid 是一个草案代码,标记为 "namespace",描述为:Ceramic 流 ID。
	Streamid Code = 0xce // streamid

	// StellarBlock 是一个草案代码,标记为 "ipld",描述为:Stellar 区块。
	StellarBlock Code = 0xd0 // stellar-block

	// StellarTx 是一个草案代码,标记为 "ipld",描述为:Stellar 交易。
	StellarTx Code = 0xd1 // stellar-tx

	// Md4 是一个草案代码,标记为 "multihash"。
	Md4 Code = 0xd4 // md4

	// Md5 是一个草案代码,标记为 "multihash"。
	Md5 Code = 0xd5 // md5

	// DecredBlock 是一个草案代码,标记为 "ipld",描述为:Decred 区块。
	DecredBlock Code = 0xe0 // decred-block

	// DecredTx 是一个草案代码,标记为 "ipld",描述为:Decred 交易。
	DecredTx Code = 0xe1 // decred-tx

	// Ipld 是一个草案代码,标记为 "namespace",描述为:IPLD 路径。
	Ipld Code = 0xe2 // ipld

	// Ipfs 是一个草案代码,标记为 "namespace",描述为:IPFS 路径。
	Ipfs Code = 0xe3 // ipfs

	// Swarm 是一个草案代码,标记为 "namespace",描述为:Swarm 路径。
	Swarm Code = 0xe4 // swarm

	// Ipns 是一个草案代码,标记为 "namespace",描述为:IPNS 路径。
	Ipns Code = 0xe5 // ipns

	// Zeronet 是一个草案代码,标记为 "namespace",描述为:ZeroNet 站点地址。
	Zeronet Code = 0xe6 // zeronet

	// Secp256k1Pub 是一个草案代码,标记为 "key",描述为:Secp256k1 公钥(压缩)。
	Secp256k1Pub Code = 0xe7 // secp256k1-pub

	// Dnslink 是一个永久性代码,标记为 "namespace",描述为:DNSLink 路径。
	Dnslink Code = 0xe8 // dnslink

	// Bls12_381G1Pub 是一个草案代码,标记为 "key",描述为:BLS12-381 G1 字段中的公钥。
	Bls12_381G1Pub Code = 0xea // bls12_381-g1-pub

	// Bls12_381G2Pub 是一个草案代码,标记为 "key",描述为:BLS12-381 G2 字段中的公钥。
	Bls12_381G2Pub Code = 0xeb // bls12_381-g2-pub

	// X25519Pub 是一个草案代码,标记为 "key",描述为:Curve25519 公钥。
	X25519Pub Code = 0xec // x25519-pub

	// Ed25519Pub 是一个草案代码,标记为 "key",描述为:Ed25519 公钥。
	Ed25519Pub Code = 0xed // ed25519-pub

	// Bls12_381G1g2Pub 是一个草案代码,标记为 "key",描述为:BLS12-381 G1 和 G2 字段中的连接公钥。
	Bls12_381G1g2Pub Code = 0xee // bls12_381-g1g2-pub

	// Sr25519Pub 是一个草案代码,标记为 "key",描述为:Sr25519 公钥。
	Sr25519Pub Code = 0xef // sr25519-pub

	// DashBlock 是一个草案代码,标记为 "ipld",描述为:Dash 区块。
	DashBlock Code = 0xf0 // dash-block

	// DashTx 是一个草案代码,标记为 "ipld",描述为:Dash 交易。
	DashTx Code = 0xf1 // dash-tx

	// SwarmManifest 是一个草案代码,标记为 "ipld",描述为:Swarm 清单。
	SwarmManifest Code = 0xfa // swarm-manifest

	// SwarmFeed 是一个草案代码,标记为 "ipld",描述为:Swarm Feed。
	SwarmFeed Code = 0xfb // swarm-feed

	// Beeson 是一个草案代码,标记为 "ipld",描述为:Swarm BeeSon。
	Beeson Code = 0xfc // beeson

	// Udp 是一个草案代码,标记为 "multiaddr"。
	Udp Code = 0x0111 // udp

	// P2pWebrtcStar 是一个已弃用代码,标记为 "multiaddr",描述为:请使用 webrtc 或 webrtc-direct 代替。
	P2pWebrtcStar Code = 0x0113 // p2p-webrtc-star

	// P2pWebrtcDirect 是一个已弃用代码,标记为 "multiaddr",描述为:请使用 webrtc 或 webrtc-direct 代替。
	P2pWebrtcDirect Code = 0x0114 // p2p-webrtc-direct

	// P2pStardust 是一个已弃用代码,标记为 "multiaddr"。
	P2pStardust Code = 0x0115 // p2p-stardust

	// WebrtcDirect 是一个草案代码,标记为 "multiaddr",描述为:ICE-lite webrtc 传输,在连接建立期间进行 SDP 处理,不使用 STUN 服务器。
	WebrtcDirect Code = 0x0118 // webrtc-direct

	// Webrtc 是一个草案代码,标记为 "multiaddr",描述为:按照 w3c 规范建立连接的 webrtc 传输。
	Webrtc Code = 0x0119 // webrtc

	// P2pCircuit 是一个永久性代码,标记为 "multiaddr"。
	P2pCircuit Code = 0x0122 // p2p-circuit

	// DagJson 是一个永久性代码,标记为 "ipld",描述为:MerkleDAG json。
	DagJson Code = 0x0129 // dag-json

	// Udt 是一个草案代码,标记为 "multiaddr"。
	Udt Code = 0x012d // udt

	// Utp 是一个草案代码,标记为 "multiaddr"。
	Utp Code = 0x012e // utp

	// Crc32 是一个草案代码,标记为 "hash",描述为:CRC-32 非加密哈希算法(IEEE 802.3)。
	Crc32 Code = 0x0132 // crc32

	// Crc64Ecma 是一个草案代码,标记为 "hash",描述为:CRC-64 非加密哈希算法(ECMA-182 - 附录 B)。
	Crc64Ecma Code = 0x0164 // crc64-ecma

	// Unix 是一个永久性代码,标记为 "multiaddr"。
	Unix Code = 0x0190 // unix

	// Thread 是一个草案代码,标记为 "multiaddr",描述为:Textile Thread。
	Thread Code = 0x0196 // thread

	// P2p 是一个永久性代码,标记为 "multiaddr",描述为:libp2p。
	P2p Code = 0x01a5 // p2p

	// Https 是一个草案代码,标记为 "multiaddr"。
	Https Code = 0x01bb // https

	// Onion 是一个草案代码,标记为 "multiaddr"。
	Onion Code = 0x01bc // onion

	// Onion3 是一个草案代码,标记为 "multiaddr"。
	Onion3 Code = 0x01bd // onion3

	// Garlic64 是一个草案代码,标记为 "multiaddr",描述为:I2P base64(原始公钥)。
	Garlic64 Code = 0x01be // garlic64

	// Garlic32 是一个草案代码,标记为 "multiaddr",描述为:I2P base32(哈希公钥或编码的公钥/校验和+可选密钥)。
	Garlic32 Code = 0x01bf // garlic32

	// Tls 是一个草案代码,标记为 "multiaddr"。
	Tls Code = 0x01c0 // tls

	// Sni 是一个草案代码,标记为 "multiaddr",描述为:服务器名称指示 RFC 6066 § 3。
	Sni Code = 0x01c1 // sni

	// Noise 是一个草案代码,标记为 "multiaddr"。
	Noise Code = 0x01c6 // noise

	// Quic 是一个永久性代码,标记为 "multiaddr"。
	Quic Code = 0x01cc // quic

	// QuicV1 是一个永久性代码,标记为 "multiaddr"。
	QuicV1 Code = 0x01cd // quic-v1

	// Webtransport 是一个草案代码,标记为 "multiaddr"。
	Webtransport Code = 0x01d1 // webtransport

	// Certhash 是一个草案代码,标记为 "multiaddr",描述为:TLS 证书的指纹作为 multihash。
	Certhash Code = 0x01d2 // certhash

	// Ws 是一个永久性代码,标记为 "multiaddr"。
	Ws Code = 0x01dd // ws

	// Wss 是一个永久性代码,标记为 "multiaddr"。
	Wss Code = 0x01de // wss

	// P2pWebsocketStar 是一个永久性代码,标记为 "multiaddr"。
	P2pWebsocketStar Code = 0x01df // p2p-websocket-star

	// Http 是一个草案代码,标记为 "multiaddr"。
	Http Code = 0x01e0 // http

	// Swhid1Snp 是一个草案代码,标记为 "ipld",描述为:软件遗产持久标识符版本 1 快照。
	Swhid1Snp Code = 0x01f0 // swhid-1-snp

	// Json 是一个永久性代码,标记为 "ipld",描述为:JSON(UTF-8 编码)。
	Json Code = 0x0200 // json

	// Messagepack 是一个草案代码,标记为 "serialization",描述为:MessagePack。
	Messagepack Code = 0x0201 // messagepack

	// Car 是一个草案代码,标记为 "serialization",描述为:内容可寻址存档(CAR)。
	Car Code = 0x0202 // car

	// IpnsRecord 是一个永久性代码,标记为 "serialization",描述为:已签名的 IPNS 记录。
	IpnsRecord Code = 0x0300 // ipns-record

	// Libp2pPeerRecord 是一个永久性代码,标记为 "libp2p",描述为:dep2p 对等节点记录类型。
	Libp2pPeerRecord Code = 0x0301 // libp2p-peer-record

	// Libp2pRelayRsvp 是一个永久性代码,标记为 "libp2p",描述为:dep2p 中继预留凭证。
	Libp2pRelayRsvp Code = 0x0302 // libp2p-relay-rsvp

	// Memorytransport 是一个永久性代码,标记为 "libp2p",描述为:用于自拨号和测试的内存传输;任意。
	Memorytransport Code = 0x0309 // memorytransport

	// CarIndexSorted 是一个草案代码,标记为 "serialization",描述为:CARv2 IndexSorted 索引格式。
	CarIndexSorted Code = 0x0400 // car-index-sorted

	// CarMultihashIndexSorted 是一个草案代码,标记为 "serialization",描述为:CARv2 MultihashIndexSorted 索引格式。
	CarMultihashIndexSorted Code = 0x0401 // car-multihash-index-sorted

	// TransportBitswap 是一个草案代码,标记为 "transport",描述为:Bitswap 数据传输。
	TransportBitswap Code = 0x0900 // transport-bitswap

	// TransportGraphsyncFilecoinv1 是一个草案代码,标记为 "transport",描述为:Filecoin graphsync 数据传输。
	TransportGraphsyncFilecoinv1 Code = 0x0910 // transport-graphsync-filecoinv1

	// TransportIpfsGatewayHttp 是一个草案代码,标记为 "transport",描述为:HTTP IPFS 网关无信任数据传输。
	TransportIpfsGatewayHttp Code = 0x0920 // transport-ipfs-gateway-http

	// Multidid 是一个草案代码,标记为 "multiformat",描述为:去中心化标识符的紧凑编码。
	Multidid Code = 0x0d1d // multidid

	// Sha2_256Trunc254Padded 是一个永久性代码,标记为 "multihash",描述为:SHA2-256 最后一个字节的两个最高有效位置零(通过与 0b00111111 掩码) - 用于 Filecoin 中的证明树。
	Sha2_256Trunc254Padded Code = 0x1012 // sha2-256-trunc254-padded

	// Sha2_224 是一个永久性代码,标记为 "multihash",描述为:又称 SHA-224;由 FIPS 180-4 规范定义。
	Sha2_224 Code = 0x1013 // sha2-224

	// Sha2_512_224 是一个永久性代码,标记为 "multihash",描述为:又称 SHA-512/224;由 FIPS 180-4 规范定义。
	Sha2_512_224 Code = 0x1014 // sha2-512-224

	// Sha2_512_256 是一个永久性代码,标记为 "multihash",描述为:又称 SHA-512/256;由 FIPS 180-4 规范定义。
	Sha2_512_256 Code = 0x1015 // sha2-512-256

	// Murmur3X64_128 是一个草案代码,标记为 "hash"。
	Murmur3X64_128 Code = 0x1022 // murmur3-x64-128

	// Ripemd128 是一个草案代码,标记为 "multihash"。
	Ripemd128 Code = 0x1052 // ripemd-128

	// Ripemd160 是一个草案代码,标记为 "multihash"。
	Ripemd160 Code = 0x1053 // ripemd-160

	// Ripemd256 是一个草案代码,标记为 "multihash"。
	Ripemd256 Code = 0x1054 // ripemd-256

	// Ripemd320 是一个草案代码,标记为 "multihash"。
	Ripemd320 Code = 0x1055 // ripemd-320

	// X11 是一个草案代码,标记为 "multihash"。
	X11 Code = 0x1100 // x11

	// P256Pub 是一个草案代码,标记为 "key",描述为:P-256 公钥(压缩)。
	P256Pub Code = 0x1200 // p256-pub

	// P384Pub 是一个草案代码,标记为 "key",描述为:P-384 公钥(压缩)。
	P384Pub Code = 0x1201 // p384-pub

	// P521Pub 是一个草案代码,标记为 "key",描述为:P-521 公钥(压缩)。
	P521Pub Code = 0x1202 // p521-pub

	// Ed448Pub 是一个草案代码,标记为 "key",描述为:Ed448 公钥。
	Ed448Pub Code = 0x1203 // ed448-pub

	// X448Pub 是一个草案代码,标记为 "key",描述为:X448 公钥。
	X448Pub Code = 0x1204 // x448-pub

	// RsaPub 是一个草案代码,标记为 "key",描述为:RSA 公钥。根据 IETF RFC 8017 (PKCS #1) 的 DER 编码 ASN.1 类型 RSAPublicKey。
	RsaPub Code = 0x1205 // rsa-pub

	// Sm2Pub 是一个草案代码,标记为 "key",描述为:SM2 公钥(压缩)。
	Sm2Pub Code = 0x1206 // sm2-pub

	// Ed25519Priv 是一个草案代码,标记为 "key",描述为:Ed25519 私钥。
	Ed25519Priv Code = 0x1300 // ed25519-priv

	// Secp256k1Priv 是一个草案代码,标记为 "key",描述为:Secp256k1 私钥。
	Secp256k1Priv Code = 0x1301 // secp256k1-priv

	// X25519Priv 是一个草案代码,标记为 "key",描述为:Curve25519 私钥。
	X25519Priv Code = 0x1302 // x25519-priv

	// Sr25519Priv 是一个草案代码,标记为 "key",描述为:Sr25519 私钥。
	Sr25519Priv Code = 0x1303 // sr25519-priv

	// RsaPriv 是一个草案代码,标记为 "key",描述为:RSA 私钥。
	RsaPriv Code = 0x1305 // rsa-priv

	// P256Priv 是一个草案代码,标记为 "key",描述为:P-256 私钥。
	P256Priv Code = 0x1306 // p256-priv

	// P384Priv 是一个草案代码,标记为 "key",描述为:P-384 私钥。
	P384Priv Code = 0x1307 // p384-priv

	// P521Priv 是一个草案代码,标记为 "key",描述为:P-521 私钥。
	P521Priv Code = 0x1308 // p521-priv

	// Kangarootwelve 是一个草案代码,标记为 "multihash",描述为:KangarooTwelve 是基于 Keccak-p 的可扩展输出哈希函数。
	Kangarootwelve Code = 0x1d01 // kangarootwelve

	// AesGcm256 是一个草案代码,标记为 "encryption",描述为:使用 256 位密钥和 12 字节 IV 的 AES Galois/Counter 模式。
	AesGcm256 Code = 0x2000 // aes-gcm-256

	// Silverpine 是一个草案代码,标记为 "multiaddr",描述为:基于 yggdrasil 和 ironwood 路由协议的实验性 QUIC。
	Silverpine Code = 0x3f42 // silverpine

	// Sm3_256 是一个草案代码,标记为 "multihash"。
	Sm3_256 Code = 0x534d // sm3-256

	// Blake2b8 是一个草案代码,标记为 "multihash",描述为:Blake2b 包含 64 种输出长度,每种长度产生不同的哈希值。
	Blake2b8 Code = 0xb201 // blake2b-8

	// Blake2b16 是一个草案代码,标记为 "multihash"。
	Blake2b16 Code = 0xb202 // blake2b-16

	// Blake2b24 是一个草案代码,标记为 "multihash"。
	Blake2b24 Code = 0xb203 // blake2b-24

	// Blake2b32 是一个草案代码,标记为 "multihash"。
	Blake2b32 Code = 0xb204 // blake2b-32

	// Blake2b40 是一个草案代码,标记为 "multihash"。
	Blake2b40 Code = 0xb205 // blake2b-40

	// Blake2b48 是一个草案代码,标记为 "multihash"。
	Blake2b48 Code = 0xb206 // blake2b-48

	// Blake2b56 是一个草案代码,标记为 "multihash"。
	Blake2b56 Code = 0xb207 // blake2b-56

	// Blake2b64 是一个草案代码,标记为 "multihash"。
	Blake2b64 Code = 0xb208 // blake2b-64

	// Blake2b72 是一个草案代码,标记为 "multihash"。
	Blake2b72 Code = 0xb209 // blake2b-72

	// Blake2b80 是一个草案代码,标记为 "multihash"。
	Blake2b80 Code = 0xb20a // blake2b-80

	// Blake2b88 是一个草案代码,标记为 "multihash"。
	Blake2b88 Code = 0xb20b // blake2b-88

	// Blake2b96 是一个草案代码,标记为 "multihash"。
	Blake2b96 Code = 0xb20c // blake2b-96

	// Blake2b104 是一个草案代码,标记为 "multihash"。
	Blake2b104 Code = 0xb20d // blake2b-104

	// Blake2b112 是一个草案代码,标记为 "multihash"。
	Blake2b112 Code = 0xb20e // blake2b-112

	// Blake2b120 是一个草案代码,标记为 "multihash"。
	Blake2b120 Code = 0xb20f // blake2b-120

	// Blake2b128 是一个草案代码,标记为 "multihash"。
	Blake2b128 Code = 0xb210 // blake2b-128

	// Blake2b136 是一个草案代码,标记为 "multihash"。
	Blake2b136 Code = 0xb211 // blake2b-136

	// Blake2b144 是一个草案代码,标记为 "multihash"。
	Blake2b144 Code = 0xb212 // blake2b-144

	// Blake2b152 是一个草案代码,标记为 "multihash"。
	Blake2b152 Code = 0xb213 // blake2b-152

	// Blake2b160 是一个草案代码,标记为 "multihash"。
	Blake2b160 Code = 0xb214 // blake2b-160

	// Blake2b168 是一个草案代码,标记为 "multihash"。
	Blake2b168 Code = 0xb215 // blake2b-168

	// Blake2b176 是一个草案代码,标记为 "multihash"。
	Blake2b176 Code = 0xb216 // blake2b-176

	// Blake2b184 是一个草案代码,标记为 "multihash"。
	Blake2b184 Code = 0xb217 // blake2b-184

	// Blake2b192 是一个草案代码,标记为 "multihash"。
	Blake2b192 Code = 0xb218 // blake2b-192

	// Blake2b200 是一个草案代码,标记为 "multihash"。
	Blake2b200 Code = 0xb219 // blake2b-200

	// Blake2b208 是一个草案代码,标记为 "multihash"。
	Blake2b208 Code = 0xb21a // blake2b-208

	// Blake2b216 是一个草案代码,标记为 "multihash"。
	Blake2b216 Code = 0xb21b // blake2b-216

	// Blake2b224 是一个草案代码,标记为 "multihash"。
	Blake2b224 Code = 0xb21c // blake2b-224

	// Blake2b232 是一个草案代码,标记为 "multihash"。
	Blake2b232 Code = 0xb21d // blake2b-232

	// Blake2b240 是一个草案代码,标记为 "multihash"。
	Blake2b240 Code = 0xb21e // blake2b-240

	// Blake2b248 是一个草案代码,标记为 "multihash"。
	Blake2b248 Code = 0xb21f // blake2b-248

	// Blake2b256 是一个永久性代码,标记为 "multihash"。
	Blake2b256 Code = 0xb220 // blake2b-256

	// Blake2b264 是一个草案代码,标记为 "multihash"。
	Blake2b264 Code = 0xb221 // blake2b-264

	// Blake2b272 是一个草案代码,标记为 "multihash"。
	Blake2b272 Code = 0xb222 // blake2b-272

	// Blake2b280 是一个草案代码,标记为 "multihash"。
	Blake2b280 Code = 0xb223 // blake2b-280

	// Blake2b288 是一个草案代码,标记为 "multihash"。
	Blake2b288 Code = 0xb224 // blake2b-288

	// Blake2b296 是一个草案代码,标记为 "multihash"。
	Blake2b296 Code = 0xb225 // blake2b-296

	// Blake2b304 是一个草案代码,标记为 "multihash"。
	Blake2b304 Code = 0xb226 // blake2b-304

	// Blake2b312 是一个草案代码,标记为 "multihash"。
	Blake2b312 Code = 0xb227 // blake2b-312

	// Blake2b320 是一个草案代码,标记为 "multihash"。
	Blake2b320 Code = 0xb228 // blake2b-320

	// Blake2b328 是一个草案代码,标记为 "multihash"。
	Blake2b328 Code = 0xb229 // blake2b-328

	// Blake2b336 是一个草案代码,标记为 "multihash"。
	Blake2b336 Code = 0xb22a // blake2b-336

	// Blake2b344 是一个草案代码,标记为 "multihash"。
	Blake2b344 Code = 0xb22b // blake2b-344

	// Blake2b352 是一个草案代码,标记为 "multihash"。
	Blake2b352 Code = 0xb22c // blake2b-352

	// Blake2b360 是一个草案代码,标记为 "multihash"。
	Blake2b360 Code = 0xb22d // blake2b-360

	// Blake2b368 是一个草案代码,标记为 "multihash"。
	Blake2b368 Code = 0xb22e // blake2b-368

	// Blake2b376 是一个草案代码,标记为 "multihash"。
	Blake2b376 Code = 0xb22f // blake2b-376

	// Blake2b384 是一个草案代码,标记为 "multihash"。
	Blake2b384 Code = 0xb230 // blake2b-384

	// Blake2b392 是一个草案代码,标记为 "multihash"。
	Blake2b392 Code = 0xb231 // blake2b-392

	// Blake2b400 是一个草案代码,标记为 "multihash"。
	Blake2b400 Code = 0xb232 // blake2b-400

	// Blake2b408 是一个草案代码,标记为 "multihash"。
	Blake2b408 Code = 0xb233 // blake2b-408

	// Blake2b416 是一个草案代码,标记为 "multihash"。
	Blake2b416 Code = 0xb234 // blake2b-416

	// Blake2b424 是一个草案代码,标记为 "multihash"。
	Blake2b424 Code = 0xb235 // blake2b-424

	// Blake2b432 是一个草案代码,标记为 "multihash"。
	Blake2b432 Code = 0xb236 // blake2b-432

	// Blake2b440 是一个草案代码,标记为 "multihash"。
	Blake2b440 Code = 0xb237 // blake2b-440

	// Blake2b448 是一个草案代码,标记为 "multihash"。
	Blake2b448 Code = 0xb238 // blake2b-448

	// Blake2b456 是一个草案代码,标记为 "multihash"。
	Blake2b456 Code = 0xb239 // blake2b-456

	// Blake2b464 是一个草案代码,标记为 "multihash"。
	Blake2b464 Code = 0xb23a // blake2b-464

	// Blake2b472 是一个草案代码,标记为 "multihash"。
	Blake2b472 Code = 0xb23b // blake2b-472

	// Blake2b480 是一个草案代码,标记为 "multihash"。
	Blake2b480 Code = 0xb23c // blake2b-480

	// Blake2b488 是一个草案代码,标记为 "multihash"。
	Blake2b488 Code = 0xb23d // blake2b-488

	// Blake2b496 是一个草案代码,标记为 "multihash"。
	Blake2b496 Code = 0xb23e // blake2b-496

	// Blake2b504 是一个草案代码,标记为 "multihash"。
	Blake2b504 Code = 0xb23f // blake2b-504

	// Blake2b512 是一个草案代码,标记为 "multihash"。
	Blake2b512 Code = 0xb240 // blake2b-512

	// Blake2s8 是一个草案代码,标记为 "multihash",描述为:Blake2s 包含 32 种输出长度,每种长度产生不同的哈希值。
	Blake2s8 Code = 0xb241 // blake2s-8

	// Blake2s16 是一个草案代码,标记为 "multihash"。
	Blake2s16 Code = 0xb242 // blake2s-16

	// Blake2s24 是一个草案代码,标记为 "multihash"。
	Blake2s24 Code = 0xb243 // blake2s-24

	// Blake2s32 是一个草案代码,标记为 "multihash"。
	Blake2s32 Code = 0xb244 // blake2s-32

	// Blake2s40 是一个草案代码,标记为 "multihash"。
	Blake2s40 Code = 0xb245 // blake2s-40

	// Blake2s48 是一个草案代码,标记为 "multihash"。
	Blake2s48 Code = 0xb246 // blake2s-48

	// Blake2s56 是一个草案代码,标记为 "multihash"。
	Blake2s56 Code = 0xb247 // blake2s-56

	// Blake2s64 是一个草案代码,标记为 "multihash"。
	Blake2s64 Code = 0xb248 // blake2s-64

	// Blake2s72 是一个草案代码,标记为 "multihash"。
	Blake2s72 Code = 0xb249 // blake2s-72

	// Blake2s80 是一个草案代码,标记为 "multihash"。
	Blake2s80 Code = 0xb24a // blake2s-80

	// Blake2s88 是一个草案代码,标记为 "multihash"。
	Blake2s88 Code = 0xb24b // blake2s-88

	// Blake2s96 是一个草案代码,标记为 "multihash"。
	Blake2s96 Code = 0xb24c // blake2s-96

	// Blake2s104 是一个草案代码,标记为 "multihash"。
	Blake2s104 Code = 0xb24d // blake2s-104

	// Blake2s112 是一个草案代码,标记为 "multihash"。
	Blake2s112 Code = 0xb24e // blake2s-112

	// Blake2s120 是一个草案代码,标记为 "multihash"。
	Blake2s120 Code = 0xb24f // blake2s-120

	// Blake2s128 是一个草案代码,标记为 "multihash"。
	Blake2s128 Code = 0xb250 // blake2s-128

	// Blake2s136 是一个草案代码,标记为 "multihash"。
	Blake2s136 Code = 0xb251 // blake2s-136

	// Blake2s144 是一个草案代码,标记为 "multihash"。
	Blake2s144 Code = 0xb252 // blake2s-144

	// Blake2s152 是一个草案代码,标记为 "multihash"。
	Blake2s152 Code = 0xb253 // blake2s-152

	// Blake2s160 是一个草案代码,标记为 "multihash"。
	Blake2s160 Code = 0xb254 // blake2s-160

	// Blake2s168 是一个草案代码,标记为 "multihash"。
	Blake2s168 Code = 0xb255 // blake2s-168

	// Blake2s176 是一个草案代码,标记为 "multihash"。
	Blake2s176 Code = 0xb256 // blake2s-176

	// Blake2s184 是一个草案代码,标记为 "multihash"。
	Blake2s184 Code = 0xb257 // blake2s-184

	// Blake2s192 是一个草案代码,标记为 "multihash"。
	Blake2s192 Code = 0xb258 // blake2s-192

	// Blake2s200 是一个草案代码,标记为 "multihash"。
	Blake2s200 Code = 0xb259 // blake2s-200

	// Blake2s208 是一个草案代码,标记为 "multihash"。
	Blake2s208 Code = 0xb25a // blake2s-208

	// Blake2s216 是一个草案代码,标记为 "multihash"。
	Blake2s216 Code = 0xb25b // blake2s-216

	// Blake2s224 是一个草案代码,标记为 "multihash"。
	Blake2s224 Code = 0xb25c // blake2s-224

	// Blake2s232 是一个草案代码,标记为 "multihash"。
	Blake2s232 Code = 0xb25d // blake2s-232

	// Blake2s240 是一个草案代码,标记为 "multihash"。
	Blake2s240 Code = 0xb25e // blake2s-240

	// Blake2s248 是一个草案代码,标记为 "multihash"。
	Blake2s248 Code = 0xb25f // blake2s-248

	// Blake2s256 是一个草案代码,标记为 "multihash"。
	Blake2s256 Code = 0xb260 // blake2s-256

	// Skein256_8 是一个草案代码,标记为 "multihash",描述为:Skein256 包含 32 种输出长度,每种长度产生不同的哈希值。
	Skein256_8 Code = 0xb301 // skein256-8

	// Skein256_16 是一个草案代码,标记为 "multihash"。
	Skein256_16 Code = 0xb302 // skein256-16

	// Skein256_24 是一个草案代码,标记为 "multihash"。
	Skein256_24 Code = 0xb303 // skein256-24

	// Skein256_32 是一个草案代码,标记为 "multihash"。
	Skein256_32 Code = 0xb304 // skein256-32

	// Skein256_40 是一个草案代码,标记为 "multihash"。
	Skein256_40 Code = 0xb305 // skein256-40

	// Skein256_48 是一个草案代码,标记为 "multihash"。
	Skein256_48 Code = 0xb306 // skein256-48

	// Skein256_56 是一个草案代码,标记为 "multihash"。
	Skein256_56 Code = 0xb307 // skein256-56

	// Skein256_64 是一个草案代码,标记为 "multihash"。
	Skein256_64 Code = 0xb308 // skein256-64

	// Skein256_72 是一个草案代码,标记为 "multihash"。
	Skein256_72 Code = 0xb309 // skein256-72

	// Skein256_80 是一个草案代码,标记为 "multihash"。
	Skein256_80 Code = 0xb30a // skein256-80

	// Skein256_88 是一个草案代码,标记为 "multihash"。
	Skein256_88 Code = 0xb30b // skein256-88

	// Skein256_96 是一个草案代码,标记为 "multihash"。
	Skein256_96 Code = 0xb30c // skein256-96

	// Skein256_104 是一个草案代码,标记为 "multihash"。
	Skein256_104 Code = 0xb30d // skein256-104

	// Skein256_112 是一个草案代码,标记为 "multihash"。
	Skein256_112 Code = 0xb30e // skein256-112

	// Skein256_120 是一个草案代码,标记为 "multihash"。
	Skein256_120 Code = 0xb30f // skein256-120

	// Skein256_128 是一个草案代码,标记为 "multihash"。
	Skein256_128 Code = 0xb310 // skein256-128

	// Skein256_136 是一个草案代码,标记为 "multihash"。
	Skein256_136 Code = 0xb311 // skein256-136

	// Skein256_144 是一个草案代码,标记为 "multihash"。
	Skein256_144 Code = 0xb312 // skein256-144

	// Skein256_152 是一个草案代码,标记为 "multihash"。
	Skein256_152 Code = 0xb313 // skein256-152

	// Skein256_160 是一个草案代码,标记为 "multihash"。
	Skein256_160 Code = 0xb314 // skein256-160

	// Skein256_168 是一个草案代码,标记为 "multihash"。
	Skein256_168 Code = 0xb315 // skein256-168

	// Skein256_176 是一个草案代码,标记为 "multihash"。
	Skein256_176 Code = 0xb316 // skein256-176

	// Skein256_184 是一个草案代码,标记为 "multihash"。
	Skein256_184 Code = 0xb317 // skein256-184

	// Skein256_192 是一个草案代码,标记为 "multihash"。
	Skein256_192 Code = 0xb318 // skein256-192

	// Skein256_200 是一个草案代码,标记为 "multihash"。
	Skein256_200 Code = 0xb319 // skein256-200

	// Skein256_208 是一个草案代码,标记为 "multihash"。
	Skein256_208 Code = 0xb31a // skein256-208

	// Skein256_216 是一个草案代码,标记为 "multihash"。
	// Skein256_216 is a draft code tagged "multihash".
	Skein256_216 Code = 0xb31b // skein256-216
	// Skein256_224 是一个草案代码,标记为 "multihash"。
	Skein256_224 Code = 0xb31c // skein256-224

	// Skein256_232 是一个草案代码,标记为 "multihash"。
	Skein256_232 Code = 0xb31d // skein256-232

	// Skein256_240 是一个草案代码,标记为 "multihash"。
	Skein256_240 Code = 0xb31e // skein256-240

	// Skein256_248 是一个草案代码,标记为 "multihash"。
	Skein256_248 Code = 0xb31f // skein256-248

	// Skein256_256 是一个草案代码,标记为 "multihash"。
	Skein256_256 Code = 0xb320 // skein256-256

	// Skein512_8 是一个草案代码,标记为 "multihash",描述为:Skein512 包含 64 个输出长度,每个长度产生不同的哈希值。
	Skein512_8 Code = 0xb321 // skein512-8

	// Skein512_16 是一个草案代码,标记为 "multihash"。
	Skein512_16 Code = 0xb322 // skein512-16

	// Skein512_24 是一个草案代码,标记为 "multihash"。
	Skein512_24 Code = 0xb323 // skein512-24

	// Skein512_32 是一个草案代码,标记为 "multihash"。
	Skein512_32 Code = 0xb324 // skein512-32

	// Skein512_40 是一个草案代码,标记为 "multihash"。
	Skein512_40 Code = 0xb325 // skein512-40

	// Skein512_48 是一个草案代码,标记为 "multihash"。
	Skein512_48 Code = 0xb326 // skein512-48

	// Skein512_56 是一个草案代码,标记为 "multihash"。
	Skein512_56 Code = 0xb327 // skein512-56

	// Skein512_64 是一个草案代码,标记为 "multihash"。
	Skein512_64 Code = 0xb328 // skein512-64

	// Skein512_72 是一个草案代码,标记为 "multihash"。
	Skein512_72 Code = 0xb329 // skein512-72

	// Skein512_80 是一个草案代码,标记为 "multihash"。
	Skein512_80 Code = 0xb32a // skein512-80

	// Skein512_88 是一个草案代码,标记为 "multihash"。
	Skein512_88 Code = 0xb32b // skein512-88

	// Skein512_96 是一个草案代码,标记为 "multihash"。
	Skein512_96 Code = 0xb32c // skein512-96

	// Skein512_104 是一个草案代码,标记为 "multihash"。
	Skein512_104 Code = 0xb32d // skein512-104

	// Skein512_112 是一个草案代码,标记为 "multihash"。
	Skein512_112 Code = 0xb32e // skein512-112

	// Skein512_120 是一个草案代码,标记为 "multihash"。
	Skein512_120 Code = 0xb32f // skein512-120

	// Skein512_128 是一个草案代码,标记为 "multihash"。
	Skein512_128 Code = 0xb330 // skein512-128

	// Skein512_136 是一个草案代码,标记为 "multihash"。
	Skein512_136 Code = 0xb331 // skein512-136

	// Skein512_144 是一个草案代码,标记为 "multihash"。
	Skein512_144 Code = 0xb332 // skein512-144

	// Skein512_152 是一个草案代码,标记为 "multihash"。
	Skein512_152 Code = 0xb333 // skein512-152

	// Skein512_160 是一个草案代码,标记为 "multihash"。
	Skein512_160 Code = 0xb334 // skein512-160

	// Skein512_168 是一个草案代码,标记为 "multihash"。
	Skein512_168 Code = 0xb335 // skein512-168

	// Skein512_176 是一个草案代码,标记为 "multihash"。
	Skein512_176 Code = 0xb336 // skein512-176

	// Skein512_184 是一个草案代码,标记为 "multihash"。
	Skein512_184 Code = 0xb337 // skein512-184

	// Skein512_192 是一个草案代码,标记为 "multihash"。
	Skein512_192 Code = 0xb338 // skein512-192

	// Skein512_200 是一个草案代码,标记为 "multihash"。
	Skein512_200 Code = 0xb339 // skein512-200

	// Skein512_208 是一个草案代码,标记为 "multihash"。
	Skein512_208 Code = 0xb33a // skein512-208

	// Skein512_216 是一个草案代码,标记为 "multihash"。
	Skein512_216 Code = 0xb33b // skein512-216

	// Skein512_224 是一个草案代码,标记为 "multihash"。
	Skein512_224 Code = 0xb33c // skein512-224

	// Skein512_232 是一个草案代码,标记为 "multihash"。
	Skein512_232 Code = 0xb33d // skein512-232

	// Skein512_240 是一个草案代码,标记为 "multihash"。
	Skein512_240 Code = 0xb33e // skein512-240

	// Skein512_248 是一个草案代码,标记为 "multihash"。
	Skein512_248 Code = 0xb33f // skein512-248

	// Skein512_256 是一个草案代码,标记为 "multihash"。
	Skein512_256 Code = 0xb340 // skein512-256

	// Skein512_264 是一个草案代码,标记为 "multihash"。
	Skein512_264 Code = 0xb341 // skein512-264

	// Skein512_272 是一个草案代码,标记为 "multihash"。
	Skein512_272 Code = 0xb342 // skein512-272

	// Skein512_280 是一个草案代码,标记为 "multihash"。
	Skein512_280 Code = 0xb343 // skein512-280

	// Skein512_288 是一个草案代码,标记为 "multihash"。
	Skein512_288 Code = 0xb344 // skein512-288

	// Skein512_296 是一个草案代码,标记为 "multihash"。
	Skein512_296 Code = 0xb345 // skein512-296

	// Skein512_304 是一个草案代码,标记为 "multihash"。
	Skein512_304 Code = 0xb346 // skein512-304

	// Skein512_312 是一个草案代码,标记为 "multihash"。
	Skein512_312 Code = 0xb347 // skein512-312

	// Skein512_320 是一个草案代码,标记为 "multihash"。
	Skein512_320 Code = 0xb348 // skein512-320

	// Skein512_328 是一个草案代码,标记为 "multihash"。
	Skein512_328 Code = 0xb349 // skein512-328

	// Skein512_336 是一个草案代码,标记为 "multihash"。
	Skein512_336 Code = 0xb34a // skein512-336

	// Skein512_344 是一个草案代码,标记为 "multihash"。
	Skein512_344 Code = 0xb34b // skein512-344

	// Skein512_352 是一个草案代码,标记为 "multihash"。
	Skein512_352 Code = 0xb34c // skein512-352

	// Skein512_360 是一个草案代码,标记为 "multihash"。
	Skein512_360 Code = 0xb34d // skein512-360

	// Skein512_368 是一个草案代码,标记为 "multihash"。
	Skein512_368 Code = 0xb34e // skein512-368

	// Skein512_376 是一个草案代码,标记为 "multihash"。
	Skein512_376 Code = 0xb34f // skein512-376

	// Skein512_384 是一个草案代码,标记为 "multihash"。
	Skein512_384 Code = 0xb350 // skein512-384

	// Skein512_392 是一个草案代码,标记为 "multihash"。
	Skein512_392 Code = 0xb351 // skein512-392

	// Skein512_400 是一个草案代码,标记为 "multihash"。
	Skein512_400 Code = 0xb352 // skein512-400

	// Skein512_408 是一个草案代码,标记为 "multihash"。
	Skein512_408 Code = 0xb353 // skein512-408

	// Skein512_416 是一个草案代码,标记为 "multihash"。
	Skein512_416 Code = 0xb354 // skein512-416

	// Skein512_424 是一个草案代码,标记为 "multihash"。
	Skein512_424 Code = 0xb355 // skein512-424

	// Skein512_432 是一个草案代码,标记为 "multihash"。
	Skein512_432 Code = 0xb356 // skein512-432

	// Skein512_440 是一个草案代码,标记为 "multihash"。
	Skein512_440 Code = 0xb357 // skein512-440

	// Skein512_448 是一个草案代码,标记为 "multihash"。
	Skein512_448 Code = 0xb358 // skein512-448

	// Skein512_456 是一个草案代码,标记为 "multihash"。
	Skein512_456 Code = 0xb359 // skein512-456

	// Skein512_464 是一个草案代码,标记为 "multihash"。
	Skein512_464 Code = 0xb35a // skein512-464

	// Skein512_472 是一个草案代码,标记为 "multihash"。
	Skein512_472 Code = 0xb35b // skein512-472

	// Skein512_480 是一个草案代码,标记为 "multihash"。
	Skein512_480 Code = 0xb35c // skein512-480

	// Skein512_488 是一个草案代码,标记为 "multihash"。
	Skein512_488 Code = 0xb35d // skein512-488

	// Skein512_496 是一个草案代码,标记为 "multihash"。
	Skein512_496 Code = 0xb35e // skein512-496

	// Skein512_504 是一个草案代码,标记为 "multihash"。
	Skein512_504 Code = 0xb35f // skein512-504

	// Skein512_512 是一个草案代码,标记为 "multihash"。
	Skein512_512 Code = 0xb360 // skein512-512

	// Skein1024_8 是一个草案代码,标记为 "multihash",描述为:Skein1024 包含 128 个输出长度,每个长度产生不同的哈希值。
	Skein1024_8 Code = 0xb361 // skein1024-8

	// Skein1024_16 是一个草案代码,标记为 "multihash"。
	Skein1024_16 Code = 0xb362 // skein1024-16

	// Skein1024_24 是一个草案代码,标记为 "multihash"。
	Skein1024_24 Code = 0xb363 // skein1024-24

	// Skein1024_32 是一个草案代码,标记为 "multihash"。
	Skein1024_32 Code = 0xb364 // skein1024-32

	// Skein1024_40 是一个草案代码,标记为 "multihash"。
	Skein1024_40 Code = 0xb365 // skein1024-40

	// Skein1024_48 是一个草案代码,标记为 "multihash"。
	Skein1024_48 Code = 0xb366 // skein1024-48

	// Skein1024_56 是一个草案代码,标记为 "multihash"。
	Skein1024_56 Code = 0xb367 // skein1024-56

	// Skein1024_64 是一个草案代码,标记为 "multihash"。
	Skein1024_64 Code = 0xb368 // skein1024-64

	// Skein1024_72 是一个草案代码,标记为 "multihash"。
	Skein1024_72 Code = 0xb369 // skein1024-72

	// Skein1024_80 是一个草案代码,标记为 "multihash"。
	Skein1024_80 Code = 0xb36a // skein1024-80

	// Skein1024_88 是一个草案代码,标记为 "multihash"。
	Skein1024_88 Code = 0xb36b // skein1024-88

	// Skein1024_96 是一个草案代码,标记为 "multihash"。
	Skein1024_96 Code = 0xb36c // skein1024-96

	// Skein1024_104 是一个草案代码,标记为 "multihash"。
	Skein1024_104 Code = 0xb36d // skein1024-104

	// Skein1024_112 是一个草案代码,标记为 "multihash"。
	Skein1024_112 Code = 0xb36e // skein1024-112

	// Skein1024_120 是一个草案代码,标记为 "multihash"。
	Skein1024_120 Code = 0xb36f // skein1024-120

	// Skein1024_128 是一个草案代码,标记为 "multihash"。
	Skein1024_128 Code = 0xb370 // skein1024-128

	// Skein1024_136 是一个草案代码,标记为 "multihash"。
	Skein1024_136 Code = 0xb371 // skein1024-136

	// Skein1024_144 是一个草案代码,标记为 "multihash"。
	Skein1024_144 Code = 0xb372 // skein1024-144

	// Skein1024_152 是一个草案代码,标记为 "multihash"。
	Skein1024_152 Code = 0xb373 // skein1024-152

	// Skein1024_160 是一个草案代码,标记为 "multihash"。
	Skein1024_160 Code = 0xb374 // skein1024-160

	// Skein1024_168 是一个草案代码,标记为 "multihash"。
	Skein1024_168 Code = 0xb375 // skein1024-168

	// Skein1024_176 是一个草案代码,标记为 "multihash"。
	Skein1024_176 Code = 0xb376 // skein1024-176

	// Skein1024_184 是一个草案代码,标记为 "multihash"。
	Skein1024_184 Code = 0xb377 // skein1024-184

	// Skein1024_192 是一个草案代码,标记为 "multihash"。
	Skein1024_192 Code = 0xb378 // skein1024-192

	// Skein1024_200 是一个草案代码,标记为 "multihash"。
	Skein1024_200 Code = 0xb379 // skein1024-200

	// Skein1024_208 是一个草案代码,标记为 "multihash"。
	Skein1024_208 Code = 0xb37a // skein1024-208

	// Skein1024_216 是一个草案代码,标记为 "multihash"。
	Skein1024_216 Code = 0xb37b // skein1024-216

	// Skein1024_224 是一个草案代码,标记为 "multihash"。
	Skein1024_224 Code = 0xb37c // skein1024-224

	// Skein1024_232 是一个草案代码,标记为 "multihash"。
	Skein1024_232 Code = 0xb37d // skein1024-232

	// Skein1024_240 是一个草案代码,标记为 "multihash"。
	Skein1024_240 Code = 0xb37e // skein1024-240

	// Skein1024_248 是一个草案代码,标记为 "multihash"。
	Skein1024_248 Code = 0xb37f // skein1024-248

	// Skein1024_256 是一个草案代码,标记为 "multihash"。
	Skein1024_256 Code = 0xb380 // skein1024-256

	// Skein1024_264 是一个草案代码,标记为 "multihash"。
	Skein1024_264 Code = 0xb381 // skein1024-264

	// Skein1024_272 是一个草案代码,标记为 "multihash"。
	Skein1024_272 Code = 0xb382 // skein1024-272

	// Skein1024_280 是一个草案代码,标记为 "multihash"。
	Skein1024_280 Code = 0xb383 // skein1024-280

	// Skein1024_288 是一个草案代码,标记为 "multihash"。
	Skein1024_288 Code = 0xb384 // skein1024-288

	// Skein1024_296 是一个草案代码,标记为 "multihash"。
	Skein1024_296 Code = 0xb385 // skein1024-296

	// Skein1024_304 是一个草案代码,标记为 "multihash"。
	Skein1024_304 Code = 0xb386 // skein1024-304

	// Skein1024_312 是一个草案代码,标记为 "multihash"。
	Skein1024_312 Code = 0xb387 // skein1024-312

	// Skein1024_320 是一个草案代码,标记为 "multihash"。
	Skein1024_320 Code = 0xb388 // skein1024-320

	// Skein1024_328 是一个草案代码,标记为 "multihash"。
	Skein1024_328 Code = 0xb389 // skein1024-328

	// Skein1024_336 是一个草案代码,标记为 "multihash"。
	Skein1024_336 Code = 0xb38a // skein1024-336

	// Skein1024_344 是一个草案代码,标记为 "multihash"。
	Skein1024_344 Code = 0xb38b // skein1024-344

	// Skein1024_352 是一个草案代码,标记为 "multihash"。
	Skein1024_352 Code = 0xb38c // skein1024-352

	// Skein1024_360 是一个草案代码,标记为 "multihash"。
	Skein1024_360 Code = 0xb38d // skein1024-360

	// Skein1024_368 是一个草案代码,标记为 "multihash"。
	Skein1024_368 Code = 0xb38e // skein1024-368

	// Skein1024_376 是一个草案代码,标记为 "multihash"。
	Skein1024_376 Code = 0xb38f // skein1024-376

	// Skein1024_384 是一个草案代码,标记为 "multihash"。
	Skein1024_384 Code = 0xb390 // skein1024-384

	// Skein1024_392 是一个草案代码,标记为 "multihash"。
	Skein1024_392 Code = 0xb391 // skein1024-392

	// Skein1024_400 是一个草案代码,标记为 "multihash"。
	Skein1024_400 Code = 0xb392 // skein1024-400

	// Skein1024_408 是一个草案代码,标记为 "multihash"。
	Skein1024_408 Code = 0xb393 // skein1024-408

	// Skein1024_416 是一个草案代码,标记为 "multihash"。
	Skein1024_416 Code = 0xb394 // skein1024-416

	// Skein1024_424 是一个草案代码,标记为 "multihash"。
	Skein1024_424 Code = 0xb395 // skein1024-424

	// Skein1024_432 是一个草案代码,标记为 "multihash"。
	Skein1024_432 Code = 0xb396 // skein1024-432

	// Skein1024_440 是一个草案代码,标记为 "multihash"。
	Skein1024_440 Code = 0xb397 // skein1024-440

	// Skein1024_448 是一个草案代码,标记为 "multihash"。
	Skein1024_448 Code = 0xb398 // skein1024-448

	// Skein1024_456 是一个草案代码,标记为 "multihash"。
	Skein1024_456 Code = 0xb399 // skein1024-456

	// Skein1024_464 是一个草案代码,标记为 "multihash"。
	Skein1024_464 Code = 0xb39a // skein1024-464

	// Skein1024_472 是一个草案代码,标记为 "multihash"。
	Skein1024_472 Code = 0xb39b // skein1024-472

	// Skein1024_480 是一个草案代码,标记为 "multihash"。
	Skein1024_480 Code = 0xb39c // skein1024-480

	// Skein1024_488 是一个草案代码,标记为 "multihash"。
	Skein1024_488 Code = 0xb39d // skein1024-488

	// Skein1024_496 是一个草案代码,标记为 "multihash"。
	Skein1024_496 Code = 0xb39e // skein1024-496

	// Skein1024_504 是一个草案代码,标记为 "multihash"。
	Skein1024_504 Code = 0xb39f // skein1024-504

	// Skein1024_512 是一个草案代码,标记为 "multihash"。
	Skein1024_512 Code = 0xb3a0 // skein1024-512

	// Skein1024_520 是一个草案代码,标记为 "multihash"。
	Skein1024_520 Code = 0xb3a1 // skein1024-520

	// Skein1024_528 是一个草案代码,标记为 "multihash"。
	Skein1024_528 Code = 0xb3a2 // skein1024-528

	// Skein1024_536 是一个草案代码,标记为 "multihash"。
	Skein1024_536 Code = 0xb3a3 // skein1024-536

	// Skein1024_544 是一个草案代码,标记为 "multihash"。
	Skein1024_544 Code = 0xb3a4 // skein1024-544

	// Skein1024_552 是一个草案代码,标记为 "multihash"。
	Skein1024_552 Code = 0xb3a5 // skein1024-552

	// Skein1024_560 是一个草案代码,标记为 "multihash"。
	Skein1024_560 Code = 0xb3a6 // skein1024-560

	// Skein1024_568 是一个草案代码,标记为 "multihash"。
	Skein1024_568 Code = 0xb3a7 // skein1024-568

	// Skein1024_576 是一个草案代码,标记为 "multihash"。
	Skein1024_576 Code = 0xb3a8 // skein1024-576

	// Skein1024_584 是一个草案代码,标记为 "multihash"。
	Skein1024_584 Code = 0xb3a9 // skein1024-584

	// Skein1024_592 是一个草案代码,标记为 "multihash"。
	Skein1024_592 Code = 0xb3aa // skein1024-592

	// Skein1024_600 是一个草案代码,标记为 "multihash"。
	Skein1024_600 Code = 0xb3ab // skein1024-600

	// Skein1024_608 是一个草案代码,标记为 "multihash"。
	Skein1024_608 Code = 0xb3ac // skein1024-608

	// Skein1024_616 是一个草案代码,标记为 "multihash"。
	Skein1024_616 Code = 0xb3ad // skein1024-616

	// Skein1024_624 是一个草案代码,标记为 "multihash"。
	Skein1024_624 Code = 0xb3ae // skein1024-624

	// Skein1024_632 是一个草案代码,标记为 "multihash"。
	Skein1024_632 Code = 0xb3af // skein1024-632

	// Skein1024_640 是一个草案代码,标记为 "multihash"。
	Skein1024_640 Code = 0xb3b0 // skein1024-640

	// Skein1024_648 是一个草案代码,标记为 "multihash"。
	Skein1024_648 Code = 0xb3b1 // skein1024-648

	// Skein1024_656 is a draft code tagged "multihash".
	Skein1024_656 Code = 0xb3b2 // skein1024-656

	// Skein1024_664 is a draft code tagged "multihash".
	Skein1024_664 Code = 0xb3b3 // skein1024-664

	// Skein1024_672 is a draft code tagged "multihash".
	Skein1024_672 Code = 0xb3b4 // skein1024-672

	// Skein1024_680 is a draft code tagged "multihash".
	Skein1024_680 Code = 0xb3b5 // skein1024-680

	// Skein1024_688 is a draft code tagged "multihash".
	Skein1024_688 Code = 0xb3b6 // skein1024-688

	// Skein1024_696 is a draft code tagged "multihash".
	Skein1024_696 Code = 0xb3b7 // skein1024-696

	// Skein1024_704 is a draft code tagged "multihash".
	Skein1024_704 Code = 0xb3b8 // skein1024-704

	// Skein1024_712 is a draft code tagged "multihash".
	Skein1024_712 Code = 0xb3b9 // skein1024-712

	// Skein1024_720 is a draft code tagged "multihash".
	Skein1024_720 Code = 0xb3ba // skein1024-720

	// Skein1024_728 is a draft code tagged "multihash".
	Skein1024_728 Code = 0xb3bb // skein1024-728

	// Skein1024_736 is a draft code tagged "multihash".
	Skein1024_736 Code = 0xb3bc // skein1024-736

	// Skein1024_744 is a draft code tagged "multihash".
	Skein1024_744 Code = 0xb3bd // skein1024-744

	// Skein1024_752 is a draft code tagged "multihash".
	Skein1024_752 Code = 0xb3be // skein1024-752

	// Skein1024_760 is a draft code tagged "multihash".
	Skein1024_760 Code = 0xb3bf // skein1024-760

	// Skein1024_768 is a draft code tagged "multihash".
	Skein1024_768 Code = 0xb3c0 // skein1024-768

	// Skein1024_776 is a draft code tagged "multihash".
	Skein1024_776 Code = 0xb3c1 // skein1024-776

	// Skein1024_784 is a draft code tagged "multihash".
	Skein1024_784 Code = 0xb3c2 // skein1024-784

	// Skein1024_792 is a draft code tagged "multihash".
	Skein1024_792 Code = 0xb3c3 // skein1024-792

	// Skein1024_800 is a draft code tagged "multihash".
	Skein1024_800 Code = 0xb3c4 // skein1024-800

	// Skein1024_808 is a draft code tagged "multihash".
	Skein1024_808 Code = 0xb3c5 // skein1024-808

	// Skein1024_816 is a draft code tagged "multihash".
	Skein1024_816 Code = 0xb3c6 // skein1024-816

	// Skein1024_824 is a draft code tagged "multihash".
	Skein1024_824 Code = 0xb3c7 // skein1024-824

	// Skein1024_832 is a draft code tagged "multihash".
	Skein1024_832 Code = 0xb3c8 // skein1024-832

	// Skein1024_840 is a draft code tagged "multihash".
	Skein1024_840 Code = 0xb3c9 // skein1024-840

	// Skein1024_848 is a draft code tagged "multihash".
	Skein1024_848 Code = 0xb3ca // skein1024-848

	// Skein1024_856 is a draft code tagged "multihash".
	Skein1024_856 Code = 0xb3cb // skein1024-856

	// Skein1024_864 is a draft code tagged "multihash".
	Skein1024_864 Code = 0xb3cc // skein1024-864

	// Skein1024_872 is a draft code tagged "multihash".
	Skein1024_872 Code = 0xb3cd // skein1024-872

	// Skein1024_880 is a draft code tagged "multihash".
	Skein1024_880 Code = 0xb3ce // skein1024-880

	// Skein1024_888 is a draft code tagged "multihash".
	Skein1024_888 Code = 0xb3cf // skein1024-888

	// Skein1024_896 is a draft code tagged "multihash".
	Skein1024_896 Code = 0xb3d0 // skein1024-896

	// Skein1024_904 is a draft code tagged "multihash".
	Skein1024_904 Code = 0xb3d1 // skein1024-904

	// Skein1024_912 is a draft code tagged "multihash".
	Skein1024_912 Code = 0xb3d2 // skein1024-912

	// Skein1024_920 is a draft code tagged "multihash".
	Skein1024_920 Code = 0xb3d3 // skein1024-920

	// Skein1024_928 is a draft code tagged "multihash".
	Skein1024_928 Code = 0xb3d4 // skein1024-928

	// Skein1024_936 is a draft code tagged "multihash".
	Skein1024_936 Code = 0xb3d5 // skein1024-936

	// Skein1024_944 is a draft code tagged "multihash".
	Skein1024_944 Code = 0xb3d6 // skein1024-944

	// Skein1024_952 is a draft code tagged "multihash".
	Skein1024_952 Code = 0xb3d7 // skein1024-952

	// Skein1024_960 is a draft code tagged "multihash".
	Skein1024_960 Code = 0xb3d8 // skein1024-960

	// Skein1024_968 is a draft code tagged "multihash".
	Skein1024_968 Code = 0xb3d9 // skein1024-968

	// Skein1024_976 is a draft code tagged "multihash".
	Skein1024_976 Code = 0xb3da // skein1024-976

	// Skein1024_984 is a draft code tagged "multihash".
	Skein1024_984 Code = 0xb3db // skein1024-984

	// Skein1024_992 is a draft code tagged "multihash".
	Skein1024_992 Code = 0xb3dc // skein1024-992

	// Skein1024_1000 is a draft code tagged "multihash".
	Skein1024_1000 Code = 0xb3dd // skein1024-1000

	// Skein1024_1008 is a draft code tagged "multihash".
	Skein1024_1008 Code = 0xb3de // skein1024-1008

	// Skein1024_1016 is a draft code tagged "multihash".
	Skein1024_1016 Code = 0xb3df // skein1024-1016

	// Skein1024_1024 is a draft code tagged "multihash".
	Skein1024_1024 Code = 0xb3e0 // skein1024-1024

	// Xxh32 is a draft code tagged "hash" and described by: Extremely fast non-cryptographic hash algorithm.
	Xxh32 Code = 0xb3e1 // xxh-32

	// Xxh64 is a draft code tagged "hash" and described by: Extremely fast non-cryptographic hash algorithm.
	Xxh64 Code = 0xb3e2 // xxh-64

	// Xxh3_64 is a draft code tagged "hash" and described by: Extremely fast non-cryptographic hash algorithm.
	Xxh3_64 Code = 0xb3e3 // xxh3-64

	// Xxh3_128 is a draft code tagged "hash" and described by: Extremely fast non-cryptographic hash algorithm.
	Xxh3_128 Code = 0xb3e4 // xxh3-128

	// PoseidonBls12_381A2Fc1 is a permanent code tagged "multihash" and described by: Poseidon using BLS12-381 and arity of 2 with Filecoin parameters.
	PoseidonBls12_381A2Fc1 Code = 0xb401 // poseidon-bls12_381-a2-fc1

	// PoseidonBls12_381A2Fc1Sc is a draft code tagged "multihash" and described by: Poseidon using BLS12-381 and arity of 2 with Filecoin parameters - high-security variant.
	PoseidonBls12_381A2Fc1Sc Code = 0xb402 // poseidon-bls12_381-a2-fc1-sc

	// Urdca2015Canon is a draft code tagged "ipld" and described by: The result of canonicalizing an input according to URDCA-2015 and then expressing its hash value as a multihash value..
	Urdca2015Canon Code = 0xb403 // urdca-2015-canon

	// Ssz is a draft code tagged "serialization" and described by: SimpleSerialize (SSZ) serialization.
	Ssz Code = 0xb501 // ssz

	// SszSha2_256Bmt is a draft code tagged "multihash" and described by: SSZ Merkle tree root using SHA2-256 as the hashing function and SSZ serialization for the block binary.
	SszSha2_256Bmt Code = 0xb502 // ssz-sha2-256-bmt

	// JsonJcs is a draft code tagged "ipld" and described by: The result of canonicalizing an input according to JCS - JSON Canonicalisation Scheme (RFC 8785).
	JsonJcs Code = 0xb601 // json-jcs

	// Iscc is a draft code tagged "softhash" and described by: ISCC (International Standard Content Code) - similarity preserving hash.
	Iscc Code = 0xcc01 // iscc

	// ZeroxcertImprint256 is a draft code tagged "zeroxcert" and described by: 0xcert Asset Imprint (root hash).
	ZeroxcertImprint256 Code = 0xce11 // zeroxcert-imprint-256

	// Varsig is a draft code tagged "varsig" and described by: Namespace for all not yet standard signature algorithms.
	Varsig Code = 0xd000 // varsig

	// Es256k is a draft code tagged "varsig" and described by: ES256K Siganture Algorithm (secp256k1).
	Es256k Code = 0xd0e7 // es256k

	// Bls12381G1Sig is a draft code tagged "varsig" and described by: G1 signature for BLS-12381-G2.
	Bls12381G1Sig Code = 0xd0ea // bls-12381-g1-sig

	// Bls12381G2Sig is a draft code tagged "varsig" and described by: G2 signature for BLS-12381-G1.
	Bls12381G2Sig Code = 0xd0eb // bls-12381-g2-sig

	// Eddsa is a draft code tagged "varsig" and described by: Edwards-Curve Digital Signature Algorithm.
	Eddsa Code = 0xd0ed // eddsa

	// Eip191 is a draft code tagged "varsig" and described by: EIP-191 Ethereum Signed Data Standard.
	Eip191 Code = 0xd191 // eip-191

	// Jwk_jcsPub is a draft code tagged "key" and described by: JSON object containing only the required members of a JWK (RFC 7518 and RFC 7517) representing the public key. Serialisation based on JCS (RFC 8785).
	Jwk_jcsPub Code = 0xeb51 // jwk_jcs-pub

	// FilCommitmentUnsealed is a permanent code tagged "filecoin" and described by: Filecoin piece or sector data commitment merkle node/root (CommP & CommD).
	FilCommitmentUnsealed Code = 0xf101 // fil-commitment-unsealed

	// FilCommitmentSealed is a permanent code tagged "filecoin" and described by: Filecoin sector data commitment merkle node/root - sealed and replicated (CommR).
	FilCommitmentSealed Code = 0xf102 // fil-commitment-sealed

	// Plaintextv2 is a draft code tagged "multiaddr".
	Plaintextv2 Code = 0x706c61 // plaintextv2

	// HolochainAdrV0 is a draft code tagged "holochain" and described by: Holochain v0 address    + 8 R-S (63 x Base-32).
	HolochainAdrV0 Code = 0x807124 // holochain-adr-v0

	// HolochainAdrV1 is a draft code tagged "holochain" and described by: Holochain v1 address    + 8 R-S (63 x Base-32).
	HolochainAdrV1 Code = 0x817124 // holochain-adr-v1

	// HolochainKeyV0 is a draft code tagged "holochain" and described by: Holochain v0 public key + 8 R-S (63 x Base-32).
	HolochainKeyV0 Code = 0x947124 // holochain-key-v0

	// HolochainKeyV1 is a draft code tagged "holochain" and described by: Holochain v1 public key + 8 R-S (63 x Base-32).
	HolochainKeyV1 Code = 0x957124 // holochain-key-v1

	// HolochainSigV0 is a draft code tagged "holochain" and described by: Holochain v0 signature  + 8 R-S (63 x Base-32).
	HolochainSigV0 Code = 0xa27124 // holochain-sig-v0

	// HolochainSigV1 is a draft code tagged "holochain" and described by: Holochain v1 signature  + 8 R-S (63 x Base-32).
	HolochainSigV1 Code = 0xa37124 // holochain-sig-v1

	// SkynetNs is a draft code tagged "namespace" and described by: Skynet Namespace.
	SkynetNs Code = 0xb19910 // skynet-ns

	// ArweaveNs is a draft code tagged "namespace" and described by: Arweave Namespace.
	ArweaveNs Code = 0xb29910 // arweave-ns

	// SubspaceNs is a draft code tagged "namespace" and described by: Subspace Network Namespace.
	SubspaceNs Code = 0xb39910 // subspace-ns

	// KumandraNs is a draft code tagged "namespace" and described by: Kumandra Network Namespace.
	KumandraNs Code = 0xb49910 // kumandra-ns

	// Es256 is a draft code tagged "varsig" and described by: ES256 Signature Algorithm.
	Es256 Code = 0xd01200 // es256

	// Es284 is a draft code tagged "varsig" and described by: ES384 Signature Algorithm.
	Es284 Code = 0xd01201 // es284

	// Es512 is a draft code tagged "varsig" and described by: ES512 Signature Algorithm.
	Es512 Code = 0xd01202 // es512

	// Rs256 is a draft code tagged "varsig" and described by: RS256 Signature Algorithm.
	Rs256 Code = 0xd01205 // rs256
)

var knownCodes = []Code{
	Identity,
	Cidv1,
	Cidv2,
	Cidv3,
	Ip4,
	Tcp,
	Sha1,
	Sha2_256,
	Sha2_512,
	Sha3_512,
	Sha3_384,
	Sha3_256,
	Sha3_224,
	Shake128,
	Shake256,
	Keccak224,
	Keccak256,
	Keccak384,
	Keccak512,
	Blake3,
	Sha2_384,
	Dccp,
	Murmur3X64_64,
	Murmur3_32,
	Ip6,
	Ip6zone,
	Ipcidr,
	Path,
	Multicodec,
	Multihash,
	Multiaddr,
	Multibase,
	Dns,
	Dns4,
	Dns6,
	Dnsaddr,
	Protobuf,
	Cbor,
	Raw,
	DblSha2_256,
	Rlp,
	Bencode,
	DagPb,
	DagCbor,
	Libp2pKey,
	GitRaw,
	TorrentInfo,
	TorrentFile,
	LeofcoinBlock,
	LeofcoinTx,
	LeofcoinPr,
	Sctp,
	DagJose,
	DagCose,
	EthBlock,
	EthBlockList,
	EthTxTrie,
	EthTx,
	EthTxReceiptTrie,
	EthTxReceipt,
	EthStateTrie,
	EthAccountSnapshot,
	EthStorageTrie,
	EthReceiptLogTrie,
	EthRecieptLog,
	Aes128,
	Aes192,
	Aes256,
	Chacha128,
	Chacha256,
	BitcoinBlock,
	BitcoinTx,
	BitcoinWitnessCommitment,
	ZcashBlock,
	ZcashTx,
	Caip50,
	Streamid,
	StellarBlock,
	StellarTx,
	Md4,
	Md5,
	DecredBlock,
	DecredTx,
	Ipld,
	Ipfs,
	Swarm,
	Ipns,
	Zeronet,
	Secp256k1Pub,
	Dnslink,
	Bls12_381G1Pub,
	Bls12_381G2Pub,
	X25519Pub,
	Ed25519Pub,
	Bls12_381G1g2Pub,
	Sr25519Pub,
	DashBlock,
	DashTx,
	SwarmManifest,
	SwarmFeed,
	Beeson,
	Udp,
	P2pWebrtcStar,
	P2pWebrtcDirect,
	P2pStardust,
	WebrtcDirect,
	Webrtc,
	P2pCircuit,
	DagJson,
	Udt,
	Utp,
	Crc32,
	Crc64Ecma,
	Unix,
	Thread,
	P2p,
	Https,
	Onion,
	Onion3,
	Garlic64,
	Garlic32,
	Tls,
	Sni,
	Noise,
	Quic,
	QuicV1,
	Webtransport,
	Certhash,
	Ws,
	Wss,
	P2pWebsocketStar,
	Http,
	Swhid1Snp,
	Json,
	Messagepack,
	Car,
	IpnsRecord,
	Libp2pPeerRecord,
	Libp2pRelayRsvp,
	Memorytransport,
	CarIndexSorted,
	CarMultihashIndexSorted,
	TransportBitswap,
	TransportGraphsyncFilecoinv1,
	TransportIpfsGatewayHttp,
	Multidid,
	Sha2_256Trunc254Padded,
	Sha2_224,
	Sha2_512_224,
	Sha2_512_256,
	Murmur3X64_128,
	Ripemd128,
	Ripemd160,
	Ripemd256,
	Ripemd320,
	X11,
	P256Pub,
	P384Pub,
	P521Pub,
	Ed448Pub,
	X448Pub,
	RsaPub,
	Sm2Pub,
	Ed25519Priv,
	Secp256k1Priv,
	X25519Priv,
	Sr25519Priv,
	RsaPriv,
	P256Priv,
	P384Priv,
	P521Priv,
	Kangarootwelve,
	AesGcm256,
	Silverpine,
	Sm3_256,
	Blake2b8,
	Blake2b16,
	Blake2b24,
	Blake2b32,
	Blake2b40,
	Blake2b48,
	Blake2b56,
	Blake2b64,
	Blake2b72,
	Blake2b80,
	Blake2b88,
	Blake2b96,
	Blake2b104,
	Blake2b112,
	Blake2b120,
	Blake2b128,
	Blake2b136,
	Blake2b144,
	Blake2b152,
	Blake2b160,
	Blake2b168,
	Blake2b176,
	Blake2b184,
	Blake2b192,
	Blake2b200,
	Blake2b208,
	Blake2b216,
	Blake2b224,
	Blake2b232,
	Blake2b240,
	Blake2b248,
	Blake2b256,
	Blake2b264,
	Blake2b272,
	Blake2b280,
	Blake2b288,
	Blake2b296,
	Blake2b304,
	Blake2b312,
	Blake2b320,
	Blake2b328,
	Blake2b336,
	Blake2b344,
	Blake2b352,
	Blake2b360,
	Blake2b368,
	Blake2b376,
	Blake2b384,
	Blake2b392,
	Blake2b400,
	Blake2b408,
	Blake2b416,
	Blake2b424,
	Blake2b432,
	Blake2b440,
	Blake2b448,
	Blake2b456,
	Blake2b464,
	Blake2b472,
	Blake2b480,
	Blake2b488,
	Blake2b496,
	Blake2b504,
	Blake2b512,
	Blake2s8,
	Blake2s16,
	Blake2s24,
	Blake2s32,
	Blake2s40,
	Blake2s48,
	Blake2s56,
	Blake2s64,
	Blake2s72,
	Blake2s80,
	Blake2s88,
	Blake2s96,
	Blake2s104,
	Blake2s112,
	Blake2s120,
	Blake2s128,
	Blake2s136,
	Blake2s144,
	Blake2s152,
	Blake2s160,
	Blake2s168,
	Blake2s176,
	Blake2s184,
	Blake2s192,
	Blake2s200,
	Blake2s208,
	Blake2s216,
	Blake2s224,
	Blake2s232,
	Blake2s240,
	Blake2s248,
	Blake2s256,
	Skein256_8,
	Skein256_16,
	Skein256_24,
	Skein256_32,
	Skein256_40,
	Skein256_48,
	Skein256_56,
	Skein256_64,
	Skein256_72,
	Skein256_80,
	Skein256_88,
	Skein256_96,
	Skein256_104,
	Skein256_112,
	Skein256_120,
	Skein256_128,
	Skein256_136,
	Skein256_144,
	Skein256_152,
	Skein256_160,
	Skein256_168,
	Skein256_176,
	Skein256_184,
	Skein256_192,
	Skein256_200,
	Skein256_208,
	Skein256_216,
	Skein256_224,
	Skein256_232,
	Skein256_240,
	Skein256_248,
	Skein256_256,
	Skein512_8,
	Skein512_16,
	Skein512_24,
	Skein512_32,
	Skein512_40,
	Skein512_48,
	Skein512_56,
	Skein512_64,
	Skein512_72,
	Skein512_80,
	Skein512_88,
	Skein512_96,
	Skein512_104,
	Skein512_112,
	Skein512_120,
	Skein512_128,
	Skein512_136,
	Skein512_144,
	Skein512_152,
	Skein512_160,
	Skein512_168,
	Skein512_176,
	Skein512_184,
	Skein512_192,
	Skein512_200,
	Skein512_208,
	Skein512_216,
	Skein512_224,
	Skein512_232,
	Skein512_240,
	Skein512_248,
	Skein512_256,
	Skein512_264,
	Skein512_272,
	Skein512_280,
	Skein512_288,
	Skein512_296,
	Skein512_304,
	Skein512_312,
	Skein512_320,
	Skein512_328,
	Skein512_336,
	Skein512_344,
	Skein512_352,
	Skein512_360,
	Skein512_368,
	Skein512_376,
	Skein512_384,
	Skein512_392,
	Skein512_400,
	Skein512_408,
	Skein512_416,
	Skein512_424,
	Skein512_432,
	Skein512_440,
	Skein512_448,
	Skein512_456,
	Skein512_464,
	Skein512_472,
	Skein512_480,
	Skein512_488,
	Skein512_496,
	Skein512_504,
	Skein512_512,
	Skein1024_8,
	Skein1024_16,
	Skein1024_24,
	Skein1024_32,
	Skein1024_40,
	Skein1024_48,
	Skein1024_56,
	Skein1024_64,
	Skein1024_72,
	Skein1024_80,
	Skein1024_88,
	Skein1024_96,
	Skein1024_104,
	Skein1024_112,
	Skein1024_120,
	Skein1024_128,
	Skein1024_136,
	Skein1024_144,
	Skein1024_152,
	Skein1024_160,
	Skein1024_168,
	Skein1024_176,
	Skein1024_184,
	Skein1024_192,
	Skein1024_200,
	Skein1024_208,
	Skein1024_216,
	Skein1024_224,
	Skein1024_232,
	Skein1024_240,
	Skein1024_248,
	Skein1024_256,
	Skein1024_264,
	Skein1024_272,
	Skein1024_280,
	Skein1024_288,
	Skein1024_296,
	Skein1024_304,
	Skein1024_312,
	Skein1024_320,
	Skein1024_328,
	Skein1024_336,
	Skein1024_344,
	Skein1024_352,
	Skein1024_360,
	Skein1024_368,
	Skein1024_376,
	Skein1024_384,
	Skein1024_392,
	Skein1024_400,
	Skein1024_408,
	Skein1024_416,
	Skein1024_424,
	Skein1024_432,
	Skein1024_440,
	Skein1024_448,
	Skein1024_456,
	Skein1024_464,
	Skein1024_472,
	Skein1024_480,
	Skein1024_488,
	Skein1024_496,
	Skein1024_504,
	Skein1024_512,
	Skein1024_520,
	Skein1024_528,
	Skein1024_536,
	Skein1024_544,
	Skein1024_552,
	Skein1024_560,
	Skein1024_568,
	Skein1024_576,
	Skein1024_584,
	Skein1024_592,
	Skein1024_600,
	Skein1024_608,
	Skein1024_616,
	Skein1024_624,
	Skein1024_632,
	Skein1024_640,
	Skein1024_648,
	Skein1024_656,
	Skein1024_664,
	Skein1024_672,
	Skein1024_680,
	Skein1024_688,
	Skein1024_696,
	Skein1024_704,
	Skein1024_712,
	Skein1024_720,
	Skein1024_728,
	Skein1024_736,
	Skein1024_744,
	Skein1024_752,
	Skein1024_760,
	Skein1024_768,
	Skein1024_776,
	Skein1024_784,
	Skein1024_792,
	Skein1024_800,
	Skein1024_808,
	Skein1024_816,
	Skein1024_824,
	Skein1024_832,
	Skein1024_840,
	Skein1024_848,
	Skein1024_856,
	Skein1024_864,
	Skein1024_872,
	Skein1024_880,
	Skein1024_888,
	Skein1024_896,
	Skein1024_904,
	Skein1024_912,
	Skein1024_920,
	Skein1024_928,
	Skein1024_936,
	Skein1024_944,
	Skein1024_952,
	Skein1024_960,
	Skein1024_968,
	Skein1024_976,
	Skein1024_984,
	Skein1024_992,
	Skein1024_1000,
	Skein1024_1008,
	Skein1024_1016,
	Skein1024_1024,
	Xxh32,
	Xxh64,
	Xxh3_64,
	Xxh3_128,
	PoseidonBls12_381A2Fc1,
	PoseidonBls12_381A2Fc1Sc,
	Urdca2015Canon,
	Ssz,
	SszSha2_256Bmt,
	JsonJcs,
	Iscc,
	ZeroxcertImprint256,
	Varsig,
	Es256k,
	Bls12381G1Sig,
	Bls12381G2Sig,
	Eddsa,
	Eip191,
	Jwk_jcsPub,
	FilCommitmentUnsealed,
	FilCommitmentSealed,
	Plaintextv2,
	HolochainAdrV0,
	HolochainAdrV1,
	HolochainKeyV0,
	HolochainKeyV1,
	HolochainSigV0,
	HolochainSigV1,
	SkynetNs,
	ArweaveNs,
	SubspaceNs,
	KumandraNs,
	Es256,
	Es284,
	Es512,
	Rs256,
}

func (c Code) Tag() string {
	switch c {
	case Cidv1,
		Cidv2,
		Cidv3:
		return "cid"

	case AesGcm256:
		return "encryption"

	case FilCommitmentUnsealed,
		FilCommitmentSealed:
		return "filecoin"

	case Murmur3X64_64,
		Murmur3_32,
		Crc32,
		Crc64Ecma,
		Murmur3X64_128,
		Xxh32,
		Xxh64,
		Xxh3_64,
		Xxh3_128:
		return "hash"

	case HolochainAdrV0,
		HolochainAdrV1,
		HolochainKeyV0,
		HolochainKeyV1,
		HolochainSigV0,
		HolochainSigV1:
		return "holochain"

	case Cbor,
		Raw,
		DagPb,
		DagCbor,
		Libp2pKey,
		GitRaw,
		TorrentInfo,
		TorrentFile,
		LeofcoinBlock,
		LeofcoinTx,
		LeofcoinPr,
		DagJose,
		DagCose,
		EthBlock,
		EthBlockList,
		EthTxTrie,
		EthTx,
		EthTxReceiptTrie,
		EthTxReceipt,
		EthStateTrie,
		EthAccountSnapshot,
		EthStorageTrie,
		EthReceiptLogTrie,
		EthRecieptLog,
		BitcoinBlock,
		BitcoinTx,
		BitcoinWitnessCommitment,
		ZcashBlock,
		ZcashTx,
		StellarBlock,
		StellarTx,
		DecredBlock,
		DecredTx,
		DashBlock,
		DashTx,
		SwarmManifest,
		SwarmFeed,
		Beeson,
		DagJson,
		Swhid1Snp,
		Json,
		Urdca2015Canon,
		JsonJcs:
		return "ipld"

	case Aes128,
		Aes192,
		Aes256,
		Chacha128,
		Chacha256,
		Secp256k1Pub,
		Bls12_381G1Pub,
		Bls12_381G2Pub,
		X25519Pub,
		Ed25519Pub,
		Bls12_381G1g2Pub,
		Sr25519Pub,
		P256Pub,
		P384Pub,
		P521Pub,
		Ed448Pub,
		X448Pub,
		RsaPub,
		Sm2Pub,
		Ed25519Priv,
		Secp256k1Priv,
		X25519Priv,
		Sr25519Priv,
		RsaPriv,
		P256Priv,
		P384Priv,
		P521Priv,
		Jwk_jcsPub:
		return "key"

	case Libp2pPeerRecord,
		Libp2pRelayRsvp,
		Memorytransport:
		return "libp2p"

	case Ip4,
		Tcp,
		Dccp,
		Ip6,
		Ip6zone,
		Ipcidr,
		Dns,
		Dns4,
		Dns6,
		Dnsaddr,
		Sctp,
		Udp,
		P2pWebrtcStar,
		P2pWebrtcDirect,
		P2pStardust,
		WebrtcDirect,
		Webrtc,
		P2pCircuit,
		Udt,
		Utp,
		Unix,
		Thread,
		P2p,
		Https,
		Onion,
		Onion3,
		Garlic64,
		Garlic32,
		Tls,
		Sni,
		Noise,
		Quic,
		QuicV1,
		Webtransport,
		Certhash,
		Ws,
		Wss,
		P2pWebsocketStar,
		Http,
		Silverpine,
		Plaintextv2:
		return "multiaddr"

	case Multicodec,
		Multihash,
		Multiaddr,
		Multibase,
		Caip50,
		Multidid:
		return "multiformat"

	case Identity,
		Sha1,
		Sha2_256,
		Sha2_512,
		Sha3_512,
		Sha3_384,
		Sha3_256,
		Sha3_224,
		Shake128,
		Shake256,
		Keccak224,
		Keccak256,
		Keccak384,
		Keccak512,
		Blake3,
		Sha2_384,
		DblSha2_256,
		Md4,
		Md5,
		Sha2_256Trunc254Padded,
		Sha2_224,
		Sha2_512_224,
		Sha2_512_256,
		Ripemd128,
		Ripemd160,
		Ripemd256,
		Ripemd320,
		X11,
		Kangarootwelve,
		Sm3_256,
		Blake2b8,
		Blake2b16,
		Blake2b24,
		Blake2b32,
		Blake2b40,
		Blake2b48,
		Blake2b56,
		Blake2b64,
		Blake2b72,
		Blake2b80,
		Blake2b88,
		Blake2b96,
		Blake2b104,
		Blake2b112,
		Blake2b120,
		Blake2b128,
		Blake2b136,
		Blake2b144,
		Blake2b152,
		Blake2b160,
		Blake2b168,
		Blake2b176,
		Blake2b184,
		Blake2b192,
		Blake2b200,
		Blake2b208,
		Blake2b216,
		Blake2b224,
		Blake2b232,
		Blake2b240,
		Blake2b248,
		Blake2b256,
		Blake2b264,
		Blake2b272,
		Blake2b280,
		Blake2b288,
		Blake2b296,
		Blake2b304,
		Blake2b312,
		Blake2b320,
		Blake2b328,
		Blake2b336,
		Blake2b344,
		Blake2b352,
		Blake2b360,
		Blake2b368,
		Blake2b376,
		Blake2b384,
		Blake2b392,
		Blake2b400,
		Blake2b408,
		Blake2b416,
		Blake2b424,
		Blake2b432,
		Blake2b440,
		Blake2b448,
		Blake2b456,
		Blake2b464,
		Blake2b472,
		Blake2b480,
		Blake2b488,
		Blake2b496,
		Blake2b504,
		Blake2b512,
		Blake2s8,
		Blake2s16,
		Blake2s24,
		Blake2s32,
		Blake2s40,
		Blake2s48,
		Blake2s56,
		Blake2s64,
		Blake2s72,
		Blake2s80,
		Blake2s88,
		Blake2s96,
		Blake2s104,
		Blake2s112,
		Blake2s120,
		Blake2s128,
		Blake2s136,
		Blake2s144,
		Blake2s152,
		Blake2s160,
		Blake2s168,
		Blake2s176,
		Blake2s184,
		Blake2s192,
		Blake2s200,
		Blake2s208,
		Blake2s216,
		Blake2s224,
		Blake2s232,
		Blake2s240,
		Blake2s248,
		Blake2s256,
		Skein256_8,
		Skein256_16,
		Skein256_24,
		Skein256_32,
		Skein256_40,
		Skein256_48,
		Skein256_56,
		Skein256_64,
		Skein256_72,
		Skein256_80,
		Skein256_88,
		Skein256_96,
		Skein256_104,
		Skein256_112,
		Skein256_120,
		Skein256_128,
		Skein256_136,
		Skein256_144,
		Skein256_152,
		Skein256_160,
		Skein256_168,
		Skein256_176,
		Skein256_184,
		Skein256_192,
		Skein256_200,
		Skein256_208,
		Skein256_216,
		Skein256_224,
		Skein256_232,
		Skein256_240,
		Skein256_248,
		Skein256_256,
		Skein512_8,
		Skein512_16,
		Skein512_24,
		Skein512_32,
		Skein512_40,
		Skein512_48,
		Skein512_56,
		Skein512_64,
		Skein512_72,
		Skein512_80,
		Skein512_88,
		Skein512_96,
		Skein512_104,
		Skein512_112,
		Skein512_120,
		Skein512_128,
		Skein512_136,
		Skein512_144,
		Skein512_152,
		Skein512_160,
		Skein512_168,
		Skein512_176,
		Skein512_184,
		Skein512_192,
		Skein512_200,
		Skein512_208,
		Skein512_216,
		Skein512_224,
		Skein512_232,
		Skein512_240,
		Skein512_248,
		Skein512_256,
		Skein512_264,
		Skein512_272,
		Skein512_280,
		Skein512_288,
		Skein512_296,
		Skein512_304,
		Skein512_312,
		Skein512_320,
		Skein512_328,
		Skein512_336,
		Skein512_344,
		Skein512_352,
		Skein512_360,
		Skein512_368,
		Skein512_376,
		Skein512_384,
		Skein512_392,
		Skein512_400,
		Skein512_408,
		Skein512_416,
		Skein512_424,
		Skein512_432,
		Skein512_440,
		Skein512_448,
		Skein512_456,
		Skein512_464,
		Skein512_472,
		Skein512_480,
		Skein512_488,
		Skein512_496,
		Skein512_504,
		Skein512_512,
		Skein1024_8,
		Skein1024_16,
		Skein1024_24,
		Skein1024_32,
		Skein1024_40,
		Skein1024_48,
		Skein1024_56,
		Skein1024_64,
		Skein1024_72,
		Skein1024_80,
		Skein1024_88,
		Skein1024_96,
		Skein1024_104,
		Skein1024_112,
		Skein1024_120,
		Skein1024_128,
		Skein1024_136,
		Skein1024_144,
		Skein1024_152,
		Skein1024_160,
		Skein1024_168,
		Skein1024_176,
		Skein1024_184,
		Skein1024_192,
		Skein1024_200,
		Skein1024_208,
		Skein1024_216,
		Skein1024_224,
		Skein1024_232,
		Skein1024_240,
		Skein1024_248,
		Skein1024_256,
		Skein1024_264,
		Skein1024_272,
		Skein1024_280,
		Skein1024_288,
		Skein1024_296,
		Skein1024_304,
		Skein1024_312,
		Skein1024_320,
		Skein1024_328,
		Skein1024_336,
		Skein1024_344,
		Skein1024_352,
		Skein1024_360,
		Skein1024_368,
		Skein1024_376,
		Skein1024_384,
		Skein1024_392,
		Skein1024_400,
		Skein1024_408,
		Skein1024_416,
		Skein1024_424,
		Skein1024_432,
		Skein1024_440,
		Skein1024_448,
		Skein1024_456,
		Skein1024_464,
		Skein1024_472,
		Skein1024_480,
		Skein1024_488,
		Skein1024_496,
		Skein1024_504,
		Skein1024_512,
		Skein1024_520,
		Skein1024_528,
		Skein1024_536,
		Skein1024_544,
		Skein1024_552,
		Skein1024_560,
		Skein1024_568,
		Skein1024_576,
		Skein1024_584,
		Skein1024_592,
		Skein1024_600,
		Skein1024_608,
		Skein1024_616,
		Skein1024_624,
		Skein1024_632,
		Skein1024_640,
		Skein1024_648,
		Skein1024_656,
		Skein1024_664,
		Skein1024_672,
		Skein1024_680,
		Skein1024_688,
		Skein1024_696,
		Skein1024_704,
		Skein1024_712,
		Skein1024_720,
		Skein1024_728,
		Skein1024_736,
		Skein1024_744,
		Skein1024_752,
		Skein1024_760,
		Skein1024_768,
		Skein1024_776,
		Skein1024_784,
		Skein1024_792,
		Skein1024_800,
		Skein1024_808,
		Skein1024_816,
		Skein1024_824,
		Skein1024_832,
		Skein1024_840,
		Skein1024_848,
		Skein1024_856,
		Skein1024_864,
		Skein1024_872,
		Skein1024_880,
		Skein1024_888,
		Skein1024_896,
		Skein1024_904,
		Skein1024_912,
		Skein1024_920,
		Skein1024_928,
		Skein1024_936,
		Skein1024_944,
		Skein1024_952,
		Skein1024_960,
		Skein1024_968,
		Skein1024_976,
		Skein1024_984,
		Skein1024_992,
		Skein1024_1000,
		Skein1024_1008,
		Skein1024_1016,
		Skein1024_1024,
		PoseidonBls12_381A2Fc1,
		PoseidonBls12_381A2Fc1Sc,
		SszSha2_256Bmt:
		return "multihash"

	case Path,
		Streamid,
		Ipld,
		Ipfs,
		Swarm,
		Ipns,
		Zeronet,
		Dnslink,
		SkynetNs,
		ArweaveNs,
		SubspaceNs,
		KumandraNs:
		return "namespace"

	case Protobuf,
		Rlp,
		Bencode,
		Messagepack,
		Car,
		IpnsRecord,
		CarIndexSorted,
		CarMultihashIndexSorted,
		Ssz:
		return "serialization"

	case Iscc:
		return "softhash"

	case TransportBitswap,
		TransportGraphsyncFilecoinv1,
		TransportIpfsGatewayHttp:
		return "transport"

	case Varsig,
		Es256k,
		Bls12381G1Sig,
		Bls12381G2Sig,
		Eddsa,
		Eip191,
		Es256,
		Es284,
		Es512,
		Rs256:
		return "varsig"

	case ZeroxcertImprint256:
		return "zeroxcert"
	default:
		return "<unknown>"
	}
}
