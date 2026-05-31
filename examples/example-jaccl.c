/* Copyright © 2026 Apple Inc. */

#include <stdio.h>

#include "mlx/c/jaccl.h"

int main(void) {
  printf("JACCL available: %s\n", mlx_jaccl_is_available() ? "yes" : "no");
  printf("JACCL float32 bytes: %zu\n", mlx_jaccl_dtype_size(MLX_JACCL_FLOAT32));

  mlx_jaccl_config config = mlx_jaccl_config_new();
  if (!config.ctx) {
    fprintf(stderr, "JACCL config: %s\n", mlx_jaccl_last_error());
    return 1;
  }

  if (mlx_jaccl_config_set_rank(config, 0) ||
      mlx_jaccl_config_set_coordinator(config, "127.0.0.1:0") ||
      mlx_jaccl_config_set_devices_json(
          config, "[[null,\"rdma0\"],[\"rdma1\",null]]")) {
    fprintf(stderr, "JACCL config JSON skipped: %s\n", mlx_jaccl_last_error());
    mlx_jaccl_clear_error();
  } else {
    printf(
        "JACCL config mesh: %s\n",
        mlx_jaccl_config_is_valid_mesh(config) ? "valid" : "invalid");
  }

  mlx_jaccl_config env_config = mlx_jaccl_config_from_env();
  if (env_config.ctx) {
    if (mlx_jaccl_config_free(env_config)) {
      fprintf(stderr, "JACCL env config: %s\n", mlx_jaccl_last_error());
    }
  } else {
    mlx_jaccl_clear_error();
  }

  mlx_jaccl_config_free(config);
  return 0;
}
