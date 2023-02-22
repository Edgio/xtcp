//
// This package contains the golang versions of the kernel structs
// These structs are used while doing the binary.Read calls
// on the netlink messages coming back from the kernel

package inetdiag

//import "github.com/Edgio/xtcp/inetdiag" // kernel structs

//	struct nlmsghdr {
//		__u32		nlmsg_len;	/* Length of message including header */
//		__u16		nlmsg_type;	/* Message content */
//		__u16		nlmsg_flags;	/* Additional flags */
//		__u32		nlmsg_seq;	/* Sequence number */
//		__u32		nlmsg_pid;	/* Sending process port ID */
//	};
type NlMsgHdr struct {
	Length   uint32
	Type     uint16
	Flags    uint16
	Sequence uint32
	Pid      uint32
}

// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L115

// // Base info structure. It contains socket identity (addrs/ports/cookie)
// // and, alas, the information shown by netstat.
// struct inet_diag_msg {
// 	__u8	idiag_family;
// 	__u8	idiag_state;
// 	__u8	idiag_timer;
// 	__u8	idiag_retrans;

// 	struct inet_diag_sockid id;

// 	__u32	idiag_expires;
// 	__u32	idiag_rqueue;
// 	__u32	idiag_wqueue;
// 	__u32	idiag_uid;
// 	__u32	idiag_inode;
// };

// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L13
// struct inet_diag_sockid {
// 	__be16	idiag_sport;
// 	__be16	idiag_dport;
// 	__be32	idiag_src[4];
// 	__be32	idiag_dst[4];
// 	__u32	idiag_if;
// 	__u32	idiag_cookie[2];
// #define INET_DIAG_NOCOOKIE (~0U)
// };

type InetDiagMsg struct {
	Family   uint8
	State    uint8
	Timer    uint8
	Retrans  uint8
	SocketID SocketID
	Expires  uint32
	Rqueue   uint32
	Wqueue   uint32
	UID      uint32 // golang seems to want all caps UID.  ??!
	Inode    uint32
}

// SocketID identifies a single socket
type SocketID struct {
	SourcePort      uint16 //warning the Ports here are BigEndian for some reason.  See dodgy swap order hack later.
	DestinationPort uint16
	Source          [16]byte
	Destination     [16]byte
	Interface       uint32
	Cookie          uint64
}

//Cookie          [2]uint32

// https://github.com/torvalds/linux/blob/bd2463ac7d7ec51d432f23bf0e893fb371a908cd/include/uapi/linux/rtnetlink.h#L195
//
//	struct rtattr {
//		unsigned short	rta_len;
//		unsigned short	rta_type;
//	};
type Nlattr struct {
	NlaLen  uint16
	NlaType uint16
}

// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L174
// /* INET_DIAG_MEM */
//
//	struct inet_diag_meminfo {
//		__u32	idiag_rmem;
//		__u32	idiag_wmem;
//		__u32	idiag_fmem;
//		__u32	idiag_tmem;
//	};
type MemInfo struct {
	Rmem uint32
	Wmem uint32
	Fmem uint32
	Tmem uint32
}

// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/tcp.h#L214
// struct tcp_info {
// 	__u8	_state;
// 	__u8	_ca_state;
// 	__u8	_retransmits;
// 	__u8	_probes;
// 	__u8	_backoff;
// 	__u8	_options;
// 	__u8	_snd_wscale : 4, _rcv_wscale : 4;
// 	__u8	_delivery_rate_app_limited:1, _fastopen_client_fail:2;

// 	__u32	_rto;
// 	__u32	_ato;
// 	__u32	_snd_mss;
// 	__u32	_rcv_mss;

// 	__u32	_unacked;
// 	__u32	_sacked;
// 	__u32	_lost;
// 	__u32	_retrans;
// 	__u32	_fackets;

// 	/* Times. */
// 	__u32	_last_data_sent;
// 	__u32	_last_ack_sent;     /* Not remembered, sorry. */
// 	__u32	_last_data_recv;
// 	__u32	_last_ack_recv;

// 	/* Metrics. */
// 	__u32	_pmtu;
// 	__u32	_rcv_ssthresh;
// 	__u32	_rtt;
// 	__u32	_rttvar;
// 	__u32	_snd_ssthresh;
// 	__u32	_snd_cwnd;
// 	__u32	_advmss;
// 	__u32	_reordering;

// 	__u32	_rcv_rtt;
// 	__u32	_rcv_space;

