package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ClusterConfig holds configuration for the local development cluster.
// All services run as Docker containers - no local repo clones needed.
type ClusterConfig struct {
	Services  ServiceConfig  `yaml:"services"`
	Endpoints EndpointConfig `yaml:"endpoints"`
	Images    ImageConfig    `yaml:"images"`
	Ports     PortConfig     `yaml:"ports"`
}

// ServiceConfig defines which services run in Docker vs production.
type ServiceConfig struct {
	Postgres       string `yaml:"postgres"`
	Redis          string `yaml:"redis"`
	Minio          string `yaml:"minio"`
	Relay          string `yaml:"relay"`
	Vultiserver    string `yaml:"vultiserver"`
	Verifier       string `yaml:"verifier"`
	VerifierWorker string `yaml:"verifier_worker"`
	DCAServer      string `yaml:"dca_server"`
	DCAWorker      string `yaml:"dca_worker"`
	DCAScheduler   string `yaml:"dca_scheduler"`
	DCATxIndexer   string `yaml:"dca_tx_indexer"`
}

// EndpointConfig holds production endpoint URLs.
type EndpointConfig struct {
	Relay       string `yaml:"relay"`
	Vultiserver string `yaml:"vultiserver"`
}

// ImageConfig allows pinning specific Docker image versions.
type ImageConfig struct {
	Verifier       string `yaml:"verifier"`
	VerifierWorker string `yaml:"verifier_worker"`
	DCAServer      string `yaml:"dca_server"`
	DCAWorker      string `yaml:"dca_worker"`
	DCAScheduler   string `yaml:"dca_scheduler"`
	DCATxIndexer   string `yaml:"dca_tx_indexer"`
}

// PortConfig holds port numbers for local services.
type PortConfig struct {
	Verifier              int `yaml:"verifier"`
	VerifierWorkerMetrics int `yaml:"verifier_worker_metrics"`
	DCAServer             int `yaml:"dca_server"`
	DCAWorkerMetrics      int `yaml:"dca_worker_metrics"`
	DCASchedulerMetrics   int `yaml:"dca_scheduler_metrics"`
	DCATxIndexerMetrics   int `yaml:"dca_tx_indexer_metrics"`
	Vultiserver           int `yaml:"vultiserver"`
	Relay                 int `yaml:"relay"`
	Postgres              int `yaml:"postgres"`
	Redis                 int `yaml:"redis"`
	Minio                 int `yaml:"minio"`
	MinioConsole          int `yaml:"minio_console"`
}

var clusterConfig *ClusterConfig

// LoadClusterConfig loads the cluster configuration from cluster.yaml.
// It searches in: current directory, local/, and ~/.vultisig/
func LoadClusterConfig() (*ClusterConfig, error) {
	if clusterConfig != nil {
		return clusterConfig, nil
	}

	configPaths := []string{
		"cluster.yaml",
		"local/cluster.yaml",
		filepath.Join(os.Getenv("HOME"), ".vultisig", "cluster.yaml"),
	}

	var configPath string
	for _, p := range configPaths {
		if _, err := os.Stat(p); err == nil {
			configPath = p
			break
		}
	}

	if configPath == "" {
		// Return default config if no file found
		config := &ClusterConfig{}
		config.setDefaults()
		clusterConfig = config
		return clusterConfig, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read cluster.yaml: %w", err)
	}

	config := &ClusterConfig{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, fmt.Errorf("parse cluster.yaml: %w", err)
	}

	config.setDefaults()

	clusterConfig = config
	return clusterConfig, nil
}

