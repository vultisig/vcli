package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ClusterConfig struct {
	Repos     RepoConfig     `yaml:"repos"`
	Services  ServiceConfig  `yaml:"services"`
	Endpoints EndpointConfig `yaml:"endpoints"`
	Library   LibraryConfig  `yaml:"library"`
	Ports     PortConfig     `yaml:"ports"`
}

type RepoConfig struct {
	Verifier    string `yaml:"verifier"`
	Vultiserver string `yaml:"vultiserver"`
	Relay       string `yaml:"relay"`
	DCA         string `yaml:"dca"`
	GoWrappers  string `yaml:"go_wrappers"`
}

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

type EndpointConfig struct {
	Relay       string `yaml:"relay"`
	Vultiserver string `yaml:"vultiserver"`
}

type LibraryConfig struct {
	DYLDPath string `yaml:"dyld_path"`
}

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
		return nil, fmt.Errorf("cluster.yaml not found. Copy cluster.yaml.example to cluster.yaml and configure paths")
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

	config.expandPaths()
	config.setDefaults()

	clusterConfig = config
	return clusterConfig, nil
}

func (c *ClusterConfig) expandPaths() {
	home := os.Getenv("HOME")
	expand := func(p string) string {
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:])
		}
		return p
	}

	c.Repos.Verifier = expand(c.Repos.Verifier)
	c.Repos.Vultiserver = expand(c.Repos.Vultiserver)
	c.Repos.Relay = expand(c.Repos.Relay)
	c.Repos.DCA = expand(c.Repos.DCA)
	c.Repos.GoWrappers = expand(c.Repos.GoWrappers)
	c.Library.DYLDPath = expand(c.Library.DYLDPath)
}

func (c *ClusterConfig) setDefaults() {
	if c.Endpoints.Relay == "" {
		c.Endpoints.Relay = "https://api.vultisig.com/router"
	}
	if c.Endpoints.Vultiserver == "" {
		c.Endpoints.Vultiserver = "https://api.vultisig.com"
	}

	if c.Ports.Verifier == 0 {
		c.Ports.Verifier = 8080
	}
	if c.Ports.DCAServer == 0 {
		c.Ports.DCAServer = 8082
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
}

func (c *ClusterConfig) IsLocal(service string) bool {
	switch service {
	case "relay":
		return c.Services.Relay == "local"
	case "vultiserver":
		return c.Services.Vultiserver == "local"
	case "verifier":
		return c.Services.Verifier == "local"
	case "dca":
		return c.Services.DCAServer == "local"
	default:
		return true
	}
}

func (c *ClusterConfig) GetRelayURL() string {
	if c.IsLocal("relay") {
		return fmt.Sprintf("http://localhost:%d", c.Ports.Relay)
	}
	return c.Endpoints.Relay
}

func (c *ClusterConfig) GetVultiserverURL() string {
	if c.IsLocal("vultiserver") {
		return fmt.Sprintf("http://localhost:%d", c.Ports.Vultiserver)
	}
	return c.Endpoints.Vultiserver
}

func (c *ClusterConfig) GetDYLDPath() string {
	return c.Library.DYLDPath
}

// ApplyMode overrides service settings based on mode:
// - local: all services run locally
// - dev: relay and vultiserver use production, rest local
// - prod: all services use production endpoints
func (c *ClusterConfig) ApplyMode(mode string) {
	switch mode {
	case "local":
		c.Services.Relay = "local"
		c.Services.Vultiserver = "local"
		c.Services.Verifier = "local"
		c.Services.VerifierWorker = "local"
		c.Services.DCAServer = "local"
		c.Services.DCAWorker = "local"
		c.Services.DCAScheduler = "local"
		c.Services.DCATxIndexer = "local"
	case "dev":
		c.Services.Relay = "production"
		c.Services.Vultiserver = "production"
		c.Services.Verifier = "local"
		c.Services.VerifierWorker = "local"
		c.Services.DCAServer = "local"
		c.Services.DCAWorker = "local"
		c.Services.DCAScheduler = "local"
		c.Services.DCATxIndexer = "local"
	case "prod":
		c.Services.Relay = "production"
		c.Services.Vultiserver = "production"
		c.Services.Verifier = "production"
		c.Services.VerifierWorker = "production"
		c.Services.DCAServer = "production"
		c.Services.DCAWorker = "production"
		c.Services.DCAScheduler = "production"
		c.Services.DCATxIndexer = "production"
	}
}

func (c *ClusterConfig) ValidateRepos() error {
	repos := map[string]string{
		"verifier":   c.Repos.Verifier,
		"go_wrappers": c.Repos.GoWrappers,
	}

	if c.IsLocal("vultiserver") {
		repos["vultiserver"] = c.Repos.Vultiserver
	}
	if c.IsLocal("relay") {
		repos["relay"] = c.Repos.Relay
	}
	if c.IsLocal("dca") {
		repos["dca"] = c.Repos.DCA
	}

	for name, path := range repos {
		if path == "" {
			return fmt.Errorf("repo path for %s is not configured", name)
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("repo %s not found at %s", name, path)
		}
	}

	return nil
}