// 	__u32	_total_retrans;

// 	__u64	_pacing_rate;
// 	__u64	_max_pacing_rate;
// 	__u64	_bytes_acked;    /* RFC4898 tcpEStatsAppHCThruOctetsAcked */
// 	__u64	_bytes_received; /* RFC4898 tcpEStatsAppHCThruOctetsReceived */
// 	__u32	_segs_out;	     /* RFC4898 tcpEStatsPerfSegsOut */
// 	__u32	_segs_in;	     /* RFC4898 tcpEStatsPerfSegsIn */

// 	__u32	_notsent_bytes;
// 	__u32	_min_rtt;
// 	__u32	_data_segs_in;	/* RFC4898 tcpEStatsDataSegsIn */
// 	__u32	_data_segs_out;	/* RFC4898 tcpEStatsDataSegsOut */

// 	__u64   _delivery_rate;

// 	__u64	_busy_time;      /* Time (usec) busy sending data */
// 	__u64	_rwnd_limited;   /* Time (usec) limited by receive window */
// 	__u64	_sndbuf_limited; /* Time (usec) limited by send buffer */

// 	__u32	_delivered;
// 	__u32	_delivered_ce;

// 	__u64	_bytes_sent;     /* RFC4898 tcpEStatsPerfHCDataOctetsOut */
// 	__u64	_bytes_retrans;  /* RFC4898 tcpEStatsPerfOctetsRetrans */
// 	__u32	_dsack_dups;     /* RFC4898 tcpEStatsStackDSACKDups */
// 	__u32	_reord_seen;     /* reordering events seen */

// 	__u32	_rcv_ooopack;    /* Out-of-order packets received */

// 	__u32	_snd_wnd;	     /* peer's advertised receive window after
// 				      * scaling (bytes)
// 				      */
// };

// tcp_info_for kernel 5.4+
type TCPInfo54 struct {
	State       uint8
	CaState     uint8
	Retransmits uint8
	Probes      uint8
	Backoff     uint8
	Options     uint8
	ScaleTemp   uint8 // _snd_wscale : 4, _rcv_wscale : 4; fix me
	FlagsTemp   uint8 // _delivery_rate_app_limited:1, _fastopen_client_fail:2; TODO fix me!

	Rto    uint32
	Ato    uint32
	SndMss uint32
	RcvMss uint32

	Unacked uint32
	Sacked  uint32
	Lost    uint32
	Retrans uint32
	Fackets uint32

	// 	Times
	LastDataSent uint32
	LastAckSent  uint32
	LastDataRecv uint32
	LastAckRecv  uint32

	// 	Metrics
	Pmtu        uint32
	RcvSsthresh uint32
	Rtt         uint32
	Rttvar      uint32
	SndSsthresh uint32
	SndCwnd     uint32
	AdvMss      uint32
	Reordering  uint32

	RcvRtt   uint32
	RcvSpace uint32

	TotalRetrans uint32

	PacingRate    uint64
	MaxPacingRate uint64
	BytesAcked    uint64 // RFC4898 tcpEStatsAppHCThruOctetsAcked
	BytesReceived uint64 // RFC4898 tcpEStatsAppHCThruOctetsReceived
	SegsOut       uint32 // RFC4898 tcpEStatsPerfSegsOut
	SegsIn        uint32 // RFC4898 tcpEStatsPerfSegsIn

	NotSentBytes uint32
	MinRtt       uint32
	DataSegsIn   uint32 // RFC4898 tcpEStatsDataSegsIn
	DataSegsOut  uint32 // RFC4898 tcpEStatsDataSegsOut

	DeliveryRate uint64

	BusyTime      uint64 // Time (usec) busy sending data
	RwndLimited   uint64 // Time (usec) limited by receive window
	SndbufLimited uint64 // Time (usec) limited by send buffer

	//4.15 kernel tcp_info ends here, 5+ below

	Delivered   uint32
	DeliveredCe uint32

	BytesSent    uint64 // RFC4898 tcpEStatsPerfHCDataOctetsOut
	BytesRetrans uint64 // RFC4898 tcpEStatsPerfOctetsRetrans
	DsackDups    uint32 // RFC4898 tcpEStatsStackDSACKDups
	ReordSeen    uint32 // reordering events seen

	RcvOoopack uint32 // Out-of-order packets received

	SndWnd uint32 // peer's advertised receive window after scaling (bytes)
}

