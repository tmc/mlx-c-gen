#include <stdio.h>

#include "mlx/c/mlx.h"

#ifndef MLX_C_HAS_JACCL
#error "mlx/c/config.h did not define MLX_C_HAS_JACCL"
#endif

#if MLX_C_HAS_JACCL != MLX_C_CONSUMER_HAS_JACCL
#error "MLX_C_HAS_JACCL does not match the MLXC CMake package"
#endif

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
  mlx_jaccl_config config = mlx_jaccl_config_new();
  if (config.ctx) {
    mlx_jaccl_config_set_rank(config, 0);
    mlx_jaccl_config_set_coordinator(config, "127.0.0.1:9000");
    printf("JACCL config rank: %d\n", mlx_jaccl_config_rank(config));
    printf(
        "JACCL config coordinator: %s\n", mlx_jaccl_config_coordinator(config));
    mlx_jaccl_config_free(config);
  }
#endif

  return 0;
}
