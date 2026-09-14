package core

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"
)

type mockStorageForMailing struct {
	ports.StoragePort
	data []byte
}

func (m *mockStorageForMailing) DownloadFileStream(ctx context.Context, fileURI string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

type mockLeadRepoForMailing struct {
	ports.LeadRepository
	inserted []*domain.Lead
}

func (m *mockLeadRepoForMailing) BatchInsert(ctx context.Context, leads []*domain.Lead) (int64, error) {
	m.inserted = append(m.inserted, leads...)
	return int64(len(leads)), nil
}

func createTestZip(csvContent string) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("leads.csv")
	_, _ = w.Write([]byte(csvContent))
	_ = zw.Close()
	return buf.Bytes()
}

func TestMailingProcessor_CommasInFirstName(t *testing.T) {
	ctx := context.Background()

	// 1. Linha com aspas RFC 4180 contendo vírgula
	// 2. Linha sem aspas com vírgula no campo first_name (reconstituição automática)
	// 3. Linha com vírgula escapada (\,)
	csvData := strings.Join([]string{
		"cpf,telefone,campanha_id,tenant_id,name,first_name",
		`11122233344,11988887777,camp101,tenant1,MARCIO NASCIMENTO,"; Márcio, tudo bem? ;"`,
		`55566677788,11977776666,camp101,tenant1,JOAO SILVA,; João, beleza? ;`,
		`99900011122,11966665555,camp101,tenant1,PEDRO SOUZA,; Pedro\, como vai? ;`,
	}, "\n")

	zipBytes := createTestZip(csvData)

	storage := &mockStorageForMailing{data: zipBytes}
	repo := &mockLeadRepoForMailing{}
	cache := &mockCachePredictive{}

	processor := NewMailingProcessor(storage, repo, cache)

	resp, err := processor.ProcessZipRefill(ctx, "tenant1", "camp101", "s3://test.zip")
	if err != nil {
		t.Fatalf("ProcessZipRefill falhou: %v", err)
	}

	if resp.ValidRows != 3 {
		t.Fatalf("esperava 3 linhas válidas, obteve %d", resp.ValidRows)
	}

	if len(repo.inserted) != 3 {
		t.Fatalf("esperava 3 leads persistidos, obteve %d", len(repo.inserted))
	}

	// 1. Quoted RFC 4180
	if repo.inserted[0].Name != "MARCIO NASCIMENTO" {
		t.Errorf("lead 0 Name incorreto: %s", repo.inserted[0].Name)
	}
	if repo.inserted[0].FirstName != "marcio_tudo_bem" {
		t.Errorf("lead 0 FirstName slug incorreto: '%s' (esperava 'marcio_tudo_bem')", repo.inserted[0].FirstName)
	}

	// 2. Unquoted CSV com vírgula no first_name
	if repo.inserted[1].Name != "JOAO SILVA" {
		t.Errorf("lead 1 Name incorreto: %s", repo.inserted[1].Name)
	}
	if repo.inserted[1].FirstName != "joao_beleza" {
		t.Errorf("lead 1 FirstName slug incorreto: '%s' (esperava 'joao_beleza')", repo.inserted[1].FirstName)
	}

	// 3. Escaped comma (\,)
	if repo.inserted[2].Name != "PEDRO SOUZA" {
		t.Errorf("lead 2 Name incorreto: %s", repo.inserted[2].Name)
	}
	if repo.inserted[2].FirstName != "pedro_como_vai" {
		t.Errorf("lead 2 FirstName slug incorreto: '%s' (esperava 'pedro_como_vai')", repo.inserted[2].FirstName)
	}
}