// https://git.launchpad.net/~ubuntu-kernel/ubuntu/+source/linux/+git/xenial/tree/include/uapi/linux/tcp.h?h=Ubuntu-hwe-4.15.0-107.108_16.04.1#n168
type TCPInfo415 struct {
	State       uint8
	CaState     uint8
	Retransmits uint8
	Probes      uint8
	Backoff     uint8
	Options     uint8
	ScaleTemp   uint8 //_snd_wscale : 4, _rcv_wscale : 4; fix me
	FlagsTemp   uint8 // _delivery_rate_app_limited:1, _fastopen_client_fail:2; TODO fix me!

	Rto    uint32
	Ato    uint32
	SndMss uint32
	RcvMss uint32

	Unacked uint32
	Sacked  uint32
	Lost    uint32
	Retrans uint32
	Fackets uint32

	// 	Times
	LastDataSent uint32
	LastAckSent  uint32
	LastDataRecv uint32
	LastAckRecv  uint32

	// 	Metrics
	Pmtu        uint32
	RcvSsthresh uint32
	Rtt         uint32
	Rttvar      uint32
	SndSsthresh uint32
	SndCwnd     uint32
	AdvMss      uint32
	Reordering  uint32

	RcvRtt   uint32
	RcvSpace uint32

	TotalRetrans uint32

	PacingRate    uint64
	MaxPacingRate uint64
	BytesAcked    uint64 // RFC4898 tcpEStatsAppHCThruOctetsAcked
	BytesReceived uint64 // RFC4898 tcpEStatsAppHCThruOctetsReceived
	SegsOut       uint32 // RFC4898 tcpEStatsPerfSegsOut
	SegsIn        uint32 // RFC4898 tcpEStatsPerfSegsIn

	NotSentBytes uint32
	MinRtt       uint32
	DataSegsIn   uint32 // RFC4898 tcpEStatsDataSegsIn
	DataSegsOut  uint32 // RFC4898 tcpEStatsDataSegsOut

	DeliveryRate uint64

	BusyTime      uint64 // Time (usec) busy sending data
	RwndLimited   uint64 // Time (usec) limited by receive window
	SndbufLimited uint64 // Time (usec) limited by send buffer

	//4.15 kernel tcp_info ends here, 5+ below}
}

// Described here:
// http://man7.org/linux/man-pages/man7/sock_diag.7.html
// https://github.com/torvalds/linux/blob/a811c1fa0a02c062555b54651065899437bacdbe/net/core/sock.c#L3226
//
//	struct sk_meminfo {
//	    __u32   rmem_alloc; //The amount of data in receive queue
//	    __u32   rcv_buf;    //The receive socket buffer as set by SO_RCVBUF.
//	    __u32   wmem_alloc; //The amount of data in send queue.
//	    __u32   snd_buf;    //The send socket buffer as set by SO_SNDBUF.
//	    __u32   fwd_alloc;  //The amount of memory scheduled for future use (TCP only).
//	    __u32   wmem_queued;//The amount of data queued by TCP, but not yet sent.
//	    __u32   optmem;     //The amount of memory allocated for the sockets service needs
//	    __u32   backlog;    //The amount of packets in the backlog (not yet processed).
//	    __u32   drops; 
//	};
type SkMemInfo struct {
	RmemAlloc  uint32
	RcvBuf     uint32
	WmemAlloc  uint32
	SndBuf     uint32
	FwdAlloc   uint32
	WmemQueued uint32
	Optmem     uint32
	Backlog    uint32
	Drops      uint32
}

// https://github.com/torvalds/linux/blob/29d9f30d4ce6c7a38745a54a8cddface10013490/include/uapi/linux/inet_diag.h#L204
//
//	struct tcp_bbr_info {
//		/* u64 bw: max-filtered BW (app throughput) estimate in Byte per sec: */
//		__u32	bbr_bw_lo;		/* lower 32 bits of bw */
//		__u32	bbr_bw_hi;		/* upper 32 bits of bw */
//		__u32	bbr_min_rtt;		/* min-filtered RTT in uSec */
//		__u32	bbr_pacing_gain;	/* pacing gain shifted left 8 bits */
//		__u32	bbr_cwnd_gain;		/* cwnd gain shifted left 8 bits */
//	};
type BBRInfo struct {
	BwLo       uint32
	BwHi       uint32
	MinRtt     uint32
	PacingGain uint32
	CwndGain   uint32
}
