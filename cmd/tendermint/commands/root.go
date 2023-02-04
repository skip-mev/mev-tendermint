package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/cli"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
)

var (
	config = cfg.DefaultConfig()
	logger = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", config.LogLevel, "log level")
}

// ParseConfig retrieves the default environment configuration,
// sets up the Tendermint root and ensures that the root exists
func ParseConfig(cmd *cobra.Command) (*cfg.Config, error) {
	conf := cfg.DefaultConfig()
	err := viper.Unmarshal(conf)
	if err != nil {
		return nil, err
	}

	var home string
	if os.Getenv("TMHOME") != "" {
		home = os.Getenv("TMHOME")
	} else {
		home, err = cmd.Flags().GetString(cli.HomeFlag)
		if err != nil {
			return nil, err
		}
	}

	conf.RootDir = home

	conf.SetRoot(conf.RootDir)
	cfg.EnsureRoot(conf.RootDir)
	if err := conf.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("error in config file: %v", err)
	}
	// Add auction sentinel to config if not present and set
	// Temporarily support both SentinelPeerString and RelayerPeerString
	sentinelPeerString := conf.Sidecar.SentinelPeerString
	if len(sentinelPeerString) == 0 {
		logger.Info(`[mev-tendermint]: WARNING: sentinel_peer_string not found in config.toml. 
			relayer_peer_string is being deprecated for sentinel_peer_string`)
		sentinelPeerString = conf.Sidecar.RelayerPeerString
	}
	if len(sentinelPeerString) > 0 {
		sentinelID := strings.Split(sentinelPeerString, "@")[0]
		if !strings.Contains(conf.P2P.PrivatePeerIDs, sentinelID) {
			// safety check to not blow away existing private peers if any
			if len(conf.P2P.PrivatePeerIDs) > 0 {
				if conf.P2P.PrivatePeerIDs[len(conf.P2P.PrivatePeerIDs)-1:] == "," {
					conf.P2P.PrivatePeerIDs = conf.P2P.PrivatePeerIDs + sentinelID
				} else {
					conf.P2P.PrivatePeerIDs = conf.P2P.PrivatePeerIDs + "," + sentinelID
				}
			} else {
				conf.P2P.PrivatePeerIDs = sentinelID
			}
		}
	}
	return conf, nil
}

// RootCmd is the root command for Tendermint core.
var RootCmd = &cobra.Command{
	Use:   "tendermint",
	Short: "BFT state machine replication for applications in any programming languages",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if cmd.Name() == VersionCmd.Name() {
			return nil
		}

		config, err = ParseConfig(cmd)
		if err != nil {
			return err
		}

		if config.LogFormat == cfg.LogFormatJSON {
			logger = log.NewTMJSONLogger(log.NewSyncWriter(os.Stdout))
		}

		logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel)
		if err != nil {
			return err
		}

		if viper.GetBool(cli.TraceFlag) {
			logger = log.NewTracingLogger(logger)
		}

		logger = logger.With("module", "main")
		return nil
	},
}

// deprecateSnakeCase is a util function for 0.34.1. Should be removed in 0.35
func deprecateSnakeCase(cmd *cobra.Command, args []string) {
	if strings.Contains(cmd.CalledAs(), "_") {
		fmt.Println("Deprecated: snake_case commands will be replaced by hyphen-case commands in the next major release")
	}
}
