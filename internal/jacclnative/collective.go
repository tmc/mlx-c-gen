package jacclnative

import (
	"context"
	"fmt"
	"unsafe"
)

// AllSum computes the element-wise sum across all ranks.
func AllSum[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return allReduce(ctx, g, "all sum", dst, src, sum[T])
}

// AllSumBytes sum-reduces src into dst using dtype.
func AllSumBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return allReduceBytes(ctx, g, "all sum", dst, src, dtype, reduceSum)
}

// AllMax computes the element-wise maximum across all ranks.
func AllMax[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return allReduce(ctx, g, "all max", dst, src, max[T])
}

// AllMaxBytes max-reduces src into dst using dtype.
func AllMaxBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return allReduceBytes(ctx, g, "all max", dst, src, dtype, reduceMax)
}

// AllMin computes the element-wise minimum across all ranks.
func AllMin[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return allReduce(ctx, g, "all min", dst, src, min[T])
}

// AllMinBytes min-reduces src into dst using dtype.
func AllMinBytes(ctx context.Context, g *Group, dst, src []byte, dtype DType) error {
	return allReduceBytes(ctx, g, "all min", dst, src, dtype, reduceMin)
}

// AllGather gathers each rank's src into dst in rank order.
func AllGather[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return AllGatherBytes(ctx, g, bytesOf(dst), bytesOf(src))
}

// AllGatherBytes gathers each rank's src into dst in rank order.
func AllGatherBytes(ctx context.Context, g *Group, dst, src []byte) error {
	if err := g.check(ctx, "all gather"); err != nil {
		return err
	}
	want, err := allGatherBytesLen(g.size, len(src))
	if err != nil {
		return err
	}
	if len(dst) != want {
		return fmt.Errorf("all gather: destination length %d, want %d", len(dst), want)
	}
	if g.backend == nil {
		copy(dst, src)
		return nil
	}
	if g.size == 2 {
		copy(dst[g.rank*len(src):(g.rank+1)*len(src)], src)
		peer := 1 - g.rank
		return g.backend.exchangeOnePeerInto(ctx, dst[peer*len(src):(peer+1)*len(src)], src)
	}
	recvs, err := g.backend.gather(ctx, src)
	if err != nil {
		return err
	}
	copy(dst[g.rank*len(src):(g.rank+1)*len(src)], src)
	for peer := 0; peer < g.size; peer++ {
		if peer == g.rank {
			continue
		}
		recv, err := gatheredBytes("all gather", peer, recvs[peer], len(src))
		if err != nil {
			return err
		}
		copy(dst[peer*len(src):(peer+1)*len(src)], recv)
	}
	return nil
}

func allReduce[T Element](ctx context.Context, g *Group, name string, dst, src []T, op func([]T, []T)) error {
	return allReduceTyped(ctx, g, name, dst, src, op)
}

func allReduceTyped[T Element](ctx context.Context, g *Group, name string, dst, src []T, op func([]T, []T)) error {
	if err := g.check(ctx, name); err != nil {
		return err
	}
	if len(dst) != len(src) {
		return fmt.Errorf("%s: destination length %d, want %d", name, len(dst), len(src))
	}
	if g.backend == nil {
		copy(dst, src)
		return nil
	}
	dstBytes := bytesOf(dst)
	srcBytes := bytesOf(src)
	if g.size == 2 && !bytesOverlap(dstBytes, srcBytes) {
		if err := g.backend.exchangeOnePeerInto(ctx, dstBytes, srcBytes); err != nil {
			return err
		}
		op(dst, src)
		return nil
	}
	if bytesOverlap(dstBytes, srcBytes) {
		src = append([]T(nil), src...)
		srcBytes = bytesOf(src)
	}
	recvs, err := g.backend.gather(ctx, srcBytes)
	if err != nil {
		return err
	}
	copy(dst, src)
	if g.rank != 0 {
		recv, err := gatheredBytes(name, 0, recvs[0], len(srcBytes))
		if err != nil {
			return err
		}
		copy(dst, typedFromBytes[T](recv, len(src)))
	}
	for rank := 1; rank < g.size; rank++ {
		if rank == g.rank {
			op(dst, src)
			continue
		}
		recv, err := gatheredBytes(name, rank, recvs[rank], len(bytesOf(src)))
		if err != nil {
			return err
		}
		op(dst, typedFromBytes[T](recv, len(src)))
	}
	return nil
}

type reduceOp int

const (
	reduceSum reduceOp = iota
	reduceMax
	reduceMin
)

