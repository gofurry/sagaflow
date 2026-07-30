package api

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (s *Server) refreshProjectManifest(ctx context.Context, projectID uuid.UUID) {
	if err := s.storage.RefreshProjectManifest(ctx, projectID); err != nil {
		s.log.Warn("refresh project file manifest", zap.String("project_id", projectID.String()), zap.Error(err))
	}
}
