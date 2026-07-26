package api

import (
	"crypto/subtle"
	"net"
	"time"

	fiberzap "github.com/gofiber/contrib/v3/zap"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofurry/sagaflow/internal/runtimecontract"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (s *Server) RegisterRoutes(app *fiber.App) {
	app.Use(recover.New(), requestid.New(), fiberzap.New(fiberzap.Config{
		Logger: s.log,
		FieldsFunc: func(c fiber.Ctx) []zap.Field {
			return []zap.Field{zap.String("request_id", c.RequestID())}
		},
		// Successful polling and health requests are useful while debugging but
		// should not grow production logs indefinitely.
		Levels: []zapcore.Level{zapcore.ErrorLevel, zapcore.WarnLevel, zapcore.DebugLevel},
	}))
	app.Use(limitNonMultipartBody(4 << 20))
	app.Get("/health", s.health)
	if s.desktopControlToken != "" && s.shutdown != nil {
		app.Post("/_desktop/shutdown", s.desktopShutdown)
	}
	api := app.Group("/api")
	auth := api.Group("/auth")
	auth.Get("/status", s.authStatus)
	auth.Post("/setup", s.setupAccount)
	auth.Post("/login", s.login)
	auth.Post("/logout", s.logout)
	p := api.Group("", s.requireAuth)
	p.Get("/me", s.currentUser)
	p.Patch("/me", s.updateProfile)
	p.Post("/me/password", s.changePassword)
	p.Get("/s3-connections", s.listStorageBackends)
	p.Post("/s3-connections", s.createStorageBackend)
	p.Patch("/s3-connections/:id", s.updateStorageBackend)
	p.Delete("/s3-connections/:id", s.deleteStorageBackend)
	p.Post("/s3-connections/:id/test", s.testStorageBackend)
	p.Get("/projects", s.listProjects)
	p.Post("/projects", s.createProject)
	p.Get("/projects/:id", s.getProject)
	p.Patch("/projects/:id", s.updateProject)
	p.Delete("/projects/:id", s.deleteProject)
	p.Get("/projects/:id/episodes", s.listEpisodes)
	p.Post("/projects/:id/episodes", s.createEpisode)
	p.Get("/episodes/:id", s.getEpisode)
	p.Patch("/episodes/:id", s.updateEpisode)
	p.Delete("/episodes/:id", s.deleteEpisode)
	p.Get("/episodes/:id/scripts", s.listEpisodeScripts)
	p.Post("/episodes/:id/scripts", s.createEpisodeScript)
	p.Post("/episode-scripts/:id/adopt", s.adoptEpisodeScript)
	p.Get("/projects/:id/asset-groups", s.listAssetGroups)
	p.Post("/projects/:id/asset-groups", s.createAssetGroup)
	p.Patch("/asset-groups/:id", s.updateAssetGroup)
	p.Delete("/asset-groups/:id", s.deleteAssetGroup)
	p.Get("/projects/:id/assets/page", s.listAssetsPage)
	p.Get("/projects/:id/assets/summary", s.assetSummary)
	p.Get("/projects/:id/assets/by-ids", s.listAssetsByIDs)
	p.Get("/projects/:id/assets", s.listAssets)
	p.Get("/media-tools/status", s.mediaToolsStatus)
	p.Post("/media-tools/install", s.installMediaTools)
	p.Post("/media-tools/install/cancel", s.cancelMediaToolsInstall)
	p.Post("/media-tools/refresh", s.refreshMediaTools)
	p.Get("/projects/:id/media-jobs", s.listMediaJobs)
	p.Post("/projects/:id/media-jobs", s.createMediaJob)
	p.Delete("/projects/:id/media-jobs/completed", s.clearCompletedMediaJobs)
	p.Get("/media-jobs/:id", s.getMediaJob)
	p.Post("/media-jobs/:id/cancel", s.cancelMediaJob)
	p.Get("/assets/:id/media-info", s.getAssetMediaInfo)
	p.Get("/projects/:id/asset-exports", s.listProjectAssetExports)
	p.Get("/asset-groups/:id/assets", s.listGroupAssets)
	p.Post("/asset-groups/:id/assets/upload", s.withUploadSlot(s.uploadAsset))
	p.Get("/assets/:id/file", s.getAssetFile)
	p.Patch("/assets/:id", s.updateAsset)
	p.Post("/assets/:id/adopt", s.adoptAsset)
	p.Post("/assets/:id/discard", s.discardAsset)
	p.Delete("/assets/:id", s.deleteAsset)
	p.Get("/assets/:id/exports", s.listAssetExports)
	p.Post("/assets/:id/exports", s.publishAsset)
	p.Delete("/asset-exports/:id", s.deleteAssetExport)
	p.Get("/asset-exports/:id/url", s.getAssetExportURL)
	p.Get("/episodes/:id/canvas", s.getCanvas)
	p.Put("/episodes/:id/canvas", s.saveCanvas)
	p.Patch("/canvas-nodes/:id/selected-video", s.selectCanvasVideo)
	p.Get("/model-providers", s.listModelProviders)
	p.Post("/model-providers", s.createModelProvider)
	p.Patch("/model-providers/:id", s.updateModelProvider)
	p.Delete("/model-providers/:id", s.deleteModelProvider)
	p.Post("/model-providers/:id/test", s.testModelProvider)
	p.Post("/model-providers/:id/discover", s.discoverProviderModels)
	p.Post("/model-providers/:id/sync", s.syncProviderModels)
	p.Get("/model-catalog", s.listModels)
	p.Post("/model-catalog", s.createModel)
	p.Get("/model-catalog/update", s.modelCatalogUpdateStatus)
	p.Post("/model-catalog/update", s.importModelCatalog)
	p.Patch("/model-catalog/:id", s.updateModel)
	p.Delete("/model-catalog/:id", s.deleteModel)
	p.Get("/model-presets", s.listModelPresets)
	p.Post("/model-presets", s.createModelPreset)
	p.Patch("/model-presets/:id", s.updateModelPreset)
	p.Delete("/model-presets/:id", s.deleteModelPreset)
	p.Get("/workflow-templates", s.listWorkflowTemplates)
	p.Post("/workflow-templates/analyze", s.analyzeWorkflowTemplate)
	p.Post("/workflow-templates", s.createWorkflowTemplate)
	p.Patch("/workflow-templates/:id", s.updateWorkflowTemplate)
	p.Delete("/workflow-templates/:id", s.deleteWorkflowTemplate)
	p.Get("/workflow-compatibilities", s.listWorkflowCompatibilities)
	p.Post("/workflow-templates/:id/check", s.checkWorkflowCompatibility)
	p.Get("/voice-capabilities", s.listVoiceCapabilities)
	p.Get("/voice-profiles", s.listVoiceProfiles)
	p.Post("/voice-profiles", s.withUploadSlot(s.createVoiceProfile))
	p.Get("/voice-profiles/:id/source", s.getVoiceProfileSource)
	p.Post("/voice-profiles/:id/bindings", s.createVoiceBinding)
	p.Patch("/voice-profiles/:id", s.updateVoiceProfile)
	p.Delete("/voice-profiles/:id", s.deleteVoiceProfile)
	p.Get("/voice-bindings/:id/preview", s.getVoiceBindingPreview)
	p.Delete("/voice-bindings/:id", s.deleteVoiceBinding)
	p.Get("/prompt-presets", s.listPromptPresets)
	p.Post("/prompt-presets", s.createPromptPreset)
	p.Patch("/prompt-presets/:id", s.updatePromptPreset)
	p.Delete("/prompt-presets/:id", s.deletePromptPreset)
	p.Get("/provider-credentials", s.listProviderCredentials)
	p.Post("/provider-credentials", s.createProviderCredential)
	p.Patch("/provider-credentials/:id", s.updateProviderCredential)
	p.Post("/provider-credentials/:id/activate", s.activateProviderCredential)
	p.Post("/provider-credentials/:id/test", s.testProviderCredential)
	p.Delete("/provider-credentials/:id", s.deleteProviderCredential)
	p.Post("/generation-jobs", s.createGenerationJob)
	p.Get("/generation-jobs", s.listGenerationJobs)
	p.Get("/generation-jobs/:id/invocation", s.getGenerationInvocation)
	p.Get("/generation-jobs/:id", s.getGenerationJob)
	p.Post("/projects/:id/generation-reference-uploads", s.withUploadSlot(s.uploadGenerationReference))
	p.Get("/generation-reference-uploads/:id/file", s.getGenerationReferenceFile)
	p.Delete("/generation-reference-uploads/:id", s.deleteGenerationReference)
	p.Get("/projects/:id/staged-assets", s.listStagedAssets)
	p.Get("/projects/:id/staged-assets/summary", s.stagedAssetSummary)
	p.Post("/projects/:id/staged-assets/upload", s.withUploadSlot(s.uploadStagedAsset))
	p.Get("/staged-assets/:id/file", s.getStagedAssetFile)
	p.Patch("/staged-assets/:id", s.updateStagedAsset)
	p.Post("/staged-assets/:id/import", s.importStagedAsset)
	p.Delete("/staged-assets/:id", s.deleteStagedAsset)
}
func (s *Server) health(c fiber.Ctx) error {
	return writeOK(c, fiber.Map{
		"status": "ok", "version": s.cfg.App.Version, "api_version": runtimecontract.APIVersion,
		"data_schema_version": runtimecontract.DataSchemaVersion,
	})
}

func (s *Server) desktopShutdown(c fiber.Ctx) error {
	ip := net.ParseIP(c.IP())
	if ip == nil || !ip.IsLoopback() {
		return fiber.ErrForbidden
	}
	provided := c.Get(runtimecontract.DesktopTokenHeader)
	if subtle.ConstantTimeCompare([]byte(provided), []byte(s.desktopControlToken)) != 1 {
		return fiber.ErrUnauthorized
	}
	time.AfterFunc(100*time.Millisecond, s.shutdown)
	return c.SendStatus(fiber.StatusAccepted)
}
