#include <stddef.h>
#include <stdio.h>
#include <string.h>

#include "mlx/c/config.h"
#include "mlx/c/jaccl.h"

#if MLX_C_HAS_JACCL != 1
#error "MLX_C_HAS_JACCL must be 1 for the JACCL-required consumer"
#endif

static int fail(const char* message) {
  fprintf(stderr, "%s\n", message);
  return 1;
}

static int expect_last_error(const char* step) {
  const char* err = mlx_jaccl_last_error();
  if (err == NULL || err[0] == '\0') {
    fprintf(stderr, "%s: expected last error\n", step);
    return 1;
  }
  return 0;
}

static int expect_clear_error(const char* step) {
  const char* err = mlx_jaccl_last_error();
  if (err != NULL && err[0] != '\0') {
    fprintf(stderr, "%s: expected clear error, got %s\n", step, err);
    return 1;
  }
  return 0;
}

int main(void) {
  if (mlx_jaccl_dtype_size(MLX_JACCL_FLOAT32) != sizeof(float)) {
    return fail("MLX_JACCL_FLOAT32 has unexpected size");
  }
  if (mlx_jaccl_dtype_size((mlx_jaccl_dtype)-1) != 0) {
    return fail("invalid JACCL dtype returned a nonzero size");
  }
  if (expect_last_error("invalid dtype")) {
    return 1;
  }
  mlx_jaccl_clear_error();
  if (expect_clear_error("clear after invalid dtype")) {
    return 1;
  }

  mlx_jaccl_config empty = {0};
  if (mlx_jaccl_config_rank(empty) != -1) {
    return fail("empty JACCL config returned a rank");
  }
  if (expect_last_error("empty config rank")) {
    return 1;
  }

  mlx_jaccl_config config = mlx_jaccl_config_new();
  if (config.ctx == NULL) {
    fprintf(stderr, "new config: %s\n", mlx_jaccl_last_error());
    return 1;
  }
  if (mlx_jaccl_config_set_coordinator(config, NULL) == 0) {
    return fail("null coordinator was accepted");
  }
  if (expect_last_error("null coordinator")) {
    return 1;
  }
  mlx_jaccl_clear_error();

  if (mlx_jaccl_config_set_rank(config, 0) != 0) {
    fprintf(stderr, "set rank: %s\n", mlx_jaccl_last_error());
    return 1;
  }
  if (mlx_jaccl_config_set_coordinator(config, "127.0.0.1:9000") != 0) {
    fprintf(stderr, "set coordinator: %s\n", mlx_jaccl_last_error());
    return 1;
  }
  if (mlx_jaccl_config_rank(config) != 0) {
    return fail("rank getter returned an unexpected value");
  }
  const char* coordinator = mlx_jaccl_config_coordinator(config);
  if (coordinator == NULL || strcmp(coordinator, "127.0.0.1:9000") != 0) {
    return fail("coordinator getter returned an unexpected value");
  }
  if (mlx_jaccl_config_free(config) != 0) {
    fprintf(stderr, "free config: %s\n", mlx_jaccl_last_error());
    return 1;
  }
  if (mlx_jaccl_group_free((mlx_jaccl_group){0}) != 0) {
    fprintf(stderr, "free zero group: %s\n", mlx_jaccl_last_error());
    return 1;
  }
  return 0;
}
