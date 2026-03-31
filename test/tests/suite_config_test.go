package test

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

const suiteConfigFile = "./testconfig/suites.yaml"

// SuiteConfig controls which test suites run in this pipeline execution.
// CI writes this file before tests run.
type SuiteConfig struct {
	Suites []string `yaml:"suites"`
}

var allSuites = []string{"zone", "postgresql", "cronjob"}

func loadSuiteConfig() SuiteConfig {
	data, err := os.ReadFile(suiteConfigFile)
	if err != nil {
		return SuiteConfig{Suites: allSuites}
	}
	var cfg SuiteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return SuiteConfig{Suites: allSuites}
	}
	return cfg
}

func (c SuiteConfig) Has(suite string) bool {
	for _, s := range c.Suites {
		if s == suite {
			return true
		}
	}
	return false
}

// requireSuite skips the test if the named suite is not active in this run.
func requireSuite(t *testing.T, cfg SuiteConfig, suite string) {
	t.Helper()
	if !cfg.Has(suite) {
		t.Skipf("suite %q not active (active: %v)", suite, cfg.Suites)
	}
}
