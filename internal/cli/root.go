package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gofurry/sagaflow/internal/app"
	"github.com/gofurry/sagaflow/internal/backup"
	"github.com/gofurry/sagaflow/internal/config"
	mediaffmpeg "github.com/gofurry/sagaflow/internal/media/ffmpeg"
	applogger "github.com/gofurry/sagaflow/internal/platform/logger"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/runtimecontract"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func NewRootCommand(version string) *cobra.Command {
	var cfgPath, runtimeFile string
	run := func(cmd *cobra.Command, _ []string) error {
		cfg, log, err := loadRuntime(cfgPath, version)
		if err != nil {
			return err
		}
		defer log.Sync()
		return app.RunWithOptions(cmd.Context(), cfg, log, app.RunOptions{
			RuntimeFile: runtimeFile, DesktopControlToken: os.Getenv("SAGAFLOW_DESKTOP_TOKEN"),
		})
	}
	root := &cobra.Command{
		Use: "sagaflow", Short: "SagaFlow personal AI production workbench", SilenceUsage: true, Version: version,
		RunE: run,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "runtime config file path")
	root.PersistentFlags().StringVar(&runtimeFile, "runtime-file", "", "write ephemeral process information after the server is ready")
	root.AddCommand(&cobra.Command{Use: "serve", Short: "Start the workbench", RunE: run})

	var versionJSON bool
	versionCmd := &cobra.Command{Use: "version", Short: "Print version and protocol compatibility", RunE: func(_ *cobra.Command, _ []string) error {
		info := struct {
			Version           string `json:"version"`
			APIVersion        int    `json:"api_version"`
			DataSchemaVersion int    `json:"data_schema_version"`
		}{Version: version, APIVersion: runtimecontract.APIVersion, DataSchemaVersion: runtimecontract.DataSchemaVersion}
		if versionJSON {
			return json.NewEncoder(os.Stdout).Encode(info)
		}
		fmt.Printf("SagaFlow %s (API %d, data schema %d)\n", info.Version, info.APIVersion, info.DataSchemaVersion)
		return nil
	}}
	versionCmd.Flags().BoolVar(&versionJSON, "json", false, "print machine-readable JSON")
	root.AddCommand(versionCmd)

	configCmd := &cobra.Command{Use: "config", Short: "Manage the small runtime configuration"}
	var output string
	var force bool
	configInit := &cobra.Command{Use: "init", Short: "Write a default runtime config", RunE: func(_ *cobra.Command, _ []string) error {
		path := output
		if path == "" {
			path = config.DefaultConfigPath(config.Default().App.DataDir)
		}
		if err := config.WriteDefault(path, force); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", path)
		return nil
	}}
	configInit.Flags().StringVarP(&output, "output", "o", "", "output config path")
	configInit.Flags().BoolVar(&force, "force", false, "overwrite an existing config")
	configCmd.AddCommand(configInit)
	root.AddCommand(configCmd)

	accountCmd := &cobra.Command{Use: "account", Short: "Manage the single local account"}
	var username, displayName, password string
	accountInit := &cobra.Command{Use: "init", Short: "Create the only account", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, closeDB, store, err := openStore(cmd.Context(), cfgPath, version)
		if err != nil {
			return err
		}
		defer closeDB()
		auth, err := service.NewAuthService(store, cfg.Auth)
		if err != nil {
			return err
		}
		account, err := auth.Initialize(cmd.Context(), username, displayName, password)
		if err != nil {
			return err
		}
		fmt.Printf("account %s initialized\n", account.Username)
		return nil
	}}
	accountInit.Flags().StringVar(&username, "username", "admin", "login username")
	accountInit.Flags().StringVar(&displayName, "display-name", "Creator", "display name")
	accountInit.Flags().StringVar(&password, "password", "", "password (at least 8 characters)")
	_ = accountInit.MarkFlagRequired("password")
	accountCmd.AddCommand(accountInit)

	var resetPassword string
	reset := &cobra.Command{Use: "reset-password", Short: "Replace the password and revoke sessions", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, closeDB, store, err := openStore(cmd.Context(), cfgPath, version)
		if err != nil {
			return err
		}
		defer closeDB()
		auth, err := service.NewAuthService(store, cfg.Auth)
		if err != nil {
			return err
		}
		account, err := auth.SetPassword(cmd.Context(), resetPassword)
		if err != nil {
			return err
		}
		fmt.Printf("password reset for %s\n", account.Username)
		return nil
	}}
	reset.Flags().StringVar(&resetPassword, "password", "", "new password (at least 8 characters)")
	_ = reset.MarkFlagRequired("password")
	accountCmd.AddCommand(reset)
	root.AddCommand(accountCmd)

	var doctorJSON bool
	doctorCmd := &cobra.Command{Use: "doctor", Short: "Check the data directory and SQLite database", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, closeDB, store, err := openStore(cmd.Context(), cfgPath, version)
		if err != nil {
			return err
		}
		defer closeDB()
		initialized, err := store.AuthInitialized(cmd.Context())
		if err != nil {
			return err
		}
		credentialService, err := service.NewCredentialService(store, cfg.MasterKeyPath())
		if err != nil {
			return err
		}
		credentials, err := store.ListProviderCredentials(cmd.Context(), nil)
		if err != nil {
			return err
		}
		for _, credential := range credentials {
			if _, err := credentialService.SecretByID(cmd.Context(), credential.ID); err != nil {
				return fmt.Errorf("decrypt provider credential %s: %w", credential.ID, err)
			}
		}
		connections, err := store.ListS3Connections(cmd.Context())
		if err != nil {
			return err
		}
		for _, connection := range connections {
			if _, err := credentialService.DecryptSecret(connection.EncryptedCredentials); err != nil {
				return fmt.Errorf("decrypt S3 connection %s: %w", connection.ID, err)
			}
		}
		ffmpegStatus := mediaffmpeg.Discover(cfg.FFmpegDir()).Status()
		result := struct {
			DataDir             string `json:"data_dir"`
			Database            string `json:"database"`
			AccountInitialized  bool   `json:"account_initialized"`
			ProviderCredentials int    `json:"provider_credentials"`
			S3Credentials       int    `json:"s3_credentials"`
			FFmpegAvailable     bool   `json:"ffmpeg_available"`
			FFmpegVersion       string `json:"ffmpeg_version,omitempty"`
			FFmpegPath          string `json:"ffmpeg_path,omitempty"`
		}{
			DataDir: cfg.App.DataDir, Database: "ok", AccountInitialized: initialized,
			ProviderCredentials: len(credentials), S3Credentials: len(connections),
			FFmpegAvailable: ffmpegStatus.Available, FFmpegVersion: ffmpegStatus.Version, FFmpegPath: ffmpegStatus.FFmpegPath,
		}
		if doctorJSON {
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		fmt.Printf("data: %s\ndatabase: %s\naccount initialized: %t\nprovider credentials: %d valid\nS3 credentials: %d valid\nFFmpeg available: %t\n",
			result.DataDir, result.Database, result.AccountInitialized, result.ProviderCredentials, result.S3Credentials, result.FFmpegAvailable)
		return nil
	}}
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "print machine-readable JSON")
	root.AddCommand(doctorCmd)

	backupCmd := &cobra.Command{Use: "backup", Short: "Back up or restore the complete personal workspace"}
	var backupOutput string
	createBackup := &cobra.Command{Use: "create", Short: "Create a consistent SQLite, object and secret archive", RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		path, err := backup.Create(cmd.Context(), cfg, backupOutput)
		if err != nil {
			return err
		}
		fmt.Printf("backup written to %s\n", path)
		return nil
	}}
	createBackup.Flags().StringVarP(&backupOutput, "output", "o", "", "output zip path")
	var restoreArchive string
	var confirmRestore bool
	restoreBackup := &cobra.Command{Use: "restore", Short: "Restore a complete archive while SagaFlow is stopped", RunE: func(cmd *cobra.Command, _ []string) error {
		if !confirmRestore {
			return fmt.Errorf("restore replaces the active data directory; pass --yes after stopping SagaFlow")
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		previous, err := backup.Restore(cmd.Context(), cfg, restoreArchive)
		if err != nil {
			return err
		}
		fmt.Println("backup restored")
		if previous != "" {
			fmt.Printf("previous data retained at %s\n", previous)
		}
		return nil
	}}
	restoreBackup.Flags().StringVarP(&restoreArchive, "archive", "i", "", "backup zip path")
	restoreBackup.Flags().BoolVar(&confirmRestore, "yes", false, "confirm replacement of the active data directory")
	_ = restoreBackup.MarkFlagRequired("archive")
	backupCmd.AddCommand(createBackup, restoreBackup)
	root.AddCommand(backupCmd)

	serviceCmd := &cobra.Command{Use: "service", Short: "Install or remove the Linux systemd service"}
	var serviceUser, unitPath string
	install := &cobra.Command{Use: "install", Short: "Install and start SagaFlow with systemd", RunE: func(cmd *cobra.Command, _ []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("systemd installation is only available on Linux")
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}
		if err := cfg.EnsureRuntimeDirs(); err != nil {
			return err
		}
		initializedDB, err := sqlite.Open(cmd.Context(), cfg.DatabasePath())
		if err != nil {
			return err
		}
		initialized, err := db.New(initializedDB).AuthInitialized(cmd.Context())
		_ = initializedDB.Close()
		if err != nil {
			return err
		}
		if !initialized {
			return fmt.Errorf("account is not initialized; run `sagaflow account init` first")
		}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		args := systemdQuote(executable) + " serve"
		if cfgPath != "" {
			absoluteConfig, err := filepath.Abs(cfgPath)
			if err != nil {
				return err
			}
			args += " --config " + systemdQuote(absoluteConfig)
		}
		unit := "[Unit]\nDescription=SagaFlow personal workbench\nAfter=network-online.target\nWants=network-online.target\n\n" +
			"[Service]\nType=simple\nUser=" + serviceUser + "\nWorkingDirectory=" + systemdQuote(cfg.App.DataDir) + "\nExecStart=" + args + "\nRestart=on-failure\nRestartSec=3\nNoNewPrivileges=true\nPrivateTmp=true\n\n" +
			"[Install]\nWantedBy=multi-user.target\n"
		if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
			return fmt.Errorf("write systemd unit (run as root): %w", err)
		}
		if err := runSystemctl(cmd.Context(), "daemon-reload"); err != nil {
			return err
		}
		if err := runSystemctl(cmd.Context(), "enable", "--now", filepath.Base(unitPath)); err != nil {
			return err
		}
		fmt.Printf("installed %s\n", unitPath)
		return nil
	}}
	currentUser := defaultServiceUser(user.Current())
	install.Flags().StringVar(&serviceUser, "user", currentUser, "Linux user that runs SagaFlow")
	install.Flags().StringVar(&unitPath, "unit", "/etc/systemd/system/sagaflow.service", "systemd unit path")
	uninstall := &cobra.Command{Use: "uninstall", Short: "Stop and remove the systemd service", RunE: func(cmd *cobra.Command, _ []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("systemd installation is only available on Linux")
		}
		path := "/etc/systemd/system/sagaflow.service"
		_ = runSystemctl(cmd.Context(), "disable", "--now", filepath.Base(path))
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return runSystemctl(cmd.Context(), "daemon-reload")
	}}
	serviceCmd.AddCommand(install, uninstall)
	root.AddCommand(serviceCmd)
	return root
}

func Execute(ctx context.Context, version string) error {
	return NewRootCommand(version).ExecuteContext(ctx)
}

func loadRuntime(path, version string) (config.Config, *zap.Logger, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, err
	}
	cfg.App.Version = version
	log, err := applogger.New(cfg.App)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, log, nil
}

func openStore(ctx context.Context, path, version string) (config.Config, func(), *db.Store, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	cfg.App.Version = version
	if err := cfg.EnsureRuntimeDirs(); err != nil {
		return config.Config{}, nil, nil, err
	}
	database, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	return cfg, func() { _ = database.Close() }, db.New(database), nil
}

func runSystemctl(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "systemctl", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func defaultServiceUser(current *user.User, err error) string {
	if err == nil && current != nil && strings.TrimSpace(current.Username) != "" {
		return current.Username
	}
	return "sagaflow"
}
