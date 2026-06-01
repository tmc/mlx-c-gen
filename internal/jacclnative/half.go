package jacclnative

import (
	"encoding/binary"
	"math"
)

const (
	float16QuietNaN  = 0x7d00
	bfloat16QuietNaN = 0x7fc0
)

func reduceFloat16Bytes(dst, src []byte, dtype DType, op reduceOp) {
	for off := 0; off < len(src); off += 2 {
		a := float16ToFloat32(binary.LittleEndian.Uint16(dst[off:]), dtype)
		b := float16ToFloat32(binary.LittleEndian.Uint16(src[off:]), dtype)
		v := reduceFloat32(a, b, op)
		binary.LittleEndian.PutUint16(dst[off:], float32ToFloat16(v, dtype))
	}
}

func reduceFloat32(a, b float32, op reduceOp) float32 {
	switch op {
	case reduceSum:
		return a + b
	case reduceMax:
		if a > b {
			return a
		}
		return b
	case reduceMin:
		if a < b {
			return a
		}
		return b
	default:
		panic("unreachable")
	}
}

func float16ToFloat32(bits uint16, dtype DType) float32 {
	if dtype == DTypeBFloat16 {
		return math.Float32frombits(uint32(bits) << 16)
	}
	sign := uint32(bits&0x8000) << 16
	exp := int((bits >> 10) & 0x1f)
	frac := uint32(bits & 0x03ff)
	switch exp {
	case 0:
		if frac == 0 {
			return math.Float32frombits(sign)
		}
		for frac&0x0400 == 0 {
			frac <<= 1
			exp--
		}
		exp++
		frac &^= 0x0400
	case 0x1f:
		return math.Float32frombits(sign | 0x7f800000 | frac<<13)
	}
	exp32 := uint32(exp + 127 - 15)
	return math.Float32frombits(sign | exp32<<23 | frac<<13)
}

func float32ToFloat16(f float32, dtype DType) uint16 {
	bits := math.Float32bits(f)
	if dtype == DTypeBFloat16 {
		if math.IsNaN(float64(f)) {
			return bfloat16QuietNaN
		}
		bits += (bits >> 16 & 1) + 0x7fff
		return uint16(bits >> 16)
	}
	sign := uint16((bits >> 16) & 0x8000)
	exp := int((bits >> 23) & 0xff)
	frac := bits & 0x7fffff
	if exp == 0xff {
		if frac != 0 {
			return sign | float16QuietNaN
		}
		return sign | 0x7c00
	}
	exp16 := exp - 127 + 15
	if exp16 >= 0x1f {
		return sign | 0x7c00
	}
	if exp16 <= 0 {
		if exp16 < -10 {
			return sign
		}
		frac |= 0x800000
		shift := uint(14 - exp16)
		half := uint16(frac >> shift)
		if frac>>(shift-1)&1 != 0 {
			half++
		}
		return sign | half
	}
	half := sign | uint16(exp16<<10) | uint16(frac>>13)
	if frac&0x00001000 != 0 {
		half++
	}
	return half
}
