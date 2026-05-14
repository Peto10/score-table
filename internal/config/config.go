package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Timer TimerConfig `yaml:"timer"`
	Teams []Team      `yaml:"teams"`
}

type TimerConfig struct {
	DefaultMinutes int  `yaml:"default_minutes"`
	DefaultSeconds int  `yaml:"default_seconds"`
	ShowByDefault  bool `yaml:"show_by_default"`
}

func (t TimerConfig) DefaultDurationSeconds() int {
	if t.DefaultMinutes < 0 || t.DefaultSeconds < 0 {
		return 0
	}
	return t.DefaultMinutes*60 + t.DefaultSeconds
}

type Team struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Players []Player `yaml:"players"`
}

type Player struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type Index struct {
	TeamsByID   map[string]Team
	PlayersByID map[string]struct {
		Player Player
		TeamID string
	}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if len(c.Teams) < 2 {
		return errors.New("config must contain at least 2 teams")
	}

	if c.Timer.DefaultMinutes < 0 {
		return errors.New("timer.default_minutes must be >= 0")
	}
	if c.Timer.DefaultSeconds < 0 || c.Timer.DefaultSeconds > 59 {
		return errors.New("timer.default_seconds must be between 0 and 59")
	}

	teamIDs := map[string]struct{}{}
	playerIDs := map[string]struct{}{}

	for i, t := range c.Teams {
		if t.ID == "" {
			return fmt.Errorf("teams[%d].id is required", i)
		}
		if t.Name == "" {
			return fmt.Errorf("teams[%d].name is required", i)
		}
		if _, ok := teamIDs[t.ID]; ok {
			return fmt.Errorf("duplicate team id %q", t.ID)
		}
		teamIDs[t.ID] = struct{}{}

		if len(t.Players) == 0 {
			return fmt.Errorf("teams[%d] (%s) must have at least 1 player", i, t.ID)
		}
		for j, p := range t.Players {
			if p.ID == "" {
				return fmt.Errorf("teams[%d].players[%d].id is required", i, j)
			}
			if p.Name == "" {
				return fmt.Errorf("teams[%d].players[%d].name is required", i, j)
			}
			if _, ok := playerIDs[p.ID]; ok {
				return fmt.Errorf("duplicate player id %q", p.ID)
			}
			playerIDs[p.ID] = struct{}{}
		}
	}

	return nil
}

func (c *Config) BuildIndex() Index {
	teamsByID := make(map[string]Team, len(c.Teams))
	playersByID := make(map[string]struct {
		Player Player
		TeamID string
	})

	for _, t := range c.Teams {
		teamsByID[t.ID] = t
		for _, p := range t.Players {
			playersByID[p.ID] = struct {
				Player Player
				TeamID string
			}{Player: p, TeamID: t.ID}
		}
	}

	return Index{
		TeamsByID:   teamsByID,
		PlayersByID: playersByID,
	}
}
