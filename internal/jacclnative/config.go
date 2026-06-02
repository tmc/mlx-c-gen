package jacclnative

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Config describes one rank in a JACCL group.
type Config struct {
	Rank               int
	Size               int
	Coordinator        string
	Devices            [][][]string
	PreferRing         bool
	ZeroDLIDWhenGlobal bool
	GRHHopLimit        uint8
}

// ConfigFromEnv reads the JACCL environment variables used by C++ JACCL.
func ConfigFromEnv() (Config, error) {
	var cfg Config
	rankText, ok := getenv("JACCL_RANK", "MLX_RANK")
	if !ok {
		return Config{}, fmt.Errorf("rank: missing JACCL_RANK or MLX_RANK")
	}
	rank, err := strconv.Atoi(rankText)
	if err != nil {
		return Config{}, fmt.Errorf("rank: parse %q: %w", rankText, err)
	}
	cfg.Rank = rank
	if sizeText, ok := getenvAny("JACCL_SIZE", "MLX_WORLD_SIZE", "MLX_SIZE"); ok {
		size, err := strconv.Atoi(sizeText)
		if err != nil {
			return Config{}, fmt.Errorf("size: parse %q: %w", sizeText, err)
		}
		cfg.Size = size
	}
	coord, ok := getenv("JACCL_COORDINATOR", "MLX_JACCL_COORDINATOR")
	if !ok {
		return Config{}, fmt.Errorf("coordinator: missing JACCL_COORDINATOR or MLX_JACCL_COORDINATOR")
	}
	cfg.Coordinator = coord
	path, ok := getenv("JACCL_IBV_DEVICES", "MLX_IBV_DEVICES")
	if !ok {
		return Config{}, fmt.Errorf("devices: missing JACCL_IBV_DEVICES or MLX_IBV_DEVICES")
	}
	devices, err := readDeviceMatrix(path)
	if err != nil {
		return Config{}, err
	}
	cfg.Devices = devices
	if _, ok := getenv("JACCL_RING", "MLX_JACCL_RING"); ok {
		cfg.PreferRing = true
	}
	if _, ok := getenv("JACCL_ZERO_DLID_WHEN_GLOBAL", "MLX_JACCL_ZERO_DLID_WHEN_GLOBAL"); ok {
		cfg.ZeroDLIDWhenGlobal = true
	}
	if hopText, ok := getenv("JACCL_GRH_HOP_LIMIT", "MLX_JACCL_GRH_HOP_LIMIT"); ok {
		hop, err := strconv.Atoi(hopText)
		if err != nil {
			return Config{}, fmt.Errorf("grh hop limit: parse %q: %w", hopText, err)
		}
		if hop < 0 || hop > 255 {
			return Config{}, fmt.Errorf("grh hop limit %d out of uint8 range", hop)
		}
		cfg.GRHHopLimit = uint8(hop)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Rank < 0 {
		return fmt.Errorf("rank %d out of range", c.Rank)
	}
	size, err := c.groupSize()
	if err != nil {
		return err
	}
	if c.Rank >= size {
		return fmt.Errorf("rank %d out of range for size %d", c.Rank, size)
	}
	return nil
}

// GroupSize reports the configured group size.
func (c Config) GroupSize() (int, error) {
	return c.groupSize()
}

// IsValidMesh reports whether every pair of ranks has a configured RDMA path.
func (c Config) IsValidMesh() bool {
	return isMesh(c)
}

// IsValidRing reports whether the device matrix has the bidirectional ring
// edges needed by JACCL ring topology.
func (c Config) IsValidRing() bool {
	return isRing(c)
}

func (c Config) groupSize() (int, error) {
	if len(c.Devices) > 0 {
		if err := validateDeviceMatrix(c.Devices); err != nil {
			return 0, err
		}
		if c.Size > 0 && c.Size != len(c.Devices) {
			return 0, fmt.Errorf("size %d does not match device matrix size %d", c.Size, len(c.Devices))
		}
		return len(c.Devices), nil
	}
	if c.Size <= 0 {
		return 0, fmt.Errorf("size %d is not positive", c.Size)
	}
	return c.Size, nil
}

func validateDeviceMatrix(devices [][][]string) error {
	if len(devices) == 0 {
		return fmt.Errorf("device matrix is empty")
	}
	for src, row := range devices {
		if len(row) != len(devices) {
			return fmt.Errorf("device matrix row %d has size %d, want %d", src, len(row), len(devices))
		}
	}
	return nil
}

func readDeviceMatrix(path string) ([][][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("devices: read %s: %w", path, err)
	}
	var matrix [][][]string
	if err := json.Unmarshal(data, &matrix); err != nil {
		return nil, fmt.Errorf("devices: parse %s: %w", path, err)
	}
	return matrix, nil
}

func getenv(primary, fallback string) (string, bool) {
	if v, ok := os.LookupEnv(primary); ok {
		return v, true
	}
	return os.LookupEnv(fallback)
}

func getenvAny(names ...string) (string, bool) {
	for _, name := range names {
		if v, ok := os.LookupEnv(name); ok {
			return v, true
		}
	}
	return "", false
}
