#include <stddef.h>

#include "mlx/c/config.h"
#include "mlx/c/jaccl.h"

#if MLX_C_HAS_JACCL != 1
#error "MLX_C_HAS_JACCL must be 1 for the JACCL-required consumer"
#endif

int main(void) {
  return mlx_jaccl_dtype_size(MLX_JACCL_FLOAT32) == sizeof(float) ? 0 : 1;
}
