package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bttnns/joblog/internal/state"
	"github.com/bttnns/joblog/internal/store"
	"github.com/spf13/cobra"
)

func init() { addCommand(newConfigCmd) }

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config [key]",
		Short: "Get or set configuration (state, min, resume, scraper)",
		Long: "With no argument, print all config. With a key, print its value. Use\n" +
			"'jl config set <key> <value>' to change one. Keys: state, min, resume, scraper.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				if wantJSON(cmd) {
					return emitJSON(cfg)
				}
				fmt.Printf("state:   %s\nmin:     %d\nresume:  %s\nscraper: %s\n", cfg.State, cfg.Min, cfg.ResumePath, scraperTemplate(cfg))
				return nil
			}
			v, err := getConfigKey(cfg, args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				return emitJSON(map[string]string{args[0]: v})
			}
			fmt.Println(v)
			return nil
		},
	}
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (state, min, resume, scraper)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, release, err := openStoreForWrite(cmd)
			if err != nil {
				return err
			}
			defer release()
			cfg, err := s.LoadConfig()
			if err != nil {
				return err
			}
			if err := setConfigKey(&cfg, args[0], args[1]); err != nil {
				return err
			}
			if err := s.SaveConfig(cfg); err != nil {
				return err
			}
			info("set %s = %s", args[0], args[1])
			// Switching state does not clear a manual weekly-minimum override, which
			// would otherwise silently apply the old number to the new state.
			if args[0] == "state" && cfg.Min > 0 {
				info("note: a manual weekly minimum (min=%d) is still set and overrides %s's default; clear it with 'jl config set min 0'", cfg.Min, args[1])
			}
			return nil
		},
	}
}

func getConfigKey(cfg store.Config, key string) (string, error) {
	switch key {
	case "state":
		return cfg.State, nil
	case "min":
		return strconv.Itoa(cfg.Min), nil
	case "resume", "resume_path":
		return cfg.ResumePath, nil
	case "scraper":
		return scraperTemplate(cfg), nil
	default:
		return "", fmt.Errorf("unknown config key %q (keys: state, min, resume, scraper)", key)
	}
}

func setConfigKey(cfg *store.Config, key, value string) error {
	switch key {
	case "state":
		if _, ok := state.Get(value); !ok {
			return fmt.Errorf("unknown state %q (states: %s)", value, joinVocab(state.Codes()))
		}
		cfg.State = value
	case "min":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("min must be a non-negative integer")
		}
		cfg.Min = n
	case "resume", "resume_path":
		cfg.ResumePath = value
	case "scraper":
		cfg.Scraper = value
	default:
		return fmt.Errorf("unknown config key %q (keys: state, min, resume, scraper)", key)
	}
	return nil
}

// scraperTemplate returns the configured scraper command template, falling back
// to store.DefaultScraper when the config key is unset. The default is applied at
// read time, never persisted, so the shipped default can change without rewriting
// every user's config.yaml.
func scraperTemplate(cfg store.Config) string {
	if strings.TrimSpace(cfg.Scraper) == "" {
		return store.DefaultScraper
	}
	return cfg.Scraper
}
