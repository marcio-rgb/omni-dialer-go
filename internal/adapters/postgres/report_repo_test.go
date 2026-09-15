package postgres

import (
	"context"
	"testing"
	"time"

	"dialer-go/internal/domain"
)

func TestReportRepo_NilPoolHandling(t *testing.T) {
	ctx := context.Background()
	repo := NewReportRepo(nil)

	t.Run("SaveCDR_NilPool", func(t *testing.T) {
		err := repo.SaveCDR(ctx, &domain.CDR{ID: "test-cdr"})
		if err == nil {
			t.Errorf("esperava erro com pool nil, obteve nil")
		}
	})

	t.Run("UpdateCDRTranscription_NilPool", func(t *testing.T) {
		err := repo.UpdateCDRTranscription(ctx, "test-cdr", "texto transcrito")
		if err == nil {
			t.Errorf("esperava erro com pool nil, obteve nil")
		}
	})

	t.Run("UpdateCDRTranscription_EmptyParams", func(t *testing.T) {
		if err := repo.UpdateCDRTranscription(ctx, "", "texto"); err != nil {
			t.Errorf("esperava retorno nil para cdrID vazio, obteve %v", err)
		}
		if err := repo.UpdateCDRTranscription(ctx, "id", ""); err != nil {
			t.Errorf("esperava retorno nil para transcription vazia, obteve %v", err)
		}
	})

	t.Run("ListCDRs_NilPool", func(t *testing.T) {
		_, err := repo.ListCDRs(ctx, domain.CDRFilter{TenantID: "tenant-1"})
		if err == nil {
			t.Errorf("esperava erro com pool nil, obteve nil")
		}
	})

	t.Run("GetCDRByID_NilPool", func(t *testing.T) {
		_, err := repo.GetCDRByID(ctx, "tenant-1", "cdr-1")
		if err == nil {
			t.Errorf("esperava erro com pool nil, obteve nil")
		}
	})

	t.Run("GetCallsSummary_NilPool", func(t *testing.T) {
		_, err := repo.GetCallsSummary(ctx, "tenant-1", time.Now().Add(-time.Hour), time.Now(), nil)
		if err == nil {
			t.Errorf("esperava erro com pool nil, obteve nil")
		}
	})
}
