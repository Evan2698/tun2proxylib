package gvisorcore

type TransportHandler interface {
	HandleTCP(TCPConn)
	HandleUDP(UDPConn)
}
