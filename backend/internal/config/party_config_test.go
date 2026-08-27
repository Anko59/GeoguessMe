package config

import (
	"testing"
	"time"
)

// The party time settings live in their own test file because
// config_test.go sits at the repository's 500-line structure limit.

func TestPartyTimeLoadDefaults(t *testing.T) {
	unsetAllConfigVariables()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("absent variables must not produce load errors, got %v", err)
	}
	if cfg.PartyTimeDuration != time.Hour {
		t.Errorf("Expected default PartyTimeDuration 1h, got %v", cfg.PartyTimeDuration)
	}
	if cfg.PartyTimeCooldown != 48*time.Hour {
		t.Errorf("Expected default PartyTimeCooldown 48h, got %v", cfg.PartyTimeCooldown)
	}
}

func TestPartyTimeLoadOverrides(t *testing.T) {
	t.Setenv("PARTY_TIME_DURATION", "30m")
	t.Setenv("PARTY_TIME_COOLDOWN", "24h")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid overrides must not produce load errors, got %v", err)
	}
	if cfg.PartyTimeDuration != 30*time.Minute || cfg.PartyTimeCooldown != 24*time.Hour {
		t.Errorf("overrides = (%v, %v), want (30m, 24h)", cfg.PartyTimeDuration, cfg.PartyTimeCooldown)
	}
}

func TestPartyTimeValidation(t *testing.T) {
	cases := map[string]func(*Config){
		"zero party duration":     func(c *Config) { c.PartyTimeDuration = 0 },
		"negative party cooldown": func(c *Config) { c.PartyTimeCooldown = -time.Minute },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
	t.Run("zero cooldown is a valid operator choice", func(t *testing.T) {
		c := validConfig()
		c.PartyTimeCooldown = 0
		if err := c.Validate(); err != nil {
			t.Fatalf("zero cooldown must validate, got %v", err)
		}
	})
}