func allReduceBytes(ctx context.Context, g *Group, name string, dst, src []byte, dtype DType, op reduceOp) error {
	if err := g.check(ctx, name); err != nil {
		return err
	}
	if err := validateDTypeBytes(name, src, dtype); err != nil {
		return err
	}
	if err := validateDTypeBytes(name, dst, dtype); err != nil {
		return err
	}
	if len(dst) != len(src) {
		return fmt.Errorf("%s: destination length %d, want %d", name, len(dst), len(src))
	}
	if g.backend == nil {
		copy(dst, src)
		return nil
	}
	switch dtype {
	case DTypeBool:
		return allReduceTyped(ctx, g, name, boolsOf(dst), boolsOf(src), reduceFunc[bool](op))
	case DTypeInt8:
		return allReduceTyped(ctx, g, name, typedFromBytes[int8](dst, len(dst)), typedFromBytes[int8](src, len(src)), reduceFunc[int8](op))
	case DTypeInt16:
		return allReduceTyped(ctx, g, name, typedFromBytes[int16](dst, len(dst)/2), typedFromBytes[int16](src, len(src)/2), reduceFunc[int16](op))
	case DTypeInt32:
		return allReduceTyped(ctx, g, name, typedFromBytes[int32](dst, len(dst)/4), typedFromBytes[int32](src, len(src)/4), reduceFunc[int32](op))
	case DTypeInt64:
		return allReduceTyped(ctx, g, name, typedFromBytes[int64](dst, len(dst)/8), typedFromBytes[int64](src, len(src)/8), reduceFunc[int64](op))
	case DTypeUint8:
		return allReduceTyped(ctx, g, name, typedFromBytes[uint8](dst, len(dst)), typedFromBytes[uint8](src, len(src)), reduceFunc[uint8](op))
	case DTypeUint16:
		return allReduceTyped(ctx, g, name, typedFromBytes[uint16](dst, len(dst)/2), typedFromBytes[uint16](src, len(src)/2), reduceFunc[uint16](op))
	case DTypeUint32:
		return allReduceTyped(ctx, g, name, typedFromBytes[uint32](dst, len(dst)/4), typedFromBytes[uint32](src, len(src)/4), reduceFunc[uint32](op))
	case DTypeUint64:
		return allReduceTyped(ctx, g, name, typedFromBytes[uint64](dst, len(dst)/8), typedFromBytes[uint64](src, len(src)/8), reduceFunc[uint64](op))
	case DTypeFloat16, DTypeBFloat16:
		return allReduceFloat16Bytes(ctx, g, name, dst, src, dtype, op)
	case DTypeFloat32:
		return allReduceTyped(ctx, g, name, typedFromBytes[float32](dst, len(dst)/4), typedFromBytes[float32](src, len(src)/4), reduceFunc[float32](op))
	case DTypeFloat64:
		return allReduceTyped(ctx, g, name, typedFromBytes[float64](dst, len(dst)/8), typedFromBytes[float64](src, len(src)/8), reduceFunc[float64](op))
	case DTypeComplex64:
		return allReduceTyped(ctx, g, name, typedFromBytes[complex64](dst, len(dst)/8), typedFromBytes[complex64](src, len(src)/8), reduceFunc[complex64](op))
	default:
		return fmt.Errorf("%s: unsupported dtype %d", name, int32(dtype))
	}
}

func allReduceFloat16Bytes(ctx context.Context, g *Group, name string, dst, src []byte, dtype DType, op reduceOp) error {
	if err := g.check(ctx, name); err != nil {
		return err
	}
	if len(dst) != len(src) {
		return fmt.Errorf("%s: destination length %d, want %d", name, len(dst), len(src))
	}
	if g.backend == nil {
		copy(dst, src)
		return nil
	}
	if g.size == 2 && !bytesOverlap(dst, src) {
		if err := g.backend.exchangeOnePeerInto(ctx, dst, src); err != nil {
			return err
		}
		reduceFloat16Bytes(dst, src, dtype, op)
		return nil
	}
	if bytesOverlap(dst, src) {
		src = append([]byte(nil), src...)
	}
	recvs, err := g.backend.gather(ctx, src)
	if err != nil {
		return err
	}
	copy(dst, src)
	if g.rank != 0 {
		recv, err := gatheredBytes(name, 0, recvs[0], len(src))
		if err != nil {
			return err
		}
		copy(dst, recv)
	}
	for rank := 1; rank < g.size; rank++ {
		if rank == g.rank {
			reduceFloat16Bytes(dst, src, dtype, op)
			continue
		}
		recv, err := gatheredBytes(name, rank, recvs[rank], len(src))
		if err != nil {
			return err
		}
		reduceFloat16Bytes(dst, recv, dtype, op)
	}
	return nil
}

