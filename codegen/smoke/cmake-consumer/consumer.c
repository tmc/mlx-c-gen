#include <stdio.h>

#include "mlx/c/mlx.h"

#if MLX_C_CONSUMER_HAS_JACCL
#include "mlx/c/jaccl.h"
#endif

int main(void) {
  mlx_string version = mlx_string_new();
  mlx_version(&version);
  printf("MLX version: %s\n", mlx_string_data(version));
  mlx_string_free(version);

#if MLX_C_CONSUMER_HAS_JACCL
  printf("JACCL float32 bytes: %zu\n", mlx_jaccl_dtype_size(MLX_JACCL_FLOAT32));
#endif

  return 0;
}
