/* Copyright © 2026 Apple Inc. */

#include "mlx/c/jaccl.h"

#include <cstdint>
#include <exception>
#include <fstream>
#include <memory>
#include <optional>
#include <sstream>
#include <stdexcept>
#include <string>
#include <type_traits>
#include <utility>
#include <vector>

#if __has_include(<json.hpp>)
#include <json.hpp>
#define MLX_JACCL_HAS_JSON 1
#else
#define MLX_JACCL_HAS_JSON 0
#endif

#include "jaccl/jaccl.h"

namespace {

thread_local std::string mlx_jaccl_error_;
thread_local std::string mlx_jaccl_string_;

void clear_error() {
  mlx_jaccl_error_.clear();
}

int fail(const char* msg) {
  mlx_jaccl_error_ = msg;
  return 1;
}

int fail(const std::string& msg) {
  mlx_jaccl_error_ = msg;
  return 1;
}

int fail(const std::exception& e) {
  mlx_jaccl_error_ = e.what();
  return 1;
}

std::shared_ptr<jaccl::Group>& group_get(mlx_jaccl_group group) {
  if (!group.ctx) {
    throw std::runtime_error("expected a non-empty mlx_jaccl_group");
  }

  auto& ptr = *static_cast<std::shared_ptr<jaccl::Group>*>(group.ctx);
  if (!ptr) {
    throw std::runtime_error("expected an initialized mlx_jaccl_group");
  }
  return ptr;
}

jaccl::Config& config_get(mlx_jaccl_config config) {
  if (!config.ctx) {
    throw std::runtime_error("expected a non-empty mlx_jaccl_config");
  }
  return *static_cast<jaccl::Config*>(config.ctx);
}

int dtype_to_jaccl(mlx_jaccl_dtype dtype) {
  switch (dtype) {
    case MLX_JACCL_BOOL:
      return jaccl::Bool;
    case MLX_JACCL_INT8:
      return jaccl::Int8;
    case MLX_JACCL_INT16:
      return jaccl::Int16;
    case MLX_JACCL_INT32:
      return jaccl::Int32;
    case MLX_JACCL_INT64:
      return jaccl::Int64;
    case MLX_JACCL_UINT8:
      return jaccl::UInt8;
    case MLX_JACCL_UINT16:
      return jaccl::UInt16;
    case MLX_JACCL_UINT32:
      return jaccl::UInt32;
    case MLX_JACCL_UINT64:
      return jaccl::UInt64;
    case MLX_JACCL_FLOAT16:
      return jaccl::Float16;
    case MLX_JACCL_BFLOAT16:
      return jaccl::BFloat16;
    case MLX_JACCL_FLOAT32:
      return jaccl::Float32;
    case MLX_JACCL_FLOAT64:
      return jaccl::Float64;
    case MLX_JACCL_COMPLEX64:
      return jaccl::Complex64;
  }

  throw std::invalid_argument("invalid mlx_jaccl_dtype");
}

#define MLX_JACCL_DTYPE_ASSERT(c_name, cpp_name)                     \
  static_assert(                                                     \
      static_cast<int>(c_name) == static_cast<int>(jaccl::cpp_name), \
      "mlx_jaccl_dtype " #cpp_name " mismatch")

MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_BOOL, Bool);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_INT8, Int8);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_INT16, Int16);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_INT32, Int32);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_INT64, Int64);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_UINT8, UInt8);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_UINT16, UInt16);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_UINT32, UInt32);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_UINT64, UInt64);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_FLOAT16, Float16);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_BFLOAT16, BFloat16);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_FLOAT32, Float32);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_FLOAT64, Float64);
MLX_JACCL_DTYPE_ASSERT(MLX_JACCL_COMPLEX64, Complex64);

#undef MLX_JACCL_DTYPE_ASSERT

bool invalid_buffer(const void* ptr, size_t n_bytes) {
  return n_bytes != 0 && ptr == nullptr;
}

int validate_typed_bytes(
    const char* function,
    size_t n_bytes,
    mlx_jaccl_dtype dtype) {
  size_t elem_size = mlx_jaccl_dtype_size(dtype);
  if (elem_size == 0) {
    return 1;
  }
  if (n_bytes % elem_size != 0) {
    std::ostringstream msg;
    msg << function << ": n_bytes is not a multiple of dtype size";
    return fail(msg.str());
  }
  return 0;
}