func validateDTypeBytes(op string, b []byte, dtype DType) error {
	size, err := dtype.Size()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if len(b)%size != 0 {
		return fmt.Errorf("%s: byte length %d is not a multiple of dtype size %d", op, len(b), size)
	}
	return nil
}

func allGatherBytesLen(size, elemLen int) (int, error) {
	if size < 0 {
		return 0, fmt.Errorf("all gather: group size %d is negative", size)
	}
	if elemLen < 0 {
		return 0, fmt.Errorf("all gather: source length %d is negative", elemLen)
	}
	max := int(^uint(0) >> 1)
	if elemLen != 0 && size > max/elemLen {
		return 0, fmt.Errorf("all gather: destination length overflows int for group size %d and source length %d", size, elemLen)
	}
	return size * elemLen, nil
}

func reduceFunc[T Element](op reduceOp) func([]T, []T) {
	switch op {
	case reduceSum:
		return sum[T]
	case reduceMax:
		return max[T]
	case reduceMin:
		return min[T]
	default:
		panic("unreachable")
	}
}

func gatheredBytes(op string, rank int, got []byte, want int) ([]byte, error) {
	if len(got) != want {
		return nil, fmt.Errorf("%s: rank %d value length %d, want %d", op, rank, len(got), want)
	}
	return got, nil
}

func bytesOf[T Element](x []T) []byte {
	if len(x) == 0 {
		return nil
	}
	size, err := dtypeFor[T]().Size()
	if err != nil {
		panic(err)
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&x[0])), len(x)*size)
}

func typedFromBytes[T Element](b []byte, n int) []T {
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&b[0])), n)
}

func boolsOf(b []byte) []bool {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*bool)(unsafe.Pointer(&b[0])), len(b))
}

func bytesOverlap(a, b []byte) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	ap := uintptr(unsafe.Pointer(&a[0]))
	bp := uintptr(unsafe.Pointer(&b[0]))
	return ap < bp+uintptr(len(b)) && bp < ap+uintptr(len(a))
}

func sum[T Element](dst, src []T) {
	switch d := any(dst).(type) {
	case []bool:
		s := any(src).([]bool)
		for i, v := range s {
			d[i] = d[i] || v
		}
	case []int8:
		add(d, any(src).([]int8))
	case []int16:
		add(d, any(src).([]int16))
	case []int32:
		add(d, any(src).([]int32))
	case []int64:
		add(d, any(src).([]int64))
	case []uint8:
		addUint8(d, any(src).([]uint8))
	case []uint16:
		add(d, any(src).([]uint16))
	case []uint32:
		add(d, any(src).([]uint32))
	case []uint64:
		add(d, any(src).([]uint64))
	case []float32:
		add(d, any(src).([]float32))
	case []float64:
		add(d, any(src).([]float64))
	case []complex64:
		add(d, any(src).([]complex64))
	}
}

type additive interface {
	~int8 | ~int16 | ~int32 | ~int64 |
		~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 | ~complex64
}

func add[T additive](dst, src []T) {
	for i, v := range src {
		dst[i] += v
	}
}

func addUint8(dst, src []uint8) {
	const highBits = uint64(0x8080808080808080)
	const lowBits = uint64(0x7f7f7f7f7f7f7f7f)
	i := 0
	for ; i+8 <= len(src); i += 8 {
		d := *(*uint64)(unsafe.Pointer(&dst[i]))
		s := *(*uint64)(unsafe.Pointer(&src[i]))
		*(*uint64)(unsafe.Pointer(&dst[i])) = ((d & lowBits) + (s & lowBits)) ^ ((d ^ s) & highBits)
	}
	for ; i < len(src); i++ {
		dst[i] += src[i]
	}
}

func max[T Element](dst, src []T) {
	for i, v := range src {
		if less(dst[i], v) {
			dst[i] = v
		}
	}
}

func min[T Element](dst, src []T) {
	for i, v := range src {
		if less(v, dst[i]) {
			dst[i] = v
		}
	}
}

func less[T Element](a, b T) bool {
	switch x := any(a).(type) {
	case bool:
		return !x && any(b).(bool)
	case int8:
		return x < any(b).(int8)
	case int16:
		return x < any(b).(int16)
	case int32:
		return x < any(b).(int32)
	case int64:
		return x < any(b).(int64)
	case uint8:
		return x < any(b).(uint8)
	case uint16:
		return x < any(b).(uint16)
	case uint32:
		return x < any(b).(uint32)
	case uint64:
		return x < any(b).(uint64)
	case float32:
		return x < any(b).(float32)
	case float64:
		return x < any(b).(float64)
	case complex64:
		y := any(b).(complex64)
		return real(x) < real(y) || real(x) == real(y) && imag(x) < imag(y)
	default:
		panic("unreachable")
	}
}
