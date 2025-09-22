package udppackage

import (
	"bytes"
	"errors"
	"net"
)

// define udp package here

//  1 byte | target address  | port (2 bytes) | 1 byte | source address | port (2 bytes) | length of payload 4 bytes | data
// +--------+-----------------+----------------+--------+----------------+----------------+--------------------------+--------+
// | 0x01/0x03/0x04 | target IP | target port | 0x01/0x03/0x04 | source IP | source port | 0x00000000000000000000000 | payload |
// +--------+-----------------+----------------+--------+----------------+----------------+--------------------------+---------+

func PackUDPData(target, src *net.UDPAddr, payload []byte) (full []byte, err error) {

	//check parameters
	if target == nil || src == nil || payload == nil {
		return nil, errors.New("invalid UDP address")
	}

	var buffer bytes.Buffer
	ipvLen := len(target.IP)
	buffer.WriteByte(byte(ipvLen))
	buffer.Write(target.IP)
	buffer.WriteByte(byte(target.Port >> 8))
	buffer.WriteByte(byte(target.Port & 0xff))

	ipvLen = len(src.IP)
	buffer.WriteByte(byte(ipvLen))
	buffer.Write(src.IP)
	buffer.WriteByte(byte(src.Port >> 8))
	buffer.WriteByte(byte(src.Port & 0xff))

	payloadLen := uint32(len(payload))
	buffer.WriteByte(byte((payloadLen >> 24) & 0xff))
	buffer.WriteByte(byte((payloadLen >> 16) & 0xff))
	buffer.WriteByte(byte((payloadLen >> 8) & 0xff))
	buffer.WriteByte(byte(payloadLen & 0xff))

	buffer.Write(payload)

	return buffer.Bytes(), nil
}

func UnpackUDPData(data []byte) (target, src *net.UDPAddr, payload []byte, err error) {
	if len(data) < 4 {
		return nil, nil, nil, errors.New("data too short")
	}

	var idx = 0
	ipLen := int(data[idx])
	idx++

	var targetIP, srcIP net.IP
	targetIP = net.IP(data[idx : idx+int(ipLen)])
	idx += int(ipLen)
	targetPort := int(data[idx])<<8 | int(data[idx+1])
	idx += 2

	ipLen = int(data[idx])
	idx++
	srcIP = net.IP(data[idx : idx+int(ipLen)])
	idx += int(ipLen)
	srcPort := int(data[idx])<<8 | int(data[idx+1])
	idx += 2

	payloadLen := uint32(data[idx])<<24 | uint32(data[idx+1])<<16 | uint32(data[idx+2])<<8 | uint32(data[idx+3])
	idx += 4

	payload = data[idx : idx+int(payloadLen)]

	target = &net.UDPAddr{
		IP:   targetIP,
		Port: targetPort,
	}
	src = &net.UDPAddr{
		IP:   srcIP,
		Port: srcPort,
	}

	return target, src, payload, nil
}
