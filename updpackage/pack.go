package updpackage

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
	if len(target.IP) == net.IPv4len {
		buffer.WriteByte(0x01)
	} else if len(target.IP) == net.IPv6len {
		buffer.WriteByte(0x04)
	} else {
		return nil, errors.New("invalid target IP address")
	}
	buffer.Write(target.IP)
	buffer.WriteByte(byte(target.Port >> 8))
	buffer.WriteByte(byte(target.Port & 0xff))

	if len(src.IP) == net.IPv4len {
		buffer.WriteByte(0x01)
	} else if len(src.IP) == net.IPv6len {
		buffer.WriteByte(0x04)
	} else {
		return nil, errors.New("invalid source IP address")
	}
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
	if len(data) < 1+4+2+1+4+2+4 {
		return nil, nil, nil, errors.New("data too short")
	}

	var idx = 0
	var atyp = data[idx]
	idx++
	var targetIP net.IP
	if atyp == 0x01 {
		if len(data) < idx+4+2+1+4+2+4 {
			return nil, nil, nil, errors.New("data too short for IPv4")
		}
		targetIP = net.IPv4(data[idx], data[idx+1], data[idx+2], data[idx+3])
		idx += 4
	}

	if atyp == 0x04 {
		if len(data) < idx+16+2+1+4+2+4 {
			return nil, nil, nil, errors.New("data too short for IPv6")
		}
		targetIP = net.IP(data[idx : idx+16])
		idx += 16
	}

	if targetIP == nil {
		return nil, nil, nil, errors.New("invalid target IP address")
	}

	if len(data) < idx+2+1+4+2+4 {
		return nil, nil, nil, errors.New("data too short for target port")
	}
	targetPort := int(data[idx])<<8 | int(data[idx+1])
	idx += 2

	atyp = data[idx]
	idx++
	var srcIP net.IP
	if atyp == 0x01 {
		if len(data) < idx+4+2+4 {
			return nil, nil, nil, errors.New("data too short for IPv4")
		}
		srcIP = net.IPv4(data[idx], data[idx+1], data[idx+2], data[idx+3])
		idx += 4
	}

	if atyp == 0x04 {
		if len(data) < idx+16+2+4 {
			return nil, nil, nil, errors.New("data too short for IPv6")
		}
		srcIP = net.IP(data[idx : idx+16])
		idx += 16
	}

	if srcIP == nil {
		return nil, nil, nil, errors.New("invalid source IP address")
	}

	if len(data) < idx+2+4 {
		return nil, nil, nil, errors.New("data too short for source port")
	}
	srcPort := int(data[idx])<<8 | int(data[idx+1])
	idx += 2

	if len(data) < idx+4 {
		return nil, nil, nil, errors.New("data too short for payload length")
	}
	payloadLen := uint32(data[idx])<<24 | uint32(data[idx+1])<<16 | uint32(data[idx+2])<<8 | uint32(data[idx+3])
	idx += 4

	if len(data) < idx+int(payloadLen) {
		return nil, nil, nil, errors.New("data too short for payload")
	}
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
