package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gofurry/sagaflow/internal/api"
	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/inference"
	"github.com/gofurry/sagaflow/internal/inference/adapters/bailian"
	"github.com/gofurry/sagaflow/internal/inference/adapters/comfyui"
	"github.com/gofurry/sagaflow/internal/inference/adapters/deepseek"
	"github.com/gofurry/sagaflow/internal/inference/adapters/minimax"
	"github.com/gofurry/sagaflow/internal/inference/adapters/ollama"
	"github.com/gofurry/sagaflow/internal/inference/adapters/openaicompat"
	"github.com/gofurry/sagaflow/internal/inference/adapters/siliconflow"
	"github.com/gofurry/sagaflow/internal/inference/adapters/tencenttokenhub"
	"github.com/gofurry/sagaflow/internal/inference/adapters/volcengine"
	"github.com/gofurry/sagaflow/internal/inference/adapters/zhipu"
	"github.com/gofurry/sagaflow/internal/modelcatalog"
	"github.com/gofurry/sagaflow/internal/platform/sqlite"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/queue"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"github.com/gofurry/sagaflow/internal/webui"
	"go.uber.org/zap"
)

func Run(ctx context.Context, cfg config.Config, log *zap.Logger) error {
	if err := cfg.EnsureRuntimeDirs(); err != nil {
		return err
	}
	database, err := sqlite.Open(ctx, cfg.DatabasePath())
	if err != nil {
		return err
	}
	defer database.Close()
	store := db.New(database)
	if _, err := modelcatalog.SyncFromPath(ctx, store, cfg.ModelCatalogPath()); err != nil {
		return fmt.Errorf("synchronize built-in model catalog: %w", err)
	}
	auth, err := service.NewAuthService(store, cfg.Auth)
	if err != nil {
		return err
	}
	initialized, err := auth.Initialized(ctx)
	if err != nil {
		return err
	}
	if !initialized && !cfg.IsLoopback() {
		return fmt.Errorf("account is not initialized; run `sagaflow account init` before binding to %s", cfg.Server.Host)
	}
	credentials, err := service.NewCredentialService(store, cfg.MasterKeyPath())
	if err != nil {
		return err
	}
	objectStore, err := storage.NewManager(store, cfg.ObjectDir(), cfg.TempDir())
	if err != nil {
		return err
	}
	storageService := service.NewStorageService(store, credentials, objectStore, log.Named("s3"))
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	comfyDriver := comfyui.New(comfyui.Config{HTTPClient: httpClient})
	volcDriver := volcengine.New(volcengine.Config{HTTPClient: httpClient})
	siliconFlowDriver := siliconflow.New(siliconflow.Config{HTTPClient: httpClient})
	zhipuDriver := zhipu.New(zhipu.Config{HTTPClient: httpClient})
	tencentTokenHubDriver := tencenttokenhub.New(tencenttokenhub.Config{HTTPClient: httpClient})
	openAIChatDriver := openaicompat.NewChat(httpClient)
	openAIResponsesDriver := openaicompat.NewResponses(httpClient)
	gateway, err := inference.NewGateway(map[string]inference.Driver{
		service.ProviderDeepSeek:        deepseek.New(httpClient),
		service.ProviderVolcengine:      volcDriver,
		service.ProviderMiniMax:         minimax.New(httpClient),
		service.ProviderAliyunBailian:   bailian.New(bailian.Config{HTTPClient: httpClient}),
		service.ProviderSiliconFlow:     siliconFlowDriver,
		service.ProviderZhipu:           zhipuDriver,
		service.ProviderTencentTokenHub: tencentTokenHubDriver,
		service.ProviderOllama:          ollama.New(httpClient),
		service.ProviderComfyUI:         comfyDriver,
		service.ProviderOpenAIChat:      openAIChatDriver,
		service.ProviderOpenAIResponses: openAIResponsesDriver,
	})
	if err != nil {
		return err
	}
	generation, err := service.NewGenerationService(store, credentials, objectStore, storageService, gateway, log.Named("generation"))
	if err != nil {
		return err
	}
	jobs := queue.NewClient(store, cfg.Jobs, generation.Execute, log.Named("jobs"))
	if err := jobs.Run(ctx); err != nil {
		return err
	}
	defer jobs.Close()
	voices := service.NewVoiceService(store, credentials, objectStore, log.Named("voices"))
	modelConnections, err := service.NewModelConnectionService(store, credentials, ollama.New(httpClient), comfyDriver, siliconFlowDriver, tencentTokenHubDriver)
	if err != nil {
		return err
	}
	workflows, err := service.NewWorkflowService(store, credentials, comfyDriver)
	if err != nil {
		return err
	}
	server := api.New(api.Dependencies{
		Config: cfg, Logger: log, Store: store, Queue: jobs, Storage: objectStore, Auth: auth,
		Credentials: credentials, Voices: voices, ModelConnections: modelConnections, Workflows: workflows, StorageService: storageService,
	})
	webui.Mount(server)
	errCh := make(chan error, 1)
	go func() {
		log.Info("starting SagaFlow", zap.String("addr", cfg.Address()), zap.String("data_dir", cfg.App.DataDir))
		errCh <- server.Listen(cfg.Address())
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.ShutdownWithContext(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// RunAPI remains a source-compatible alias for integrations migrating from the
// former split API/worker deployment.
func RunAPI(ctx context.Context, cfg config.Config, log *zap.Logger) error { return Run(ctx, cfg, log) }
