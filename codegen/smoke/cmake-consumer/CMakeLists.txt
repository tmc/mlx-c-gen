cmake_minimum_required(VERSION 3.16)

project(mlx-c-cmake-consumer LANGUAGES C)

find_package(MLXC REQUIRED)

if(NOT TARGET mlxc)
  message(FATAL_ERROR "MLXC package did not define mlxc target")
endif()

add_executable(consumer consumer.c)
target_link_libraries(consumer PRIVATE mlxc)

if(MLX_C_HAS_JACCL AND NOT TARGET jacclc)
  message(FATAL_ERROR "MLXC package declares JACCL but did not define jacclc")
endif()

if(MLX_C_HAS_JACCL)
  target_compile_definitions(consumer PRIVATE MLX_C_CONSUMER_HAS_JACCL=1)
  target_link_libraries(consumer PRIVATE jacclc)
else()
  target_compile_definitions(consumer PRIVATE MLX_C_CONSUMER_HAS_JACCL=0)
endif()
