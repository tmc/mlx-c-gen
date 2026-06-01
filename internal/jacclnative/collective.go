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

// AllMax computes the element-wise maximum across all ranks.
func AllMax[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return allReduce(ctx, g, "all max", dst, src, max[T])
}

// AllMin computes the element-wise minimum across all ranks.
func AllMin[T Element](ctx context.Context, g *Group, dst, src []T) error {
	return allReduce(ctx, g, "all min", dst, src, min[T])
}

// AllGather gathers each rank's src into dst in rank order.
func AllGather[T Element](ctx context.Context, g *Group, dst, src []T) error {
	if err := g.check(ctx, "all gather"); err != nil {
		return err
	}
	if len(dst) != g.size*len(src) {
		return fmt.Errorf("all gather: destination length %d, want %d", len(dst), g.size*len(src))
	}
	if g.backend == nil {
		copy(dst, src)
		return nil
	}
	recvs, err := g.backend.gather(ctx, bytesOf(src))
	if err != nil {
		return err
	}
	copy(dst[g.rank*len(src):(g.rank+1)*len(src)], src)
	for peer := 0; peer < g.size; peer++ {
		if peer == g.rank {
			continue
		}
		recv, err := gatheredBytes("all gather", peer, recvs[peer], len(bytesOf(src)))
		if err != nil {
			return err
		}
		copy(bytesOf(dst[peer*len(src):(peer+1)*len(src)]), recv)
	}
	return nil
}

func allReduce[T Element](ctx context.Context, g *Group, name string, dst, src []T, op func([]T, []T)) error {
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
	recvs, err := g.backend.gather(ctx, bytesOf(src))
	if err != nil {
		return err
	}
	copy(dst, src)
	if g.rank != 0 {
		recv, err := gatheredBytes(name, 0, recvs[0], len(bytesOf(src)))
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
		add(d, any(src).([]uint8))
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