func (c *ClusterConfig) setDefaults() {
	// Default endpoints
	if c.Endpoints.Relay == "" {
		c.Endpoints.Relay = "https://api.vultisig.com/router"
	}
	if c.Endpoints.Vultiserver == "" {
		c.Endpoints.Vultiserver = "https://api.vultisig.com"
	}

	// Default ports
	if c.Ports.Verifier == 0 {
		c.Ports.Verifier = 8080
	}
	if c.Ports.VerifierWorkerMetrics == 0 {
		c.Ports.VerifierWorkerMetrics = 8089
	}
	if c.Ports.DCAServer == 0 {
		c.Ports.DCAServer = 8082
	}
	if c.Ports.DCAWorkerMetrics == 0 {
		c.Ports.DCAWorkerMetrics = 8183
	}
	if c.Ports.DCASchedulerMetrics == 0 {
		c.Ports.DCASchedulerMetrics = 8185
	}
	if c.Ports.DCATxIndexerMetrics == 0 {
		c.Ports.DCATxIndexerMetrics = 8187
	}
	if c.Ports.Postgres == 0 {
		c.Ports.Postgres = 5432
	}
	if c.Ports.Redis == 0 {
		c.Ports.Redis = 6379
	}
	if c.Ports.Minio == 0 {
		c.Ports.Minio = 9000
	}
	if c.Ports.MinioConsole == 0 {
		c.Ports.MinioConsole = 9090
	}

	// Default service modes to docker
	if c.Services.Postgres == "" {
		c.Services.Postgres = "docker"
	}
	if c.Services.Redis == "" {
		c.Services.Redis = "docker"
	}
	if c.Services.Minio == "" {
		c.Services.Minio = "docker"
	}
	if c.Services.Relay == "" {
		c.Services.Relay = "production"
	}
	if c.Services.Vultiserver == "" {
		c.Services.Vultiserver = "production"
	}
	if c.Services.Verifier == "" {
		c.Services.Verifier = "docker"
	}
	if c.Services.VerifierWorker == "" {
		c.Services.VerifierWorker = "docker"
	}
	if c.Services.DCAServer == "" {
		c.Services.DCAServer = "docker"
	}
	if c.Services.DCAWorker == "" {
		c.Services.DCAWorker = "docker"
	}
	if c.Services.DCAScheduler == "" {
		c.Services.DCAScheduler = "docker"
	}
	if c.Services.DCATxIndexer == "" {
		c.Services.DCATxIndexer = "docker"
	}

	// Default Docker images (use latest)
	if c.Images.Verifier == "" {
		c.Images.Verifier = "ghcr.io/vultisig/verifier:latest"
	}
	if c.Images.VerifierWorker == "" {
		c.Images.VerifierWorker = "ghcr.io/vultisig/verifier-worker:latest"
	}
	if c.Images.DCAServer == "" {
		c.Images.DCAServer = "ghcr.io/vultisig/dca-server:latest"
	}
	if c.Images.DCAWorker == "" {
		c.Images.DCAWorker = "ghcr.io/vultisig/dca-worker:latest"
	}
	if c.Images.DCAScheduler == "" {
		c.Images.DCAScheduler = "ghcr.io/vultisig/dca-scheduler:latest"
	}
	if c.Images.DCATxIndexer == "" {
		c.Images.DCATxIndexer = "ghcr.io/vultisig/dca-tx-indexer:latest"
	}
}

// IsDocker returns true if the service runs as a Docker container.
func (c *ClusterConfig) IsDocker(service string) bool {
	switch service {
	case "relay":
		return c.Services.Relay == "docker"
	case "vultiserver":
		return c.Services.Vultiserver == "docker"
	case "verifier":
		return c.Services.Verifier == "docker"
	case "dca":
		return c.Services.DCAServer == "docker"
	default:
		return true
	}
}

// IsProduction returns true if the service uses production endpoints.
func (c *ClusterConfig) IsProduction(service string) bool {
	switch service {
	case "relay":
		return c.Services.Relay == "production"
	case "vultiserver":
		return c.Services.Vultiserver == "production"
	default:
		return false
	}
}

// GetRelayURL returns the relay URL based on configuration.
func (c *ClusterConfig) GetRelayURL() string {
	if c.IsDocker("relay") {
		return fmt.Sprintf("http://localhost:%d", c.Ports.Relay)
	}
	return c.Endpoints.Relay
}

// GetVultiserverURL returns the vultiserver URL based on configuration.
func (c *ClusterConfig) GetVultiserverURL() string {
	if c.IsDocker("vultiserver") {
		return fmt.Sprintf("http://localhost:%d", c.Ports.Vultiserver)
	}
	return c.Endpoints.Vultiserver
}

// GetVerifierURL returns the verifier URL (always local for development).
func (c *ClusterConfig) GetVerifierURL() string {
	return fmt.Sprintf("http://localhost:%d", c.Ports.Verifier)
}

// GetDCAServerURL returns the DCA server URL (always local for development).
func (c *ClusterConfig) GetDCAServerURL() string {
	return fmt.Sprintf("http://localhost:%d", c.Ports.DCAServer)
}
