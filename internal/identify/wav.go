package identify

import "encoding/binary"

func encodeWAV(samples []float32, rate int) []byte {
	if rate <= 0 {
		rate = 16000
	}
	n := len(samples)
	dataBytes := n * 2
	buf := make([]byte, 44+dataBytes)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataBytes))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], uint32(rate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataBytes))
	for i, v := range samples {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(int16(v*32767)))
	}
	return buf
}

// lastSeconds keeps the trailing window AudD/Shazam actually use (~12s).
func lastSeconds(samples []float32, rate int, sec float64) []float32 {
	if rate <= 0 {
		rate = 16000
	}
	n := int(float64(rate) * sec)
	if n <= 0 || len(samples) <= n {
		return samples
	}
	return samples[len(samples)-n:]
}
