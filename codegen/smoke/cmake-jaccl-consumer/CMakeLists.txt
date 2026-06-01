cmake_minimum_required(VERSION 3.16)

project(mlx-c-cmake-jaccl-consumer LANGUAGES C)

find_package(MLXC REQUIRED)

if(NOT MLX_C_HAS_JACCL)
  message(FATAL_ERROR "MLXC package was built without JACCL")
endif()

if(NOT TARGET jacclc)
  message(FATAL_ERROR "MLXC package declares JACCL but did not define jacclc")
endif()

add_executable(consumer consumer.c)
target_link_libraries(consumer PRIVATE jacclc)