template <typename T, typename = void>
struct has_barrier : std::false_type {};

template <typename T>
struct has_barrier<T, std::void_t<decltype(std::declval<T&>().barrier())>>
    : std::true_type {};

template <typename T>
void barrier(T& group) {
  if constexpr (has_barrier<T>::value) {
    group.barrier();
  } else {
    int input = 1;
    int output = 0;
    group.all_sum(&input, &output, sizeof(input), jaccl::Int32);
  }
}

#if MLX_JACCL_HAS_JSON
std::vector<std::vector<std::vector<std::string>>> parse_devices_json(
    std::istream& input) {
  nlohmann::json devices = nlohmann::json::parse(input);
  if (!devices.is_array()) {
    throw std::runtime_error(
        "[jaccl] the device json should start with an array");
  }

  std::vector<std::vector<std::vector<std::string>>> result(devices.size());
  for (size_t rank = 0; rank < devices.size(); rank++) {
    auto conn = devices[rank];
    if (!conn.is_array()) {
      throw std::runtime_error(
          "[jaccl] the device json should have an array of arrays");
    }
    if (conn.size() != devices.size()) {
      std::ostringstream msg;
      msg << "[jaccl] the device json should contain connectivity from each "
          << "rank to all other ranks but rank " << rank << " contains only "
          << conn.size() << " entries";
      throw std::runtime_error(msg.str());
    }

    result[rank].resize(conn.size());
    for (size_t dst = 0; dst < conn.size(); dst++) {
      auto names = conn[dst];
      if (names.is_string()) {
        result[rank][dst].push_back(names.get<std::string>());
      } else if (names.is_array()) {
        for (auto name_it = names.begin(); name_it != names.end(); name_it++) {
          if (!name_it->is_string()) {
            throw std::runtime_error(
                "[jaccl] device name arrays should contain strings");
          }
          result[rank][dst].push_back(name_it->get<std::string>());
        }
      } else if (!names.is_null()) {
        throw std::runtime_error(
            "[jaccl] device names should be null, a string, or an array of strings");
      }
    }
  }

  return result;
}
#endif

} // namespace

extern "C" size_t mlx_jaccl_dtype_size(mlx_jaccl_dtype dtype) {
  switch (dtype) {
    case MLX_JACCL_BOOL:
      clear_error();
      return sizeof(bool);
    case MLX_JACCL_INT8:
      clear_error();
      return sizeof(int8_t);
    case MLX_JACCL_INT16:
      clear_error();
      return sizeof(int16_t);
    case MLX_JACCL_INT32:
      clear_error();
      return sizeof(int32_t);
    case MLX_JACCL_INT64:
      clear_error();
      return sizeof(int64_t);
    case MLX_JACCL_UINT8:
      clear_error();
      return sizeof(uint8_t);
    case MLX_JACCL_UINT16:
      clear_error();
      return sizeof(uint16_t);
    case MLX_JACCL_UINT32:
      clear_error();
      return sizeof(uint32_t);
    case MLX_JACCL_UINT64:
      clear_error();
      return sizeof(uint64_t);
    case MLX_JACCL_FLOAT16:
      clear_error();
      return 2;
    case MLX_JACCL_BFLOAT16:
      clear_error();
      return 2;
    case MLX_JACCL_FLOAT32:
      clear_error();
      return sizeof(float);
    case MLX_JACCL_FLOAT64:
      clear_error();
      return sizeof(double);
    case MLX_JACCL_COMPLEX64:
      clear_error();
      return 2 * sizeof(float);
  }

  fail("invalid mlx_jaccl_dtype");
  return 0;
}

extern "C" const char* mlx_jaccl_last_error(void) {
  return mlx_jaccl_error_.c_str();
}

extern "C" void mlx_jaccl_clear_error(void) {
  clear_error();
}

extern "C" mlx_jaccl_group mlx_jaccl_group_new(void) {
  return {nullptr};
}

