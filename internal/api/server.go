package api

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofurry/sagaflow/internal/config"
	"github.com/gofurry/sagaflow/internal/platform/storage"
	"github.com/gofurry/sagaflow/internal/queue"
	"github.com/gofurry/sagaflow/internal/service"
	"github.com/gofurry/sagaflow/internal/store/db"
	"go.uber.org/zap"
)

type Dependencies struct {
	Config           config.Config
	Logger           *zap.Logger
	Store            *db.Store
	Queue            *queue.Client
	MediaQueue       *queue.MediaClient
	Storage          *storage.Manager
	Auth             *service.AuthService
	Credentials      *service.CredentialService
	Voices           *service.VoiceService
	ModelConnections *service.ModelConnectionService
	Workflows        *service.WorkflowService
	StorageService   *service.StorageService
	MediaTools       *service.MediaToolsService
}
type Server struct {
	cfg              config.Config
	log              *zap.Logger
	store            *db.Store
	queue            *queue.Client
	mediaQueue       *queue.MediaClient
	storage          *storage.Manager
	auth             *service.AuthService
	credentials      *service.CredentialService
	voices           *service.VoiceService
	modelConnections *service.ModelConnectionService
	workflows        *service.WorkflowService
	storageService   *service.StorageService
	mediaTools       *service.MediaToolsService
	uploadSlots      chan struct{}
	downloadSlots    chan struct{}
	loginLimiter     *loginRateLimiter
}

func New(deps Dependencies) *fiber.App {
	log := deps.Logger
	if log == nil {
		log = zap.NewNop()
	}
	s := &Server{
		cfg: deps.Config, log: log, store: deps.Store, queue: deps.Queue, mediaQueue: deps.MediaQueue,
		storage: deps.Storage, auth: deps.Auth, credentials: deps.Credentials, voices: deps.Voices,
		modelConnections: deps.ModelConnections, workflows: deps.Workflows, storageService: deps.StorageService,
		mediaTools: deps.MediaTools, uploadSlots: make(chan struct{}, 2), downloadSlots: make(chan struct{}, 4),
		loginLimiter: newLoginRateLimiter(),
	}
	app := fiber.New(fiber.Config{
		BodyLimit:                    int(maxUploadSize + (1 << 20)),
		ReadTimeout:                  10 * time.Minute,
		StreamRequestBody:            true,
		DisablePreParseMultipartForm: true,
		ErrorHandler:                 errorHandler(log),
	})
	s.RegisterRoutes(app)
	return app
}
