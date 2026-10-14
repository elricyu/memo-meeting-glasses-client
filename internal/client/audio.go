package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

const DefaultMirror = "/tmp/ai-core_audio_stream"

type Packet struct {
	Rate      uint32
	Sequence  uint64
	CaptureUS uint64
	PCM       []int16
}

func DecodeALaw(value byte) int16 {
	value ^= 0x55
	sample := int(value&15) << 4
	exponent := int((value & 0x70) >> 4)
	if exponent == 0 {
		sample += 8
	} else {
		sample += 0x108
		sample <<= exponent - 1
	}
	if value&0x80 == 0 {
		sample = -sample
	}
	return int16(sample)
}

func ParsePacket(data []byte) (Packet, error) {
	if len(data) < 44 {
		return Packet{}, errors.New("audio packet header truncated")
	}
	u16 := func(offset int) uint16 { return binary.LittleEndian.Uint16(data[offset:]) }
	u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(data[offset:]) }
	u64 := func(offset int) uint64 { return binary.LittleEndian.Uint64(data[offset:]) }
	header, size, rate := int(u16(6)), int(u32(24)), u32(12)
	if u32(0) != 0x41415346 || u16(4) != 1 || header < 44 || header > len(data) || size < 1 || size > 4096 || header+size != len(data) {
		return Packet{}, errors.New("invalid AASF packet")
	}
	if u32(8) != 1 || u16(16) != 1 || u16(18) != 8 || u32(20) != uint32(size) {
		return Packet{}, errors.New("only mono G711A mirror audio is supported")
	}
	if rate != 8000 && rate != 16000 && rate != 32000 && rate != 48000 {
		return Packet{}, fmt.Errorf("unsupported sample rate: %d", rate)
	}
	samples := make([]int16, size)
	for i, v := range data[header:] {
		samples[i] = DecodeALaw(v)
	}
	return Packet{Rate: rate, Sequence: u64(36), CaptureUS: u64(28), PCM: samples}, nil
}

// A streaming weighted resampler with fractional state preserved across packets.
type Resampler struct {
	rate               uint32
	ratio, weight, sum float64
}

func (r *Resampler) Convert(p Packet) ([]byte, error) {
	if r.rate == 0 {
		r.rate = p.Rate
		r.ratio = float64(p.Rate) / 16000
	}
	if r.rate != p.Rate {
		return nil, errors.New("mirror sample rate changed during recording")
	}
	output := make([]byte, 0, len(p.PCM)*2)
	for _, sample := range p.PCM {
		remaining := 1.0
		for remaining > 1e-8 {
			part := min(remaining, r.ratio-r.weight)
			r.sum += float64(sample) * part
			r.weight += part
			remaining -= part
			if r.weight >= r.ratio-1e-8 {
				v := int16(r.sum / r.ratio)
				output = append(output, byte(v), byte(uint16(v)>>8))
				r.weight = 0
				r.sum = 0
			}
		}
	}
	return output, nil
}
func Subscribe(path string) (*net.UnixConn, error) {
	return net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: path, Net: "unixpacket"})
}
func ReadPacket(conn *net.UnixConn) (Packet, error) {
	return ReadPacketWithTimeout(conn, 5*time.Second)
}
func ReadPacketWithTimeout(conn *net.UnixConn, timeout time.Duration) (Packet, error) {
	buffer := make([]byte, 44+4096)
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return Packet{}, err
	}
	n, _, flags, _, err := conn.ReadMsgUnix(buffer, nil)
	if err != nil {
		return Packet{}, fmt.Errorf("mirror read failed or no audio for %s: %w", timeout, err)
	}
	if flags&0x20 != 0 {
		return Packet{}, errors.New("mirror packet truncated")
	}
	return ParsePacket(buffer[:n])
}
