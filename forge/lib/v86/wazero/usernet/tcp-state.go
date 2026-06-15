package usernet

const (
	tcpStateClosed      = "closed"
	tcpStateSynReceived = "syn-received"
	tcpStateEstablished = "established"
	tcpStateFinWait1    = "fin-wait-1"
	tcpStateCloseWait   = "close-wait"
	tcpStateFinWait2    = "fin-wait-2"
	tcpStateLastAck     = "last-ack"
	tcpStateClosing     = "closing"
)

const (
	tcpFlagFIN = 0x01
	tcpFlagSYN = 0x02
	tcpFlagRST = 0x04
	tcpFlagPSH = 0x08
	tcpFlagACK = 0x10
)
