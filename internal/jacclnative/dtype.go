package jacclnative

import "fmt"

// DType is the JACCL element type used by reductions.
type DType int32

const (
	DTypeBool DType = iota
	DTypeInt8
	DTypeInt16
	DTypeInt32
	DTypeInt64
	DTypeUint8
	DTypeUint16
	DTypeUint32
	DTypeUint64
	DTypeFloat16
	DTypeBFloat16
	DTypeFloat32
	DTypeFloat64
	DTypeComplex64
)

// Size reports the element size in bytes.
func (d DType) Size() (int, error) {
	switch d {
	case DTypeBool, DTypeInt8, DTypeUint8:
		return 1, nil
	case DTypeInt16, DTypeUint16, DTypeFloat16, DTypeBFloat16:
		return 2, nil
	case DTypeInt32, DTypeUint32, DTypeFloat32:
		return 4, nil
	case DTypeInt64, DTypeUint64, DTypeFloat64, DTypeComplex64:
		return 8, nil
	default:
		return 0, fmt.Errorf("unsupported dtype %d", int32(d))
	}
}

// Element is a Go element type supported by typed collectives.
type Element interface {
	bool | int8 | int16 | int32 | int64 |
		uint8 | uint16 | uint32 | uint64 |
		float32 | float64 | complex64
}

func dtypeFor[T Element]() DType {
	var zero T
	switch any(zero).(type) {
	case bool:
		return DTypeBool
	case int8:
		return DTypeInt8
	case int16:
		return DTypeInt16
	case int32:
		return DTypeInt32
	case int64:
		return DTypeInt64
	case uint8:
		return DTypeUint8
	case uint16:
		return DTypeUint16
	case uint32:
		return DTypeUint32
	case uint64:
		return DTypeUint64
	case float32:
		return DTypeFloat32
	case float64:
		return DTypeFloat64
	case complex64:
		return DTypeComplex64
	default:
		panic("unreachable")
	}
}