extern "C" int mlx_jaccl_group_free(mlx_jaccl_group group) {
  try {
    delete static_cast<std::shared_ptr<jaccl::Group>*>(group.ctx);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" mlx_jaccl_config mlx_jaccl_config_new(void) {
  try {
    auto config = new jaccl::Config();
    config->prefer_ring(false);
    clear_error();
    return {config};
  } catch (std::exception& e) {
    fail(e);
    return {nullptr};
  }
}

extern "C" mlx_jaccl_config mlx_jaccl_config_from_env(void) {
  try {
    auto config = jaccl::Config::from_env();
    if (!config) {
      fail("jaccl environment variables missing or invalid");
      return {nullptr};
    }

    clear_error();
    return {new jaccl::Config(std::move(*config))};
  } catch (std::exception& e) {
    fail(e);
    return {nullptr};
  }
}

extern "C" int mlx_jaccl_config_free(mlx_jaccl_config config) {
  try {
    delete static_cast<jaccl::Config*>(config.ctx);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_config_set_rank(mlx_jaccl_config config, int rank) {
  try {
    config_get(config).set_rank(rank);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_config_rank(mlx_jaccl_config config) {
  try {
    int rank = config_get(config).get_rank();
    clear_error();
    return rank;
  } catch (std::exception& e) {
    fail(e);
    return -1;
  }
}

extern "C" int mlx_jaccl_config_set_coordinator(
    mlx_jaccl_config config,
    const char* coordinator) {
  if (!coordinator) {
    return fail("mlx_jaccl_config_set_coordinator: null coordinator");
  }

  try {
    config_get(config).set_coordinator(coordinator);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" const char* mlx_jaccl_config_coordinator(mlx_jaccl_config config) {
  try {
    mlx_jaccl_string_ = config_get(config).get_coordinator();
    clear_error();
    return mlx_jaccl_string_.c_str();
  } catch (std::exception& e) {
    fail(e);
    return nullptr;
  }
}

extern "C" int mlx_jaccl_config_set_devices_file(
    mlx_jaccl_config config,
    const char* path) {
  if (!path) {
    return fail("mlx_jaccl_config_set_devices_file: null path");
  }

  try {
#if MLX_JACCL_HAS_JSON
    std::ifstream input(path);
    if (!input) {
      return fail("mlx_jaccl_config_set_devices_file: open failed");
    }
    config_get(config).set_devices(parse_devices_json(input));
    clear_error();
    return 0;
#else
    return fail(
        "mlx_jaccl_config_set_devices_file: json support not available");
#endif
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_config_set_devices_json(
    mlx_jaccl_config config,
    const char* json) {
  if (!json) {
    return fail("mlx_jaccl_config_set_devices_json: null json");
  }

  try {
#if MLX_JACCL_HAS_JSON
    std::istringstream input(json);
    config_get(config).set_devices(parse_devices_json(input));
    clear_error();
    return 0;
#else
    return fail(
        "mlx_jaccl_config_set_devices_json: json support not available");
#endif
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_config_prefer_ring(
    mlx_jaccl_config config,
    bool prefer) {
  try {
    config_get(config).prefer_ring(prefer);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" bool mlx_jaccl_config_prefers_ring(mlx_jaccl_config config) {
  try {
    bool prefer = config_get(config).get_prefer_ring();
    clear_error();
    return prefer;
  } catch (std::exception& e) {
    fail(e);
    return false;
  }
}

extern "C" int mlx_jaccl_config_size(mlx_jaccl_config config) {
  try {
    int size = config_get(config).get_size();
    clear_error();
    return size;
  } catch (std::exception& e) {
    fail(e);
    return -1;
  }
}

extern "C" bool mlx_jaccl_config_is_valid_mesh(mlx_jaccl_config config) {
  try {
    bool valid = config_get(config).is_valid_mesh();
    clear_error();
    return valid;
  } catch (std::exception& e) {
    fail(e);
    return false;
  }
}

extern "C" bool mlx_jaccl_config_is_valid_ring(mlx_jaccl_config config) {
  try {
    bool valid = config_get(config).is_valid_ring();
    clear_error();
    return valid;
  } catch (std::exception& e) {
    fail(e);
    return false;
  }
}

extern "C" bool mlx_jaccl_is_available(void) {
  try {
    bool available = jaccl::is_available();
    clear_error();
    return available;
  } catch (std::exception& e) {
    fail(e);
    return false;
  }
}

extern "C" int mlx_jaccl_init(mlx_jaccl_group* res, bool strict) {
  if (!res) {
    return fail("mlx_jaccl_init: null result pointer");
  }

  try {
    auto group = jaccl::init(strict);
    if (!group) {
      *res = {nullptr};
      return fail("jaccl init returned no group");
    }

    *res = {new std::shared_ptr<jaccl::Group>(std::move(group))};
    clear_error();
    return 0;
  } catch (std::exception& e) {
    *res = {nullptr};
    return fail(e);
  }
}

extern "C" int mlx_jaccl_init_config(
    mlx_jaccl_group* res,
    mlx_jaccl_config config,
    bool strict) {
  if (!res) {
    return fail("mlx_jaccl_init_config: null result pointer");
  }

  try {
    auto group = jaccl::init(config_get(config), strict);
    if (!group) {
      *res = {nullptr};
      return fail("jaccl init returned no group");
    }

    *res = {new std::shared_ptr<jaccl::Group>(std::move(group))};
    clear_error();
    return 0;
  } catch (std::exception& e) {
    *res = {nullptr};
    return fail(e);
  }
}

extern "C" int mlx_jaccl_group_rank(mlx_jaccl_group group) {
  try {
    int rank = group_get(group)->rank();
    clear_error();
    return rank;
  } catch (std::exception& e) {
    fail(e);
    return -1;
  }
}

extern "C" int mlx_jaccl_group_size(mlx_jaccl_group group) {
  try {
    int size = group_get(group)->size();
    clear_error();
    return size;
  } catch (std::exception& e) {
    fail(e);
    return -1;
  }
}

extern "C" int mlx_jaccl_barrier(mlx_jaccl_group group) {
  try {
    barrier(*group_get(group));
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_all_sum(
    mlx_jaccl_group group,
    const void* input,
    void* output,
    size_t n_bytes,
    mlx_jaccl_dtype dtype) {
  if (invalid_buffer(input, n_bytes)) {
    return fail("mlx_jaccl_all_sum: null input");
  }
  if (invalid_buffer(output, n_bytes)) {
    return fail("mlx_jaccl_all_sum: null output");
  }
  if (validate_typed_bytes("mlx_jaccl_all_sum", n_bytes, dtype)) {
    return 1;
  }

  try {
    group_get(group)->all_sum(input, output, n_bytes, dtype_to_jaccl(dtype));
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_all_max(
    mlx_jaccl_group group,
    const void* input,
    void* output,
    size_t n_bytes,
    mlx_jaccl_dtype dtype) {
  if (invalid_buffer(input, n_bytes)) {
    return fail("mlx_jaccl_all_max: null input");
  }
  if (invalid_buffer(output, n_bytes)) {
    return fail("mlx_jaccl_all_max: null output");
  }
  if (validate_typed_bytes("mlx_jaccl_all_max", n_bytes, dtype)) {
    return 1;
  }

  try {
    group_get(group)->all_max(input, output, n_bytes, dtype_to_jaccl(dtype));
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_all_min(
    mlx_jaccl_group group,
    const void* input,
    void* output,
    size_t n_bytes,
    mlx_jaccl_dtype dtype) {
  if (invalid_buffer(input, n_bytes)) {
    return fail("mlx_jaccl_all_min: null input");
  }
  if (invalid_buffer(output, n_bytes)) {
    return fail("mlx_jaccl_all_min: null output");
  }
  if (validate_typed_bytes("mlx_jaccl_all_min", n_bytes, dtype)) {
    return 1;
  }

  try {
    group_get(group)->all_min(input, output, n_bytes, dtype_to_jaccl(dtype));
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_all_gather(
    mlx_jaccl_group group,
    const void* input,
    void* output,
    size_t n_bytes) {
  if (invalid_buffer(input, n_bytes)) {
    return fail("mlx_jaccl_all_gather: null input");
  }
  if (invalid_buffer(output, n_bytes)) {
    return fail("mlx_jaccl_all_gather: null output");
  }

  try {
    group_get(group)->all_gather(input, output, n_bytes);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int mlx_jaccl_send(
    mlx_jaccl_group group,
    const void* input,
    size_t n_bytes,
    int dst) {
  if (invalid_buffer(input, n_bytes)) {
    return fail("mlx_jaccl_send: null input");
  }

  try {
    group_get(group)->send(input, n_bytes, dst);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}

extern "C" int
mlx_jaccl_recv(mlx_jaccl_group group, void* output, size_t n_bytes, int src) {
  if (invalid_buffer(output, n_bytes)) {
    return fail("mlx_jaccl_recv: null output");
  }

  try {
    group_get(group)->recv(output, n_bytes, src);
    clear_error();
    return 0;
  } catch (std::exception& e) {
    return fail(e);
  }
}
